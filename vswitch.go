package main

import (
	"net"
	"sync"

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
}

func NewVSwitch(tap *water.Interface) *VSwitch {
	return &VSwitch{
		tap:      tap,
		clients:  make(map[net.Conn]chan []byte),
		macTable: make(map[string]net.Conn),
	}
}

func (s *VSwitch) Register(mac string, conn net.Conn, obfsMode string, key []byte, padMax int) {
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
			if err := writeFrame(conn, packet, obfsMode, key, padMax); err != nil {
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

func (s *VSwitch) Forward(packet []byte, sender net.Conn) {
	if len(packet) < 14 {
		return
	}
	dstMAC := string(packet[0:6])
	srcMAC := string(packet[6:12])

	if sender != nil {
		s.mu.RLock()
		existingConn, ok := s.macTable[srcMAC]
		s.mu.RUnlock()

		if !ok || existingConn != sender {
			s.mu.Lock()
			s.macTable[srcMAC] = sender
			s.mu.Unlock()
		}
	}

	pktCopy := make([]byte, len(packet))
	copy(pktCopy, packet)

	s.mu.RLock()
	defer s.mu.RUnlock()

	isMulticast := (packet[0] & 1) == 1
	if isMulticast {
		for conn, ch := range s.clients {
			if conn != sender {
				select {
				case ch <- pktCopy:
				default:
				}
			}
		}
	} else {
		if targetConn, ok := s.macTable[dstMAC]; ok && targetConn != sender {
			if ch, exists := s.clients[targetConn]; exists {
				select {
				case ch <- pktCopy:
				default:
				}
			}
		} else {
			for conn, ch := range s.clients {
				if conn != sender {
					select {
					case ch <- pktCopy:
					default:
					}
				}
			}
		}
	}
}
