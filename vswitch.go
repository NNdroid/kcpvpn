package main

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/songgao/water"
)

func parseMacKey(macStr string) string {
	hw, err := net.ParseMAC(macStr)
	if err != nil || len(hw) < 6 {
		return macStr
	}
	return string(hw[:6])
}

type VSwitch struct {
	tap      *water.Interface
	clients  map[net.Conn]chan []byte
	macTable map[string]net.Conn
	mu       sync.RWMutex
	drops    atomic.Uint64 // ④ 隊列滿丟棄計數

	// 流量計數（按方向，成功轉發的幀）。
	rxBytes, rxPkts atomic.Uint64 // 入站：客戶端 → TAP
	txBytes, txPkts atomic.Uint64 // 出站：TAP → 客戶端
}

// TrafficStats 是對外暴露的流量快照（JSON）。
type TrafficStats struct {
	RxBytes uint64 `json:"rx_bytes"`
	RxPkts  uint64 `json:"rx_pkts"`
	TxBytes uint64 `json:"tx_bytes"`
	TxPkts  uint64 `json:"tx_pkts"`
}

func NewVSwitch(tap *water.Interface) *VSwitch {
	return &VSwitch{
		tap:      tap,
		clients:  make(map[net.Conn]chan []byte),
		macTable: make(map[string]net.Conn),
	}
}

// Register 綁定一個客戶端連接到其 MAC。若該 MAC 已被其他連接佔用，則踢掉舊連接。
func (s *VSwitch) Register(mac string, conn net.Conn) {
	macKey := parseMacKey(mac)

	s.mu.Lock()
	if oldConn, exists := s.macTable[macKey]; exists && oldConn != conn {
		sugar.Warnf("⚠️ 發現相同 MAC (%s) 登入，踢出舊連接", mac)
		oldConn.Close()
	}
	sendChan := make(chan []byte, 1024)
	s.clients[conn] = sendChan
	s.macTable[macKey] = conn
	s.mu.Unlock()

	go func() {
		for packet := range sendChan {
			if err := writeFrame(conn, frameTypeEthernet, packet); err != nil {
				s.Unregister(conn)
				return
			}
		}
	}()
}

func (s *VSwitch) Unregister(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.clients[conn]; ok {
		close(ch)
		delete(s.clients, conn)
	}
	for macKey, c := range s.macTable {
		if c == conn {
			delete(s.macTable, macKey)
		}
	}
	conn.Close()
}

// Forward 轉發一個以太網幀。
// sender == nil 表示來自 TAP（本機網路），否則來自某個客戶端連接。
// 返回 false 表示因隊列滿而被丟棄（④：可觀測性）。
func (s *VSwitch) Forward(packet []byte, sender net.Conn) bool {
	if len(packet) < 14 {
		return false
	}
	dstMAC := string(packet[0:6])
	srcMAC := string(packet[6:12])

	if sender != nil {
		// S5: 客戶端幀的源 MAC 必須等於其註冊的 MAC，否則丟棄。
		// 防止 MAC 表污染與跨客戶端源地址仿冒。
		s.mu.RLock()
		registered, ok := s.macTable[srcMAC]
		s.mu.RUnlock()
		if !ok || registered != sender {
			sugar.Warnf("⚠️ 丟棄源 MAC 未註冊的幀 (srcMAC=%x, sender=%p)", packet[6:12], sender)
			return false
		}
	}

	pktCopy := make([]byte, len(packet))
	copy(pktCopy, packet)

	// 按方向累加流量計數（進入交換機的有效幀；隊列滿丟棄另由 drops 反映）。
	n := uint64(len(packet))
	if sender == nil {
		s.txBytes.Add(n)
		s.txPkts.Add(1)
	} else {
		s.rxBytes.Add(n)
		s.rxPkts.Add(1)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	isMulticast := (packet[0] & 1) == 1
	if isMulticast {
		for conn, ch := range s.clients {
			if conn != sender {
				select {
				case ch <- pktCopy:
				default:
					s.drops.Add(1) // ④ 隊列滿丟棄計數
				}
			}
		}
	} else {
		if targetConn, ok := s.macTable[dstMAC]; ok && targetConn != sender {
			if ch, exists := s.clients[targetConn]; exists {
				select {
				case ch <- pktCopy:
				default:
					s.drops.Add(1)
				}
			}
		} else {
			for conn, ch := range s.clients {
				if conn != sender {
					select {
					case ch <- pktCopy:
					default:
						s.drops.Add(1)
					}
				}
			}
		}
	}
	return true
}

// Drops 返回累計丟棄幀數（④）。
func (s *VSwitch) Drops() uint64 {
	return s.drops.Load()
}

// Traffic 返回按方向的累計流量（字節/包）。
func (s *VSwitch) Traffic() TrafficStats {
	return TrafficStats{
		RxBytes: s.rxBytes.Load(),
		RxPkts:  s.rxPkts.Load(),
		TxBytes: s.txBytes.Load(),
		TxPkts:  s.txPkts.Load(),
	}
}
