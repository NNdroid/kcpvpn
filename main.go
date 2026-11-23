package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/xtaci/kcp-go/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/pbkdf2"
	"os/exec"
)

var (
	sugar   *zap.SugaredLogger
	version = "dev"

	// globalConfigPath 记录当前配置文件路径，供 Web UI 自动重启使用。
	globalConfigPath string

	// kdfSalt 用於 PSK 派生（域分離）。
	kdfSalt = []byte("kcpvpn-v1-salt")

	// 物理鏈路 MTU 假設為 1500；減去 IP(20)+UDP(8) 得到不分片的外層 UDP 載荷上限。
	noFragUDPPayload = 1472

	// defaultPSK 是舊版本公開的默認密鑰，用於告警提示。
	defaultPSK = "HereIsYourSuperSecretPassword"
)

// computeVersion 依 git 歷史計算版本號。
//
// 規則: v1.0.<最後一個 commit 的 yyyyMMdd>-<最後一個 commit hash 前 7 位>-<tag 數量 + 1>
// 例如: 最後 commit 為 20260731/e0cb218，tag 數量為 2 → v1.0.20260731-e0cb218-3
//
// 若 git 不可用或非 git 倉庫，回退為 version（由 -ldflags 注入，默認 "dev"）。
func computeVersion() string {
	gitDate, err1 := gitOutput("log", "-1", "--format=%ad", "--date=format:%Y%m%d")
	gitHash, err2 := gitOutput("log", "-1", "--format=%h")
	tagCount, err3 := gitOutput("tag")
	if err1 != nil || err2 != nil || err3 != nil {
		return version
	}
	return fmt.Sprintf("v1.0.%s-%s-%d", gitDate, gitHash, len(strings.Fields(tagCount))+1)
}

// gitOutput 執行 git 命令並返回去除首尾空白後的標準輸出。
func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil // 抑制錯誤輸出
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Config 匯總運行時配置。
//
// 除派生字段外，其餘字段均來自 config.json（鍵為 JSON tag）。JSON 缺省字段
// 由 defaultConfig() 填充，不影響默認值。
type Config struct {
	Mode    string `json:"mode"`
	Addr    string `json:"addr"`
	TapName string `json:"tap"`

	V4NetStr string `json:"v4net"`
	V6NetStr string `json:"v6net"`

	FECData   int `json:"fec-d"`
	FECParity int `json:"fec-p"`

	Crypt    string `json:"crypt"`
	PSK      string `json:"psk"`
	ObfsMode string `json:"obfs"`
	PadMax   int    `json:"pad-max"`

	MTU    int `json:"mtu"`
	SndWnd int `json:"sndwnd"`
	RcvWnd int `json:"rcvwnd"`
	DSCP   int `json:"dscp"`

	LogLevel   string `json:"log-level"`
	MaxClients int    `json:"max-clients"`
	MaxRate    int    `json:"max-rate"`
	FWMark     int    `json:"fwmark"`

	WebEnabled bool   `json:"web-enabled"`
	WebBind    string `json:"web-bind"`
	WebAddr    string `json:"web-addr"`
	WebToken   string `json:"web-token"`

	ReqV4  string `json:"ip4"`
	ReqV6  string `json:"ip6"`
	ReqMac string `json:"mac"`

	// ---- 派生字段（不從 JSON 讀取）----
	V4Net       *net.IPNet     `json:"-"`
	V6Net       *net.IPNet     `json:"-"`
	MaxHosts    uint32         `json:"-"`
	ServerV4    string         `json:"-"`
	ServerV6    string         `json:"-"`
	V4Mask      int            `json:"-"`
	V6Mask      int            `json:"-"`
	Block       kcp.BlockCrypt `json:"-"`
	Key         []byte         `json:"-"`
	KCPMTU      int            `json:"-"`
	MaxFrameLen int            `json:"-"`
}

// defaultConfig 返回帶默認值的配置（JSON 僅覆蓋出現的字段）。
func defaultConfig() *Config {
	return &Config{
		Mode:       "server",
		Addr:       "127.0.0.1:4000",
		TapName:    "tap0",
		V4NetStr:   "10.0.0.0/24",
		V6NetStr:   "fd00::/64",
		FECData:    5,
		FECParity:  2,
		Crypt:      "aes-gcm",
		ObfsMode:   "none",
		MTU:        1500,
		SndWnd:     4096,
		RcvWnd:     4096,
		LogLevel:   "warn",
		MaxClients: 256,
		WebBind:    "local",
		WebAddr:    ":8080",
	}
}

// loadConfig 讀取並校驗 config.json，並派生成派生字段。
// 返回 (cfg, warnings, error)。warnings 在 logger 初始化前收集，由調用方打印。
func loadConfig(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("讀取配置文件失敗 (%s): %v（可參考 config.example.json 作為模板）", path, err)
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, nil, fmt.Errorf("解析配置文件 %s 失敗: %v", path, err)
	}

	var warns []string

	// 歸一化：空字符串回落為默認值。
	if cfg.Mode == "" {
		cfg.Mode = "server"
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:4000"
	}
	if cfg.TapName == "" {
		cfg.TapName = "tap0"
	}
	if cfg.V4NetStr == "" {
		cfg.V4NetStr = "10.0.0.0/24"
	}
	if cfg.Crypt == "" {
		cfg.Crypt = "aes-gcm"
	}
	if cfg.ObfsMode == "" {
		cfg.ObfsMode = "none"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "warn"
	}
	if cfg.WebBind == "" {
		cfg.WebBind = "local"
	}
	if cfg.WebAddr == "" {
		cfg.WebAddr = ":8080"
	}
	if cfg.Mode != "server" && cfg.Mode != "client" {
		return nil, warns, fmt.Errorf("無效的 mode: %q（有效: server, client）", cfg.Mode)
	}

	// S2: 強制指定 PSK。
	if cfg.PSK == "" {
		return nil, warns, fmt.Errorf("config.json 必須指定 psk 預共享密鑰")
	}
	if cfg.PSK == defaultPSK {
		warns = append(warns, "⚠️ 正在使用已公開的默認 PSK，請務必更換")
	}

	cfg.ObfsMode = strings.ToLower(cfg.ObfsMode)
	switch cfg.ObfsMode {
	case "none", "xor", "stun", "dns", "dtls", "quic":
	default:
		return nil, warns, fmt.Errorf("無效的 obfs: %q（有效: none, xor, stun, dns, dtls, quic）", cfg.ObfsMode)
	}

	// Web UI：bind 枚舉 + 端口校驗 + public 必須帶 token。
	cfg.WebBind = strings.ToLower(cfg.WebBind)
	switch cfg.WebBind {
	case "tunnel", "local", "public":
	default:
		return nil, warns, fmt.Errorf("無效的 web-bind: %q（有效: tunnel, local, public）", cfg.WebBind)
	}
	if _, err := webPort(cfg.WebAddr); err != nil {
		return nil, warns, fmt.Errorf("無效的 web-addr: %v", err)
	}
	if cfg.WebEnabled && cfg.WebBind == "public" && cfg.WebToken == "" {
		return nil, warns, fmt.Errorf("web-bind=public 必須設置 web-token 訪問令牌（暴露公網必須鑑權）")
	}

	cfg.Crypt = strings.ToLower(cfg.Crypt)

	// L7: 禁止明文隧道。
	if cfg.Crypt == "none" && cfg.ObfsMode == "none" {
		return nil, warns, fmt.Errorf("明文隧道被禁止 (crypt=none + obfs=none)；請改用 aes-gcm 或設置 obfs")
	}

	// L6: 參數校驗。
	if cfg.MTU < 1280 || cfg.MTU > 9000 {
		return nil, warns, fmt.Errorf("mtu 需在 1280~9000 之間，收到 %d", cfg.MTU)
	}
	if cfg.SndWnd < 32 || cfg.SndWnd > 1<<20 {
		return nil, warns, fmt.Errorf("sndwnd 需在 32~1048576 之間，收到 %d", cfg.SndWnd)
	}
	if cfg.RcvWnd < 32 || cfg.RcvWnd > 1<<20 {
		return nil, warns, fmt.Errorf("rcvwnd 需在 32~1048576 之間，收到 %d", cfg.RcvWnd)
	}
	if cfg.FECData < 0 || cfg.FECParity < 0 || (cfg.FECData == 0) != (cfg.FECParity == 0) {
		return nil, warns, fmt.Errorf("fec-d/fec-p 需同時為 0（停用）或均 ≥1，收到 %d/%d", cfg.FECData, cfg.FECParity)
	}
	if cfg.PadMax < 0 || cfg.PadMax > 255 {
		return nil, warns, fmt.Errorf("pad-max 需在 0~255 之間，收到 %d", cfg.PadMax)
	}
	if cfg.MaxClients < 1 {
		return nil, warns, fmt.Errorf("max-clients 需 ≥1，收到 %d", cfg.MaxClients)
	}
	if cfg.MaxRate < 0 {
		return nil, warns, fmt.Errorf("max-rate 需 ≥0（0=不限），收到 %d", cfg.MaxRate)
	}

	// 派生：網段。
	v4Net, v6Net, maxHosts, err := parseNetworks(cfg.V4NetStr, cfg.V6NetStr)
	if err != nil {
		return nil, warns, fmt.Errorf("網段配置錯誤: %v", err)
	}
	v4MaskSize, _ := v4Net.Mask.Size()
	v6MaskSize := 0
	if v6Net != nil {
		v6MaskSize, _ = v6Net.Mask.Size()
	}

	// 派生：密鑰 + 加密塊。
	key := deriveKey(cfg.PSK)
	block, err := newBlock(cfg.Crypt, key)
	if err != nil {
		return nil, warns, err
	}
	if cfg.Crypt == "none" {
		warns = append(warns, "⚠️ 目前處於無加密測試模式 (crypt=none)")
	}

	// 派生：KCP MTU。
	kcpMTU := computeKCPMTU(cfg.MTU, getObfsHeaderLen(cfg.ObfsMode), cfg.PadMax, cfg.Crypt, cfg.FECData > 0)

	cfg.V4Net = v4Net
	cfg.V6Net = v6Net
	cfg.MaxHosts = maxHosts
	cfg.ServerV4 = getIP4ByOffset(v4Net, 1)
	cfg.ServerV6 = getIP6ByOffset(v6Net, 1)
	cfg.V4Mask = v4MaskSize
	cfg.V6Mask = v6MaskSize
	cfg.Key = key
	cfg.Block = block
	cfg.KCPMTU = kcpMTU
	cfg.MaxFrameLen = cfg.MTU + 32

	return cfg, warns, nil
}

// newBlock 根據 crypt 模式與密鑰創建 KCP 加密塊。
func newBlock(crypt string, key []byte) (kcp.BlockCrypt, error) {
	switch crypt {
	case "aes-gcm", "aesgcm":
		block, err := kcp.NewAESGCMCrypt(key)
		if err != nil {
			return nil, fmt.Errorf("初始化 AES-GCM 加密失敗: %v", err)
		}
		return block, nil
	case "aes":
		block, _ := kcp.NewAESBlockCrypt(key)
		return block, nil
	case "salsa20":
		block, _ := kcp.NewSalsa20BlockCrypt(key)
		return block, nil
	case "none":
		return nil, nil
	default:
		return nil, fmt.Errorf("無效的 crypt: %q（有效: aes-gcm, salsa20, aes, none）", crypt)
	}
}

func initLogger(levelStr string) {
	level := zap.InfoLevel
	switch strings.ToLower(levelStr) {
	case "debug":
		level = zap.DebugLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("初始化 Zap 日誌失敗: %v", err))
	}
	sugar = logger.Sugar()
}

// deriveKey 用 PBKDF2 從 PSK 派生 32 字節密鑰（S3：加鹽 + 多次迭代，提升暴力破解成本）。
func deriveKey(psk string) []byte {
	return pbkdf2.Key([]byte(psk), kdfSalt, 40960, 32, sha256.New)
}

// cryptoOverhead 返回各加密模式每數據報的額外開銷（用於 MTU 推算）。
func cryptoOverhead(mode string) int {
	switch strings.ToLower(mode) {
	case "aes-gcm", "aesgcm":
		return 12 + 16 // AEAD: nonce(12) + tag(16)
	case "aes", "salsa20":
		return 20 // CFB: nonce(16) + crc(4)
	default:
		return 0
	}
}

// computeKCPMTU 根據隧道 MTU 派生 KCP 數據報大小。
//
// 目標：儘量讓完整以太網幀放進單個 KCP 段，同時保證外層（IP+UDP+obfs+填充）
// 不超過 1500 字節物理 MTU，避免公網分片。
func computeKCPMTU(tunnelMTU, obfsHdrLen, padMax int, cryptMode string, fecEnabled bool) int {
	innerFrame := tunnelMTU + 14 // 以太網幀（含 14B 頭）
	frameHdr := 3                // 幀頭: type(1B) + 長度(2B)
	cryptoHdr := cryptoOverhead(cryptMode)
	kcpHdr := 24
	fecHdr := 0
	if fecEnabled {
		fecHdr = 8
	}

	ideal := cryptoHdr + kcpHdr + fecHdr + frameHdr + innerFrame

	// 外層不分片上限：物理1500 - IP20 - UDP8 - obfs頭 - padLen(1) - padMax。
	noFrag := noFragUDPPayload - obfsHdrLen - 1 - padMax

	kcpMTU := ideal
	if kcpMTU > noFrag {
		kcpMTU = noFrag
	}
	if kcpMTU < 576 {
		kcpMTU = 576
	}
	if kcpMTU > 1500 {
		kcpMTU = 1500 // kcp-go 內部上限
	}
	return kcpMTU
}

func main() {
	var (
		showVersion  = flag.Bool("version", false, "顯示版本資訊")
		configPath   = flag.String("config", "config.json", "配置文件路徑 (JSON)")
		restartChild = flag.Bool("restart-child", false, "自動重啟子進程標記（內部使用）")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("kcpvpn version: %s\n", computeVersion())
		os.Exit(0)
	}

	globalConfigPath = *configPath

	// 自動重啟的子進程：給舊進程時間釋放端口/TAP 後再初始化。
	if *restartChild {
		time.Sleep(1500 * time.Millisecond)
	}

	cfg, warns, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌ 配置錯誤:", err)
		os.Exit(1)
	}

	initLogger(cfg.LogLevel)
	defer sugar.Sync()
	for _, w := range warns {
		sugar.Warnf("%s", w)
	}

	iface, err := createTAP(cfg.TapName)
	if err != nil {
		sugar.Fatalf("創建 TAP 網卡失敗: %v", err)
	}

	sugar.Infof("🔐 使用加密: %s, 偽裝: %s, 隧道MTU: %d, KCP MTU: %d", cfg.Crypt, cfg.ObfsMode, cfg.MTU, cfg.KCPMTU)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if cfg.Mode == "server" {
		serverV4CIDR := fmt.Sprintf("%s/%d", cfg.ServerV4, cfg.V4Mask)
		serverV6CIDR := ""
		if cfg.V6Net != nil {
			serverV6CIDR = fmt.Sprintf("%s/%d", cfg.ServerV6, cfg.V6Mask)
		}
		if err := setupInterface(iface.Name(), serverV4CIDR, serverV6CIDR, cfg.MTU); err != nil {
			sugar.Fatalf("伺服器網卡配置失敗: %v", err)
		}
		sugar.Infof("網段容量: %d 個可用客戶端 IP", cfg.MaxHosts-1)
		runServer(iface, cfg, sigCh)
	} else {
		runClient(iface, cfg, sigCh)
	}
}
