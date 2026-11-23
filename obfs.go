package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// obfsPacketConn 把一个 UDP 数据报包装成 STUN / DNS / DTLS / QUIC / XOR 形态。
//
// 它位于 KCP 之外（直接包装 *net.UDPConn），因此包装后的形态会真实出现在
// 线缆上，起到流量伪装（抗 DPI）的作用。KCP 的会话/监听器通过通用
// ReadFrom/WriteTo 路径读写本包装器（kcp-go 对非 *net.UDPConn 会回退到
// defaultReadLoop/defaultMonitor，见 readloop_linux.go）。
type obfsPacketConn struct {
	net.PacketConn // 底层 UDP 连接（提供 Close/SetDeadline 等）
	raw            *net.UDPConn
	mode           string
	key            []byte
	padMax         int
	scratch        []byte
}

// errObfsShort 表示数据报短于伪装头/填充，无法解包（视为非法包丢弃）。
var errObfsShort = errors.New("obfs: 数据报过短")

func newObfsPacketConn(raw *net.UDPConn, mode string, key []byte, padMax int) *obfsPacketConn {
	if padMax < 0 {
		padMax = 0
	}
	if padMax > 255 {
		padMax = 255
	}
	return &obfsPacketConn{
		PacketConn: raw,
		raw:        raw,
		mode:       mode,
		key:        key,
		padMax:     padMax,
		// 足够容纳：KCP 数据报(<=1500) + 伪装头(<=20) + padLen(1) + 填充(<=padMax)
		scratch: make([]byte, 2048+padMax),
	}
}

// SetReadBuffer 透传到底层 socket（实现 kcp-go 的 setReadBuffer 接口）。
func (c *obfsPacketConn) SetReadBuffer(n int) error { return c.raw.SetReadBuffer(n) }

// SetWriteBuffer 透传到底层 socket（实现 kcp-go 的 setWriteBuffer 接口）。
func (c *obfsPacketConn) SetWriteBuffer(n int) error { return c.raw.SetWriteBuffer(n) }

// SetDSCP 设置 DSCP 字段（实现 kcp-go 的 setDSCP 接口）。
func (c *obfsPacketConn) SetDSCP(dscp int) error {
	if dscp <= 0 {
		return nil
	}
	// 与 kcp-go 内部一致的设定方式：IPv4 使用 TOS 高 6 位，IPv6 使用 Traffic Class。
	if err := ipv4.NewConn(c.raw).SetTOS(dscp << 2); err == nil {
		return nil
	}
	return ipv6.NewConn(c.raw).SetTrafficClass(dscp)
}

// WriteTo 把 KCP 数据报 p 包装后发送。
func (c *obfsPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	wrapped := c.wrapDatagram(p)
	if _, err := c.PacketConn.WriteTo(wrapped, addr); err != nil {
		return 0, err
	}
	return len(p), nil
}

// ReadFrom 读取一个数据报，解包伪装层后回填 buf。
func (c *obfsPacketConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(c.scratch)
	if err != nil {
		return 0, addr, err
	}
	inner, err := c.unwrapDatagram(c.scratch[:n])
	if err != nil {
		return 0, addr, err
	}
	if len(inner) > len(buf) {
		return 0, addr, errObfsShort
	}
	copy(buf, inner)
	return len(inner), addr, nil
}

// wrapDatagram 包装单个 KCP 数据报。
//
// 格式：
//
//	none  且 padMax==0:            [数据报]                        （恒等）
//	none  且 padMax>0:             [padLen:1][pad][数据报]
//	xor:                          XOR([padLen:1][pad][数据报])
//	stun/dns/dtls/quic:           [伪装头][padLen:1][pad][数据报]
func (c *obfsPacketConn) wrapDatagram(datagram []byte) []byte {
	headerLen := getObfsHeaderLen(c.mode)
	needPadByte := c.padMax > 0 || c.mode == "xor"

	// 恒等路径：无头、无填充，直接透传（常见情形，避免每包分配）。
	if headerLen == 0 && !needPadByte {
		return datagram
	}

	padLen := 0
	if needPadByte {
		var p [1]byte
		rand.Read(p[:])
		padLen = int(p[0]) % (c.padMax + 1)
	}

	padByte := 0
	if needPadByte {
		padByte = 1
	}
	total := headerLen + padByte + padLen + len(datagram)
	out := make([]byte, total)
	if headerLen > 0 {
		hdrPayloadLen := len(datagram) + padLen + padByte
		writeObfsHeader(out, c.mode, c.key, hdrPayloadLen)
	}
	off := headerLen
	if needPadByte {
		out[off] = byte(padLen)
		off++
		if padLen > 0 {
			rand.Read(out[off : off+padLen])
			off += padLen
		}
	}
	copy(out[off:], datagram)

	if c.mode == "xor" {
		applyXOR(out, c.key)
	}
	return out
}

// unwrapDatagram 解包一个收到的数据报，返回原始 KCP 数据报。
func (c *obfsPacketConn) unwrapDatagram(raw []byte) ([]byte, error) {
	if c.mode == "xor" {
		applyXOR(raw, c.key)
		if len(raw) < 1 {
			return nil, errObfsShort
		}
		padLen := int(raw[0])
		raw = raw[1:]
		if padLen > 0 {
			if len(raw) < padLen {
				return nil, errObfsShort
			}
			raw = raw[padLen:]
		}
		return raw, nil
	}

	switch c.mode {
	case "stun":
		if len(raw) < 20 {
			return nil, errObfsShort
		}
		raw = raw[20:]
	case "dns":
		if len(raw) < 12 {
			return nil, errObfsShort
		}
		raw = raw[12:]
	case "dtls":
		if len(raw) < 13 {
			return nil, errObfsShort
		}
		raw = raw[13:]
	case "quic":
		if len(raw) < 8 {
			return nil, errObfsShort
		}
		raw = raw[8:]
	}

	if c.padMax > 0 {
		if len(raw) < 1 {
			return nil, errObfsShort
		}
		padLen := int(raw[0])
		raw = raw[1:]
		if padLen > 0 {
			if len(raw) < padLen {
				return nil, errObfsShort
			}
			raw = raw[padLen:]
		}
	}
	return raw, nil
}

// applyXOR 用 key 对 data 做重复 XOR（对称，加解密同函数）。
func applyXOR(data []byte, key []byte) {
	if len(key) == 0 {
		return
	}
	for i := range data {
		data[i] ^= key[i%len(key)]
	}
}

// getObfsHeaderLen 返回各伪装模式的固定头长度。
func getObfsHeaderLen(mode string) int {
	switch mode {
	case "stun":
		return 20
	case "dns":
		return 12
	case "dtls":
		return 13
	case "quic":
		return 8
	default:
		return 0
	}
}

// writeObfsHeader 写入伪装协议头。payloadLen 为头之后（含 padLen+pad+数据报）的总长度，
// 用于把协议头中的长度字段填成合理值，增强 DPI 辨识度。
func writeObfsHeader(buf []byte, mode string, key []byte, payloadLen int) {
	switch mode {
	case "stun":
		binary.BigEndian.PutUint16(buf[0:2], 0x0101) // STUN Binding Success Response
		binary.BigEndian.PutUint16(buf[2:4], uint16(payloadLen))
		binary.BigEndian.PutUint32(buf[4:8], 0x2112A442) // STUN Magic Cookie
		if len(key) >= 12 {
			copy(buf[8:20], key[:12])
		} else {
			rand.Read(buf[8:20])
		}

	case "dns":
		binary.BigEndian.PutUint16(buf[0:2], 0x1234) // Transaction ID
		binary.BigEndian.PutUint16(buf[2:4], 0x8180) // Flags: Standard query response
		binary.BigEndian.PutUint16(buf[4:6], 1)      // Questions
		binary.BigEndian.PutUint16(buf[6:8], 1)      // Answers
		binary.BigEndian.PutUint16(buf[8:10], 0)
		binary.BigEndian.PutUint16(buf[10:12], 0)

	case "dtls":
		buf[0] = 0x17                                // ContentType: Application Data
		binary.BigEndian.PutUint16(buf[1:3], 0xFEFF) // Version: DTLS 1.2
		binary.BigEndian.PutUint16(buf[3:5], 0x0001) // Epoch
		buf[5], buf[6], buf[7], buf[8], buf[9] = 0, 0, 0, 0, 0
		buf[10] = 1
		binary.BigEndian.PutUint16(buf[11:13], uint16(payloadLen))

	case "quic":
		buf[0] = 0x40 // QUIC Short Header
		if len(key) >= 4 {
			copy(buf[1:5], key[:4])
		}
		buf[5], buf[6], buf[7] = 0x00, 0x00, 0x01
	}
}
