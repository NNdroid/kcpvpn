package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
	"golang.org/x/time/rate"
)

// handshakeWindow 允許的握手時間戳偏差（②）。
const handshakeWindow = 300 * time.Second

// seenNonces 記錄最近處理過的握手 nonce，防止重放（②）。
// 使用 sync.Map + 時間戳，定期清理超過窗口的條目。
type seenNonces struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newSeenNonces() *seenNonces {
	return &seenNonces{seen: make(map[string]time.Time)}
}

// checkAndAdd 返回 false 表示 nonce 已被使用過（重放）。
func (s *seenNonces) checkAndAdd(nonce string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 清理過期條目（避免無限增長）。
	cutoff := now.Add(-handshakeWindow - time.Minute)
	for k, t := range s.seen {
		if t.Before(cutoff) {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[nonce]; ok {
		return false
	}
	s.seen[nonce] = now
	return true
}

func runServer(tap *water.Interface, cfg *Config, sigCh chan os.Signal) {
	udpaddr, err := net.ResolveUDPAddr("udp", cfg.Addr)
	if err != nil {
		sugar.Fatalf("解析監聽地址失敗: %v", err)
	}
	network := "udp4"
	if udpaddr.IP.To4() == nil {
		network = "udp"
	}
	raw, err := net.ListenUDP(network, udpaddr)
	if err != nil {
		sugar.Fatalf("UDP 監聽失敗: %v", err)
	}

	// S1: obfs 包裝器放在 KCP 之外。
	obfsConn := newObfsPacketConn(raw, cfg.ObfsMode, cfg.Key, cfg.PadMax)
	listener, err := kcp.ServeConn(cfg.Block, cfg.FECData, cfg.FECParity, obfsConn)
	if err != nil {
		raw.Close()
		sugar.Fatalf("KCP 監聽失敗: %v", err)
	}

	if cfg.DSCP > 0 {
		if err := listener.SetDSCP(cfg.DSCP); err != nil {
			sugar.Warnf("SetDSCP: %v", err)
		}
	}
	if err := listener.SetReadBuffer(4 * 1024 * 1024); err != nil {
		sugar.Warnf("SetReadBuffer: %v", err)
	}
	if err := listener.SetWriteBuffer(4 * 1024 * 1024); err != nil {
		sugar.Warnf("SetWriteBuffer: %v", err)
	}

	sugar.Infof("🚀 KCP VPN 伺服器運行中 (監聽: %s, 隧道MTU: %d, KCP MTU: %d, 偽裝模式: %s, 最大填充: %d B, 最大客戶端: %d, 速率限制: %d B/s)",
		cfg.Addr, cfg.MTU, cfg.KCPMTU, cfg.ObfsMode, cfg.PadMax, cfg.MaxClients, cfg.MaxRate)

	ipam := NewIPAM(cfg.V4Net, cfg.V6Net, cfg.MaxHosts)
	vswitch := NewVSwitch(tap)
	nonces := newSeenNonces()

	// S6: 並發連接數限制。
	sem := make(chan struct{}, cfg.MaxClients)

	quitCh := make(chan struct{})
	go func() {
		<-sigCh
		sugar.Infof("🛑 收到關閉訊號，伺服器準備退出...")
		close(quitCh)
		listener.Close() // 解除 AcceptKCP 阻塞
		raw.Close()
	}()

	// Web 管理界面（按配置启停，失败不致命）。
	if cfg.WebEnabled {
		go func() {
			if err := startWebUI(cfg, tap.Name(), ipam, vswitch, nil, quitCh); err != nil {
				sugar.Warnf("⚠️ Web UI 啟動失敗: %v", err)
			}
		}()
	}

	// TAP 讀循環（L4：錯誤記日誌並退避，避免忙轉）。
	go func() {
		buf := make([]byte, cfg.MTU+256)
		for {
			n, err := tap.Read(buf)
			if err != nil {
				sugar.Warnf("TAP 讀取錯誤: %v", err)
				select {
				case <-quitCh:
					return
				case <-time.After(500 * time.Millisecond):
				}
				continue
			}
			vswitch.Forward(buf[:n], nil)
		}
	}()

	for {
		conn, err := listener.AcceptKCP()
		if err != nil {
			select {
			case <-quitCh:
				sugar.Infof("伺服器已關閉")
				return
			default:
				continue
			}
		}

		select {
		case sem <- struct{}{}:
		default:
			sugar.Warnf("⚠️ 達到最大客戶端數 (%d)，拒絕新連線", cfg.MaxClients)
			conn.Close()
			continue
		}

		go func(kcpConn *kcp.UDPSession) {
			defer func() {
				<-sem
				if r := recover(); r != nil {
					sugar.Errorf("協程崩潰: %v", r)
				}
			}()

			kcpConn.SetWindowSize(cfg.SndWnd, cfg.RcvWnd)
			kcpConn.SetNoDelay(1, 10, 2, 1)
			if !kcpConn.SetMtu(cfg.KCPMTU) {
				sugar.Warnf("⚠️ SetMtu(%d) 無效", cfg.KCPMTU)
			}

			// ---- 握手階段（①：只接受 0x01 握手幀）----
			kcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			ft, reqBytes, err := readFrame(kcpConn, cfg.MaxFrameLen)
			kcpConn.SetReadDeadline(time.Time{})
			if err != nil || ft != frameTypeHandshake {
				kcpConn.Close()
				return
			}

			var req HandshakeMsg
			if err := json.Unmarshal(reqBytes, &req); err != nil || req.Type != "request" {
				kcpConn.Close()
				return
			}

			// ② 握手防重放：nonce + 時間窗。
			if err := validateHandshake(&req, nonces); err != nil {
				sugar.Warnf("⚠️ 握手校驗失敗 (%s): %v", req.MAC, err)
				kcpConn.Close()
				return
			}

			// ③ 服務端為連接生成 MAC（不再信任客戶端自報）。
			serverMAC, err := genServerMAC()
			if err != nil {
				sugar.Errorf("生成 MAC 失敗: %v", err)
				kcpConn.Close()
				return
			}

			// 客戶端可請求特定 MAC，服務端嘗試分配；衝突則拒絕。
			reqMAC := serverMAC
			if req.MAC != "" {
				if !validMAC(req.MAC) {
					sugar.Warnf("⚠️ 拒絕無效的請求 MAC (%q)", req.MAC)
					kcpConn.Close()
					return
				}
				reqMAC = req.MAC
			}

			var offset uint32
			var v4, v6 string
			if req.IPv4 != "" {
				v4, v6 = req.IPv4, req.IPv6
				offset, err = ipam.Reserve(v4, reqMAC, kcpConn)
			} else {
				offset, v4, v6, err = ipam.Allocate(reqMAC, kcpConn)
			}
			if err != nil {
				sugar.Warnf("⚠️ 拒絕連線 (%s): %v", reqMAC, err)
				kcpConn.Close()
				return
			}
			defer ipam.Release(offset, kcpConn)

			resp := HandshakeMsg{
				Type:      "response",
				IPv4:      v4,
				IPv6:      v6,
				IPv4Mask:  cfg.V4Mask,
				IPv6Mask:  cfg.V6Mask,
				GatewayV4: cfg.ServerV4,
				GatewayV6: cfg.ServerV6,
				MAC:       reqMAC,
			}
			respData, _ := json.Marshal(resp)
			if err := writeFrame(kcpConn, frameTypeHandshake, respData); err != nil {
				sugar.Warnf("⚠️ 發送握手響應失敗: %v", err)
				kcpConn.Close()
				return
			}

			sugar.Infof("✅ 客戶端連線: MAC=%s -> IP: %s (Offset: %d)", reqMAC, v4, offset)

			vswitch.Register(reqMAC, kcpConn)
			defer vswitch.Unregister(kcpConn)

			// ⑤ 每連接速率限制（客戶端→服務端注入方向）。
			var limiter *rate.Limiter
			if cfg.MaxRate > 0 {
				limiter = rate.NewLimiter(rate.Limit(cfg.MaxRate), int(cfg.MaxRate)/2+1)
			}
			var rateDrops uint64
			rateDropLog := time.Time{}
			var swDrops uint64
			swDropLog := time.Time{}

			// ---- 數據面（①：只接受 0x03 以太網幀 + 0x02 keepalive）----
			for {
				kcpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
				ft, data, err := readFrame(kcpConn, cfg.MaxFrameLen)
				if err != nil {
					sugar.Infof("🔌 連線中斷 (MAC=%s): %v", reqMAC, err)
					return
				}

				switch ft {
				case frameTypeKeepalive:
					// 回送 keepalive（獨立類型，不再依賴 len==0）。
					writeFrame(kcpConn, frameTypeKeepalive, nil)
					continue

				case frameTypeHandshake:
					// 數據面出現握手幀 = 協議違規，斷開。
					sugar.Warnf("⚠️ 數據面收到握手幀，斷開 (MAC=%s)", reqMAC)
					return

				case frameTypeEthernet:
					// ⑤ 速率限制。
					if limiter != nil && !limiter.AllowN(time.Now(), len(data)) {
						rateDrops++
						if time.Since(rateDropLog) > 5*time.Second {
							sugar.Warnf("⚠️ 超過速率限制，丟棄 (MAC=%s, 累計=%d)", reqMAC, rateDrops)
							rateDropLog = time.Now()
						}
						continue
					}
					tap.Write(data)
					// ④ VSwitch 丟棄計數。
					if !vswitch.Forward(data, kcpConn) {
						swDrops++
						if time.Since(swDropLog) > 5*time.Second {
							sugar.Warnf("⚠️ VSwitch 隊列滿，丟棄轉發 (MAC=%s, 累計=%d)", reqMAC, swDrops)
							swDropLog = time.Now()
						}
					}
				}
			}
		}(conn)
	}
}

// validateHandshake 校驗握手防重放（②）：時間窗 + nonce 未使用。
func validateHandshake(req *HandshakeMsg, nonces *seenNonces) error {
	now := time.Now()
	if req.Ts == 0 {
		return fmt.Errorf("缺少時間戳")
	}
	ts := time.Unix(req.Ts, 0)
	if diff := now.Sub(ts); diff < -handshakeWindow || diff > handshakeWindow {
		return fmt.Errorf("時間戳超出窗口 (%s)", ts.Format(time.RFC3339))
	}
	if len(req.Nonce) != 32 {
		return fmt.Errorf("無效的 nonce 長度: %d", len(req.Nonce))
	}
	raw, err := hex.DecodeString(req.Nonce)
	if err != nil || len(raw) != 16 {
		return fmt.Errorf("無效的 nonce 格式")
	}
	if !nonces.checkAndAdd(req.Nonce, now) {
		return fmt.Errorf("nonce 重放")
	}
	return nil
}

// genNonceHex 生成 16 字節隨機 nonce 的十六進制串（客戶端握手用）。
func genNonceHex() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
