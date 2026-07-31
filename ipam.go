package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

type HandshakeMsg struct {
	Type      string `json:"type"`
	IPv4      string `json:"ipv4,omitempty"`
	IPv6      string `json:"ipv6,omitempty"`
	IPv4Mask  int    `json:"ipv4_mask,omitempty"`
	IPv6Mask  int    `json:"ipv6_mask,omitempty"`
	GatewayV4 string `json:"gateway_v4,omitempty"`
	GatewayV6 string `json:"gateway_v6,omitempty"`
	MAC       string `json:"mac,omitempty"`
}

type IPRecord struct {
	MAC  string
	Conn net.Conn
}

type IPAM struct {
	mu       sync.Mutex
	v4Net    *net.IPNet
	v6Net    *net.IPNet
	maxHosts uint32
	usedIDs  map[uint32]IPRecord
}

func NewIPAM(v4Net, v6Net *net.IPNet, maxHosts uint32) *IPAM {
	return &IPAM{
		v4Net:    v4Net,
		v6Net:    v6Net,
		maxHosts: maxHosts,
		usedIDs:  make(map[uint32]IPRecord),
	}
}

func parseNetworks(v4Str, v6Str string) (*net.IPNet, *net.IPNet, uint32, error) {
	ip4, v4Net, err := net.ParseCIDR(v4Str)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("IPv4 解析失敗: %v", err)
	}
	v4Net.IP = ip4.Mask(v4Net.Mask)

	var v6Net *net.IPNet
	if v6Str != "" {
		ip6, parsedV6Net, err := net.ParseCIDR(v6Str)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("IPv6 解析失敗: %v", err)
		}
		v6Net = parsedV6Net
		v6Net.IP = ip6.Mask(v6Net.Mask)
	}

	ones, bits := v4Net.Mask.Size()
	if bits != 32 || ones > 30 {
		return nil, nil, 0, fmt.Errorf("無效的 IPv4 遮罩 (需小於等於 /30)")
	}

	maxHosts := (uint32(1) << (32 - ones)) - 2
	return v4Net, v6Net, maxHosts, nil
}

func getIP4ByOffset(netIP *net.IPNet, offset uint32) string {
	if netIP == nil {
		return ""
	}
	base := binary.BigEndian.Uint32(netIP.IP.To4())
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, base+offset)
	return ip.String()
}

func getIP6ByOffset(netIP *net.IPNet, offset uint32) string {
	if netIP == nil {
		return ""
	}
	res := make(net.IP, len(netIP.IP))
	copy(res, netIP.IP)
	var carry uint32 = offset
	for i := len(res) - 1; i >= 0 && carry > 0; i-- {
		sum := uint32(res[i]) + carry
		res[i] = byte(sum)
		carry = sum >> 8
	}
	return res.String()
}

func (i *IPAM) Allocate(mac string, conn net.Conn) (uint32, string, string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	for offset, rec := range i.usedIDs {
		if rec.MAC == mac {
			i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
			return offset, getIP4ByOffset(i.v4Net, offset), getIP6ByOffset(i.v6Net, offset), nil
		}
	}

	for offset := uint32(2); offset <= i.maxHosts; offset++ {
		if _, taken := i.usedIDs[offset]; !taken {
			i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
			return offset, getIP4ByOffset(i.v4Net, offset), getIP6ByOffset(i.v6Net, offset), nil
		}
	}
	return 0, "", "", fmt.Errorf("IP 地址池已耗盡 (最大容量: %d)", i.maxHosts-1)
}

func (i *IPAM) Reserve(v4Str string, mac string, conn net.Conn) (uint32, error) {
	reqIP := net.ParseIP(v4Str).To4()
	if reqIP == nil {
		return 0, fmt.Errorf("無效的 IPv4 格式: %s", v4Str)
	}

	if !i.v4Net.Contains(reqIP) {
		return 0, fmt.Errorf("請求的 IP (%s) 不在網段內", v4Str)
	}

	base := binary.BigEndian.Uint32(i.v4Net.IP.To4())
	reqInt := binary.BigEndian.Uint32(reqIP)
	offset := reqInt - base

	if offset < 2 || offset > i.maxHosts {
		return 0, fmt.Errorf("保留的 IP (%s) 是網段保留地址 (網關或廣播)", v4Str)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if rec, taken := i.usedIDs[offset]; taken {
		if rec.MAC == mac {
			i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
			return offset, nil
		}
		return 0, fmt.Errorf("指定的 IP (%s) 已被其他設備佔用", v4Str)
	}
	i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
	return offset, nil
}

func (i *IPAM) Release(offset uint32, conn net.Conn) {
	if offset <= 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if rec, taken := i.usedIDs[offset]; taken && rec.Conn == conn {
		delete(i.usedIDs, offset)
		sugar.Debugf("♻️ [IPAM] 已回收 IP 偏移量: +%d", offset)
	}
}
