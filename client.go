package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
)

// clientState 保存客戶端隧道運行時狀態，供 Web UI 讀取。
// 字段由 runClient 在握手/重連等階段更新，讀寫由 mutex 保護。
type clientState struct {
	mu         sync.Mutex
	up         bool
	ipv4       string
	ipv6       string
	mac        string
	gatewayV4  string
	gatewayV6  string
	lastError  string
	reconnects uint64
	startedAt  time.Time

	// 流量計數（字節），由數據循環累加。
	rxBytes atomic.Uint64 // 服務端 → TAP（入站）
	txBytes atomic.Uint64 // TAP → 服務端（出站）
}

// setUp 更新隧道建立狀態與地址信息。
func (s *clientState) setUp(up bool, resp *HandshakeMsg, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.up = up
	if resp != nil {
		s.ipv4 = resp.IPv4
		s.ipv6 = resp.IPv6
		s.mac = resp.MAC
		s.gatewayV4 = resp.GatewayV4
		s.gatewayV6 = resp.GatewayV6
	}
	s.lastError = lastErr
}

// setError 記錄最近一次錯誤（隧道未建立時）。
func (s *clientState) setError(errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = errMsg
}

// incReconnects 遞增重連計數。
func (s *clientState) incReconnects() {
	atomic.AddUint64(&s.reconnects, 1)
}

// snapshot 返回狀態快照。
func (s *clientState) snapshot() clientStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clientStatus{
		Up:         s.up,
		IPv4:       s.ipv4,
		IPv6:       s.ipv6,
		MAC:        s.mac,
		GatewayV4:  s.gatewayV4,
		GatewayV6:  s.gatewayV6,
		LastError:  s.lastError,
		Reconnects: atomic.LoadUint64(&s.reconnects),
		StartedAt:  s.startedAt,
	}
}

// trafficSnapshot 返回累計收發字節（供 Web UI 曲線採樣）。
func (s *clientState) trafficSnapshot() TrafficStats {
	return TrafficStats{
		RxBytes: s.rxBytes.Load(),
		TxBytes: s.txBytes.Load(),
	}
}

// clientStatus 是對外暴露的客戶端狀態快照（JSON）。
type clientStatus struct {
	Up         bool      `json:"up"`
	IPv4       string    `json:"ipv4,omitempty"`
	IPv6       string    `json:"ipv6,omitempty"`
	MAC        string    `json:"mac,omitempty"`
	GatewayV4  string    `json:"gateway_v4,omitempty"`
	GatewayV6  string    `json:"gateway_v6,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
	Reconnects uint64    `json:"reconnects"`
	StartedAt  time.Time `json:"started_at"`
}

func runClient(tap *water.Interface, cfg *Config, sigCh chan os.Signal) {
	state := &clientState{startedAt: time.Now()}
	defer state.setUp(false, nil, "process exit")
	tapChan := make(chan []byte, 100)

	// TAP 讀循環：讀錯誤不退出進程，短暫退避後繼續（L2）。
	go func() {
		buf := make([]byte, cfg.MTU+256)
		for {
			n, err := tap.Read(buf)
			if err != nil {
				sugar.Warnf("TAP 讀取錯誤: %v", err)
				time.Sleep(200 * time.Millisecond)
				continue
			}
			packet := make([]byte, n)
			copy(packet, buf[:n])
			select {
			case tapChan <- packet:
			default:
			}
		}
	}()

	quitCh := make(chan struct{})
	go func() {
		<-sigCh
		sugar.Infof("🛑 收到關閉訊號，程式清理退出...")
		close(quitCh)
	}()

	// Web 管理界面（按配置启停，失败不致命）。
	if cfg.WebEnabled {
		go func() {
			if err := startWebUI(cfg, tap.Name(), nil, nil, state, quitCh); err != nil {
				sugar.Warnf("⚠️ Web UI 啟動失敗: %v", err)
			}
		}()
	}

	for {
		select {
		case <-quitCh:
			return
		default:
		}

		sugar.Infof("正在連接伺服器 %s...", cfg.Addr)

		kcpConn, err := dialKCP(cfg)
		if err != nil {
			state.setError(fmt.Sprintf("連接失敗: %v", err))
			state.incReconnects()
			sugar.Warnf("連接失敗: %v，3秒後重試...", err)
			if !sleepOrQuit(3*time.Second, quitCh) {
				return
			}
			continue
		}

		kcpConn.SetWindowSize(cfg.SndWnd, cfg.RcvWnd)
		kcpConn.SetNoDelay(1, 10, 2, 1)
		if !kcpConn.SetMtu(cfg.KCPMTU) {
			sugar.Warnf("⚠️ SetMtu(%d) 無效，回退默認", cfg.KCPMTU)
		}

		// ② 握手帶 nonce + 時間戳防重放；mac 改為「請求特定 MAC」（可選）。
		reqData, _ := json.Marshal(HandshakeMsg{
			Type:  "request",
			MAC:   cfg.ReqMac, // 可選：請求特定 MAC；空則由服務端生成
			IPv4:  cfg.ReqV4,
			IPv6:  cfg.ReqV6,
			Nonce: genNonceHex(),
			Ts:    time.Now().Unix(),
		})

		if err := writeFrame(kcpConn, frameTypeHandshake, reqData); err != nil {
			kcpConn.Close()
			continue
		}

		kcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		ft, respBytes, err := readFrame(kcpConn, cfg.MaxFrameLen)
		kcpConn.SetReadDeadline(time.Time{})

		if err != nil || ft != frameTypeHandshake {
			state.setError(fmt.Sprintf("握手失敗: %v", err))
			state.incReconnects()
			sugar.Warnf("❌ 握手失敗: %v", err)
			kcpConn.Close()
			if !sleepOrQuit(3*time.Second, quitCh) {
				return
			}
			continue
		}

		var resp HandshakeMsg
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			state.setError(fmt.Sprintf("握手響應解析失敗: %v", err))
			sugar.Warnf("❌ 握手響應解析失敗: %v", err)
			kcpConn.Close()
			continue
		}
		if err := validateServerResp(&resp, cfg); err != nil {
			state.setError(fmt.Sprintf("伺服器回應無效: %v", err))
			state.incReconnects()
			sugar.Warnf("❌ 伺服器回應無效: %v", err)
			kcpConn.Close()
			if !sleepOrQuit(3*time.Second, quitCh) {
				return
			}
			continue
		}

		clientV4CIDR := fmt.Sprintf("%s/%d", resp.IPv4, resp.IPv4Mask)
		clientV6CIDR := ""
		if resp.IPv6 != "" {
			clientV6CIDR = fmt.Sprintf("%s/%d", resp.IPv6, resp.IPv6Mask)
		}

		// ③ 服務端下發 MAC，配置網卡使真實 MAC 與註冊一致（S5 前提）。
		if resp.MAC != "" && validMAC(resp.MAC) {
			if err := setInterfaceMAC(tap.Name(), resp.MAC); err != nil {
				sugar.Warnf("⚠️ 設置網卡 MAC (%s) 失敗: %v", resp.MAC, err)
			}
		} else {
			sugar.Warnf("⚠️ 伺服器未下發有效 MAC (%q)", resp.MAC)
		}

		if err := setupInterface(tap.Name(), clientV4CIDR, clientV6CIDR, cfg.MTU); err != nil {
			sugar.Warnf("⚠️ 客戶端網卡配置警告: %v", err)
		}
		state.setUp(true, &resp, "")
		sugar.Infof("✅ 隧道建立！IP: %s, 網關: %s, MAC: %s", clientV4CIDR, resp.GatewayV4, resp.MAC)

		setupPolicyRouting(tap.Name(), cfg.FWMark, resp.GatewayV4, resp.GatewayV6)

		dropChan := make(chan struct{})

		// 雙向 keepalive：每 10 秒發送 keepalive 幀（①：獨立類型 0x02）。
		go func(c net.Conn) {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-dropChan:
					return
				case <-ticker.C:
					if err := writeFrame(c, frameTypeKeepalive, nil); err != nil {
						return
					}
				}
			}
		}(kcpConn)

		// 讀循環：把伺服器下發的幀寫入 TAP（①：只接受 0x03 數據 + 0x02 keepalive）。
		go func(c net.Conn) {
			defer close(dropChan)
			for {
				c.SetReadDeadline(time.Now().Add(30 * time.Second))
				ft, data, err := readFrame(c, cfg.MaxFrameLen)
				if err != nil {
					state.setUp(false, nil, fmt.Sprintf("失去連線: %v", err))
					state.incReconnects()
					sugar.Warnf("⚠️ 失去連線: %v", err)
					return
				}
				switch ft {
				case frameTypeKeepalive:
					continue
				case frameTypeHandshake:
					sugar.Warnf("⚠️ 數據面收到握手幀，斷開")
					return
				case frameTypeEthernet:
					state.rxBytes.Add(uint64(len(data)))
					tap.Write(data)
				}
			}
		}(kcpConn)

	NetworkLoop:
		for {
			select {
			case <-quitCh:
				cleanPolicyRouting(tap.Name(), cfg.FWMark, resp.GatewayV4, resp.GatewayV6)
				kcpConn.Close()
				return
			case <-dropChan:
				break NetworkLoop
			case packet := <-tapChan:
				state.txBytes.Add(uint64(len(packet)))
				if err := writeFrame(kcpConn, frameTypeEthernet, packet); err != nil {
					break NetworkLoop
				}
			}
		}

		cleanPolicyRouting(tap.Name(), cfg.FWMark, resp.GatewayV4, resp.GatewayV6)
		kcpConn.Close()
		if !sleepOrQuit(3*time.Second, quitCh) {
			return
		}
	}
}

// dialKCP 建立帶 obfs 包裝的 KCP 客戶端會話。
func dialKCP(cfg *Config) (*kcp.UDPSession, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", cfg.Addr)
	if err != nil {
		return nil, err
	}
	network := "udp4"
	if udpaddr.IP.To4() == nil {
		network = "udp"
	}
	raw, err := net.ListenUDP(network, nil)
	if err != nil {
		return nil, err
	}

	obfsConn := newObfsPacketConn(raw, cfg.ObfsMode, cfg.Key, cfg.PadMax)
	sess, err := kcp.NewConn2(udpaddr, cfg.Block, cfg.FECData, cfg.FECParity, obfsConn)
	if err != nil {
		raw.Close()
		return nil, err
	}

	// 經由 obfs 包裝器透傳到底層 socket。
	if err := sess.SetReadBuffer(4 * 1024 * 1024); err != nil {
		sugar.Warnf("SetReadBuffer: %v", err)
	}
	if err := sess.SetWriteBuffer(4 * 1024 * 1024); err != nil {
		sugar.Warnf("SetWriteBuffer: %v", err)
	}
	if cfg.DSCP > 0 {
		if err := sess.SetDSCP(cfg.DSCP); err != nil {
			sugar.Warnf("SetDSCP: %v", err)
		}
	}
	return sess, nil
}

// validateServerResp 校驗伺服器握手響應（S7）。
func validateServerResp(resp *HandshakeMsg, cfg *Config) error {
	if resp.Type != "response" {
		return fmt.Errorf("回應類型錯誤: %q", resp.Type)
	}
	ip := net.ParseIP(resp.IPv4)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("無效的 IPv4: %q", resp.IPv4)
	}
	if !cfg.V4Net.Contains(ip) {
		return fmt.Errorf("IPv4 %s 不在網段 %s 內", resp.IPv4, cfg.V4Net.String())
	}
	if resp.IPv4Mask <= 0 || resp.IPv4Mask > 30 {
		return fmt.Errorf("無效的 IPv4 遮罩: %d", resp.IPv4Mask)
	}
	expectedGw := getIP4ByOffset(cfg.V4Net, 1)
	if resp.GatewayV4 != expectedGw {
		return fmt.Errorf("網關不匹配: %s != %s", resp.GatewayV4, expectedGw)
	}
	if resp.IPv4 == expectedGw {
		return fmt.Errorf("分配到的 IP %s 是網關地址", resp.IPv4)
	}
	return nil
}

// sleepOrQuit 等待 d 或 quitCh 關閉。返回 true 表示等待完整（繼續），
// false 表示已收到退出信號（調用方應返回）。
func sleepOrQuit(d time.Duration, quitCh <-chan struct{}) bool {
	select {
	case <-quitCh:
		return false
	case <-time.After(d):
		return true
	}
}
