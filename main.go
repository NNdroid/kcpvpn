package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/xtaci/kcp-go/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	sugar   *zap.SugaredLogger
	version = "dev"
)

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

func getMacAddr(name string, specified string) string {
	if specified != "" {
		if _, err := net.ParseMAC(specified); err == nil {
			return specified
		}
		sugar.Warnf("⚠️ 指定的 MAC (%s) 格式錯誤", specified)
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "02:00:00:00:00:01"
	}
	return iface.HardwareAddr.String()
}

func main() {
	showVersion := flag.Bool("v", false, "顯示版本資訊")
	mode := flag.String("mode", "server", "server/client")
	addrStr := flag.String("addr", "127.0.0.1:4000", "listen/dial address")
	tapName := flag.String("tap", "tap0", "tap interface name")
	v4NetStr := flag.String("v4net", "10.0.0.0/24", "IPv4 CIDR (e.g. 10.8.1.0/22)")
	v6NetStr := flag.String("v6net", "fd00::/64", "IPv6 CIDR (e.g. fcee:1232:3334:4556::/96)")
	fecData := flag.Int("fec-d", 5, "FEC Data Shards")
	fecParity := flag.Int("fec-p", 2, "FEC Parity Shards")

	clientIP4 := flag.String("ip4", "", "Client specify IPv4")
	clientIP6 := flag.String("ip6", "", "Client specify IPv6")
	clientMAC := flag.String("mac", "", "Client specify MAC address")

	cryptMode := flag.String("crypt", "salsa20", "加密方式: salsa20, aes, none")
	fwmark := flag.Int("fwmark", 0, "設定 fwmark 啟用策略路由 (例如 1911)")
	logLevel := flag.String("loglevel", "warn", "日誌級別: debug, info, warn, error")
	psk := flag.String("psk", "HereIsYourSuperSecretPassword", "預共享密鑰")

	mtu := flag.Int("mtu", 1350, "KCP Maximum Transmission Unit (MTU)")
	sndwnd := flag.Int("sndwnd", 4096, "KCP send window size")
	rcvwnd := flag.Int("rcvwnd", 4096, "KCP receive window size")
	dscp := flag.Int("dscp", 0, "DSCP (Differentiated Services Code Point)")

	obfsMode := flag.String("obfs", "none", "流量偽裝模式: none, xor, stun, dns, dtls, quic")
	padMax := flag.Int("pad-max", 0, "數據包最大隨機填充位元組數 (0 表示禁用)")

	flag.Parse()

	if *showVersion {
		fmt.Printf("kcpvpn version: %s\n", version)
		os.Exit(0)
	}

	initLogger(*logLevel)
	defer sugar.Sync()

	obfs := strings.ToLower(*obfsMode)
	switch obfs {
	case "none", "xor", "stun", "dns", "dtls", "quic":
	default:
		sugar.Fatalf("❌ 未知的偽裝模式 '%s'，有效選項: none, xor, stun, dns, dtls, quic", *obfsMode)
	}

	iface, err := createTAP(*tapName)
	if err != nil {
		sugar.Fatalf("創建 TAP 網卡失敗: %v", err)
	}

	key := sha256.Sum256([]byte(*psk))
	var block kcp.BlockCrypt
	switch strings.ToLower(*cryptMode) {
	case "salsa20":
		block, _ = kcp.NewSalsa20BlockCrypt(key[:])
	case "aes":
		block, _ = kcp.NewAESBlockCrypt(key[:])
	case "none":
		block = nil
		sugar.Warnf("⚠️ 警告：目前處於無加密測試模式 (-crypt none)")
	default:
		sugar.Fatalf("❌ 未知的加密模式 '%s'，有效選項: salsa20, aes, none", *cryptMode)
	}

	v4Net, v6Net, maxHosts, err := parseNetworks(*v4NetStr, *v6NetStr)
	if err != nil {
		sugar.Fatalf("網段配置錯誤: %v", err)
	}

	v4MaskSize, _ := v4Net.Mask.Size()
	v6MaskSize := 0
	if v6Net != nil {
		v6MaskSize, _ = v6Net.Mask.Size()
	}

	serverV4 := getIP4ByOffset(v4Net, 1)
	serverV6 := getIP6ByOffset(v6Net, 1)

	serverV4CIDR := fmt.Sprintf("%s/%d", serverV4, v4MaskSize)
	serverV6CIDR := ""
	if v6Net != nil {
		serverV6CIDR = fmt.Sprintf("%s/%d", serverV6, v6MaskSize)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *mode == "server" {
		if err := setupInterface(iface.Name(), serverV4CIDR, serverV6CIDR); err != nil {
			sugar.Fatalf("伺服器網卡配置失敗: %v", err)
		}
		sugar.Infof("網段容量: %d 個可用客戶端 IP", maxHosts-1)

		go func() {
			<-sigCh
			sugar.Infof("🛑 收到關閉訊號，伺服器準備退出...")
			os.Exit(0)
		}()

		runServer(iface, *addrStr, v4Net, v6Net, maxHosts, serverV4, serverV6, v4MaskSize, v6MaskSize, block, *fecData, *fecParity, *mtu, *sndwnd, *rcvwnd, *dscp, obfs, key[:], *padMax)
	} else {
		runClient(iface, *addrStr, block, *fecData, *fecParity, *clientIP4, *clientIP6, *clientMAC, *fwmark, sigCh, *mtu, *sndwnd, *rcvwnd, *dscp, obfs, key[:], *padMax)
	}
}
