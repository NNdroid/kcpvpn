package main

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed webui/*
var webFS embed.FS

// webPort 从 web-addr（"port"、":port"、"host:port"）提取端口。
func webPort(addr string) (int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// 纯端口形式 "8080"。
		if p, err2 := strconv.Atoi(addr); err2 == nil {
			return p, nil
		}
		return 0, fmt.Errorf("無法解析 web-addr %q: %v", addr, err)
	}
	_ = host
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("無效端口 %q", portStr)
	}
	return p, nil
}

// resolveBindAddrs 根据 bind 模式返回要监听的 IP 列表。
// getTunnelIPs 提供 tunnel IP（服务端返回 ServerV4/ServerV6，客户端返回分配的隧道 IP）。
func resolveBindAddrs(cfg *Config, getTunnelIPs func() []string) []string {
	switch cfg.WebBind {
	case "tunnel":
		ips := getTunnelIPs()
		if len(ips) == 0 {
			return []string{"127.0.0.1"}
		}
		return ips
	case "public":
		return []string{"0.0.0.0", "::"}
	default: // local
		return []string{"127.0.0.1", "::1"}
	}
}

// webUI 保存 Web 管理界面的运行时依赖。
type webUI struct {
	cfg         *Config
	tapName     string
	ipam        *IPAM
	vswitch     *VSwitch
	clientState *clientState
	startedAt   time.Time
	configPath  string
	mode        string
	sampler     *trafficSampler
}

// startWebUI 启动 HTTP 管理界面，按 bind 绑定所有监听地址，共享同一 mux。
// 返回 error 表示启动失败（调用方应记录）。quitCh 关闭时优雅关闭。
func startWebUI(cfg *Config, tapName string, ipam *IPAM, vswitch *VSwitch, state *clientState, quitCh chan struct{}) error {
	port, err := webPort(cfg.WebAddr)
	if err != nil {
		return err
	}

	var getTunnel func() []string
	if cfg.Mode == "server" {
		getTunnel = func() []string {
			ips := []string{cfg.ServerV4}
			if cfg.ServerV6 != "" {
				ips = append(ips, cfg.ServerV6)
			}
			return ips
		}
	} else {
		getTunnel = func() []string {
			if state != nil {
				s := state.snapshot()
				var ips []string
				if s.IPv4 != "" {
					ips = append(ips, s.IPv4)
				}
				if s.IPv6 != "" {
					ips = append(ips, s.IPv6)
				}
				return ips
			}
			return nil
		}
	}

	addrIPs := resolveBindAddrs(cfg, getTunnel)

	ui := &webUI{
		cfg:         cfg,
		tapName:     tapName,
		ipam:        ipam,
		vswitch:     vswitch,
		clientState: state,
		startedAt:   time.Now(),
		configPath:  globalConfigPath,
		mode:        cfg.Mode,
	}

	// 时间序列采样器（1s，保留 180 点 = 3 分钟），供曲线图轮询。
	// 数据常驻内存，Web UI 重新打开（进程未重启）时仍能拉到最近 3 分钟曲线。
	ui.sampler = newTrafficSampler(cfg.Mode, ipam, vswitch, state, 180)
	go ui.sampler.run(quitCh)

	mux := http.NewServeMux()
	// 静态资源：以嵌入的 webui/ 為根，/ → index.html、/style.css、/app.js、/chart.umd.js 自動提供。
	webRoot, err := fs.Sub(webFS, "webui")
	if err != nil {
		return fmt.Errorf("webui sub fs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webRoot)))
	mux.Handle("/api/status", ui.auth(http.HandlerFunc(ui.handleStatus)))
	mux.Handle("/api/config", ui.auth(http.HandlerFunc(ui.handleConfig)))
	mux.Handle("/api/kick", ui.auth(http.HandlerFunc(ui.handleKick)))
	mux.Handle("/api/chart", ui.auth(http.HandlerFunc(ui.handleChart)))
	mux.Handle("/api/events", ui.auth(http.HandlerFunc(ui.handleEvents)))

	servers := make([]*http.Server, 0, len(addrIPs))
	for _, ip := range addrIPs {
		listenAddr := net.JoinHostPort(ip, strconv.Itoa(port))
		ln, err := net.Listen("tcp", listenAddr)
		if err != nil {
			// 单个地址失败不致命（如无 IPv6），记录后继续其它地址。
			sugar.Warnf("⚠️ Web UI 監聽 %s 失敗: %v", listenAddr, err)
			continue
		}
		srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, srv)
		go func(s *http.Server, l net.Listener, addr string) {
			if err := s.Serve(l); err != nil && err != http.ErrServerClosed {
				sugar.Warnf("Web UI 伺服器 %s 異常: %v", addr, err)
			}
		}(srv, ln, listenAddr)
		sugar.Infof("🌐 Web UI 監聽: http://%s (bind=%s)", listenAddr, cfg.WebBind)
	}
	if len(servers) == 0 {
		return fmt.Errorf("Web UI 無任何可綁定的地址 (bind=%s, port=%d)", cfg.WebBind, port)
	}

	// 优雅关闭。
	go func() {
		<-quitCh
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, s := range servers {
			s.Shutdown(ctx)
		}
	}()

	return nil
}

// auth 校验 Bearer token。token 为空且非 public 时放行（本机调试便利）。
// 同时接受 ?token=<value> query（EventSource 无法携带自定义 header）。
func (u *webUI) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := u.cfg.WebToken
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == "" {
			provided = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kcpvpn"`)
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleStatus 返回运行状态（服务端：客户端列表 + 交换机统计；客户端：隧道状态）。
func (u *webUI) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, u.statusPayload())
}

func (u *webUI) statusPayload() map[string]interface{} {
	payload := map[string]interface{}{
		"mode":     u.mode,
		"version":  version,
		"uptime_s": int(time.Since(u.startedAt).Seconds()),
		"tap":      u.tapName,
	}
	if u.mode == "server" {
		clients := []ClientInfo{}
		if u.ipam != nil {
			clients = u.ipam.Clients()
		}
		payload["clients"] = clients
		payload["max_clients"] = u.cfg.MaxClients
		if u.vswitch != nil {
			payload["vswitch_drops"] = u.vswitch.Drops()
		}
	} else if u.clientState != nil {
		payload["client"] = u.clientState.snapshot()
	}
	return payload
}

// handleConfig 返回脱敏配置，或接收新配置进行校验+持久化+重启。
func (u *webUI) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, sanitizeConfig(u.cfg))
	case http.MethodPut:
		var raw map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 忽略 _comment 等非字段键。
		delete(raw, "_comment")
		// 脱敏字段还原：前端提交 "***" 表示未修改，映射回当前真实值。
		if raw["psk"] == "***" {
			raw["psk"] = u.cfg.PSK
		}
		if raw["web-token"] == "***" {
			raw["web-token"] = u.cfg.WebToken
		}

		// 序列化为临时文件，复用 loadConfig 的全部校验。
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("kcpvpn-cfg-%d.json", time.Now().UnixNano()))
		tmpData, err := json.Marshal(raw)
		if err != nil {
			http.Error(w, `{"error":"marshal failed"}`, http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(tmpPath, tmpData, 0o600); err != nil {
			http.Error(w, `{"error":"temp write failed"}`, http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmpPath)

		newCfg, warns, err := loadConfig(tmpPath)
		if err != nil {
			writeJSON(w, map[string]interface{}{"error": err.Error(), "warns": warns})
			return
		}
		// 禁止运行模式切换（server<->client）以免破坏当前进程。
		if newCfg.Mode != u.cfg.Mode {
			writeJSON(w, map[string]interface{}{"error": "mode 不可在运行时切换（需重启进程）"})
			return
		}

		// 原子写回 config.json。
		target := u.configPath
		if target == "" {
			target = "config.json"
		}
		if err := atomicWriteConfig(target, raw); err != nil {
			writeJSON(w, map[string]interface{}{"error": fmt.Sprintf("寫入配置失敗: %v", err)})
			return
		}

		writeJSON(w, map[string]interface{}{"ok": true, "restarting": true, "warns": warns})
		sugar.Infof("🌐 Web UI 已更新配置，觸發自動重啟...")
		go restartProcess(target)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleKick 断开指定客户端（仅服务端）。
func (u *webUI) handleKick(w http.ResponseWriter, r *http.Request) {
	if u.mode != "server" || u.ipam == nil {
		http.Error(w, `{"error":"not supported"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Offset uint32 `json:"offset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if u.ipam.Kick(req.Offset) {
		writeJSON(w, map[string]interface{}{"ok": true})
	} else {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "client not found"})
	}
}

// sanitizeConfig 脱敏敏感字段后返回配置（psk/web-token）。
func sanitizeConfig(cfg *Config) map[string]interface{} {
	b, _ := json.Marshal(cfg)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["psk"]; ok {
		m["psk"] = "***"
	}
	if _, ok := m["web-token"]; ok {
		m["web-token"] = "***"
	}
	return m
}

// atomicWriteConfig 原子写配置文件（临时文件 + rename）。
func atomicWriteConfig(target string, raw map[string]interface{}) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// restartProcess 重新 spawn 自身（带 -restart-child），旧进程稍后退出释放端口/TAP。
func restartProcess(configPath string) {
	if configPath == "" {
		configPath = "config.json"
	}
	exe, err := os.Executable()
	if err != nil {
		sugar.Errorf("自動重啟失敗（無法獲取可執行文件）: %v", err)
		return
	}
	cmd := exec.Command(exe, "-config", configPath, "-restart-child")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		sugar.Errorf("自動重啟失敗（啟動子進程）: %v", err)
		return
	}
	// 給子進程時間啟動，再退出當前進程釋放端口/TAP。
	go func() {
		time.Sleep(1500 * time.Millisecond)
		os.Exit(0)
	}()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// chartSeries 是曲线图的单条时间序列。
type chartSeries struct {
	Labels    []int64   `json:"labels"`      // unix 毫秒
	Clients   []float64 `json:"clients"`     // 在线客户端数
	RxBps     []float64 `json:"rx_bps"`      // 入站速率 B/s
	TxBps     []float64 `json:"tx_bps"`      // 出站速率 B/s
	DropsPerS []float64 `json:"drops_per_s"` // 丢弃速率 帧/s
}

// trafficSampler 周期采样流量/在线数/丢弃率，保留固定容量的环形时间序列。
type trafficSampler struct {
	mu    sync.Mutex
	cap   int
	mode  string
	ipam  *IPAM
	vsw   *VSwitch
	state *clientState

	labels    []int64
	clients   []float64
	rxBps     []float64
	txBps     []float64
	dropsPerS []float64

	// 上一 tick 的累计值（用于差分速率）。
	lastRx, lastTx, lastDrops uint64
	lastTime                  time.Time
}

func newTrafficSampler(mode string, ipam *IPAM, vsw *VSwitch, state *clientState, cap int) *trafficSampler {
	return &trafficSampler{
		mu:    sync.Mutex{},
		cap:   cap,
		mode:  mode,
		ipam:  ipam,
		vsw:   vsw,
		state: state,
	}
}

// run 每秒采样一次，直到 quitCh 关闭。
func (s *trafficSampler) run(quitCh chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-quitCh:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

// tick 采样一个时间点并写入环形 buffer。
func (s *trafficSampler) tick(now time.Time) {
	var rx, tx, drops uint64
	var clients float64

	if s.mode == "server" {
		if s.ipam != nil {
			clients = float64(s.ipam.Count())
		}
		if s.vsw != nil {
			t := s.vsw.Traffic()
			rx, tx = t.RxBytes, t.TxBytes
			drops = s.vsw.Drops()
		}
	} else if s.state != nil {
		t := s.state.trafficSnapshot()
		rx, tx = t.RxBytes, t.TxBytes
		clients = boolToFloat(s.state.snapshot().Up)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	first := s.lastTime.IsZero()
	if first {
		s.lastTime = now
	}
	dt := now.Sub(s.lastTime).Seconds()
	if dt <= 0 {
		dt = 1
	}

	// 首个采样点只建立基线（速率为 0），后续按差分计算。
	var rxBps, txBps, dropsPerS float64
	if !first {
		rxBps = float64(diffU64(rx, s.lastRx)) / dt
		txBps = float64(diffU64(tx, s.lastTx)) / dt
		dropsPerS = float64(diffU64(drops, s.lastDrops)) / dt
	}

	s.labels = appendRing(s.labels, now.UnixMilli(), s.cap)
	s.clients = appendRing(s.clients, clients, s.cap)
	s.rxBps = appendRing(s.rxBps, rxBps, s.cap)
	s.txBps = appendRing(s.txBps, txBps, s.cap)
	s.dropsPerS = appendRing(s.dropsPerS, dropsPerS, s.cap)

	s.lastRx, s.lastTx, s.lastDrops = rx, tx, drops
	s.lastTime = now
}

// snapshot 返回当前时间序列副本。
func (s *trafficSampler) snapshot() chartSeries {
	s.mu.Lock()
	defer s.mu.Unlock()
	return chartSeries{
		Labels:    append([]int64(nil), s.labels...),
		Clients:   append([]float64(nil), s.clients...),
		RxBps:     append([]float64(nil), s.rxBps...),
		TxBps:     append([]float64(nil), s.txBps...),
		DropsPerS: append([]float64(nil), s.dropsPerS...),
	}
}

// appendRing 追加 v 到 slice，超容时丢弃最旧元素。
func appendRing[T any](s []T, v T, cap int) []T {
	s = append(s, v)
	if len(s) > cap {
		s = s[len(s)-cap:]
	}
	return s
}

func diffU64(cur, prev uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return 0 // 计数器重置（重启）时回退为 0
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// handleChart 返回时间序列数据供曲线图轮询。
func (u *webUI) handleChart(w http.ResponseWriter, r *http.Request) {
	if u.sampler == nil {
		writeJSON(w, chartSeries{Labels: []int64{}})
		return
	}
	writeJSON(w, u.sampler.snapshot())
}

// handleEvents 通过 SSE 推送状态/配置/图表数据，替代前端轮询。
// 连接建立后立即推一个 snapshot（status+config+chart），随后每 ~1s 推 chart 与 status。
func (u *webUI) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming unsupported"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 序列化辅助：event 名 + data JSON。
	send := func(event string, v interface{}) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	// 连接后立即推送全量快照。
	chart := func() chartSeries {
		if u.sampler == nil {
			return chartSeries{Labels: []int64{}}
		}
		return u.sampler.snapshot()
	}
	if err := send("snapshot", map[string]interface{}{
		"status": u.statusPayload(),
		"config": sanitizeConfig(u.cfg),
		"chart":  chart(),
	}); err != nil {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := send("chart", chart()); err != nil {
				return
			}
			if err := send("status", u.statusPayload()); err != nil {
				return
			}
		}
	}
}
