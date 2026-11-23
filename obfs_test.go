package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

func TestObfsRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	datagrams := [][]byte{
		{},
		{1, 2, 3},
		make([]byte, 1500),
		{0x45, 0x00, 0x00, 0x14, 0xde, 0xad, 0xbe, 0xef},
	}

	for _, mode := range []string{"none", "xor", "stun", "dns", "dtls", "quic"} {
		for _, padMax := range []int{0, 10, 255} {
			c := newObfsPacketConn(nil, mode, key, padMax)
			for _, d := range datagrams {
				wrapped := c.wrapDatagram(d)
				got, err := c.unwrapDatagram(wrapped)
				if err != nil {
					t.Fatalf("[%s pad=%d] unwrap error: %v", mode, padMax, err)
				}
				if !bytes.Equal(got, d) {
					t.Fatalf("[%s pad=%d] round-trip mismatch: got %d bytes, want %d", mode, padMax, len(got), len(d))
				}
			}
		}
	}
}

func TestObfsHeaderLengths(t *testing.T) {
	if getObfsHeaderLen("stun") != 20 || getObfsHeaderLen("dns") != 12 ||
		getObfsHeaderLen("dtls") != 13 || getObfsHeaderLen("quic") != 8 ||
		getObfsHeaderLen("none") != 0 || getObfsHeaderLen("xor") != 0 {
		t.Fatal("obfs header lengths mismatch")
	}
}

func TestObfsPacketConnRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	for _, mode := range []string{"none", "xor", "stun", "dns", "dtls", "quic"} {
		underlying := &memPacketConn{max: 4096}
		c := &obfsPacketConn{
			PacketConn: underlying,
			mode:       mode,
			key:        key,
			padMax:     32,
			scratch:    make([]byte, 4096),
		}

		payload := []byte("hello-kcp-datagram")
		if _, err := c.WriteTo(payload, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}); err != nil {
			t.Fatalf("[%s] WriteTo: %v", mode, err)
		}

		got := make([]byte, 4096)
		n, _, err := c.ReadFrom(got)
		if err != nil {
			t.Fatalf("[%s] ReadFrom: %v", mode, err)
		}
		if !bytes.Equal(got[:n], payload) {
			t.Fatalf("[%s] datagram mismatch: got %q, want %q", mode, got[:n], payload)
		}
	}
}

// memPacketConn 是一個在內存中的 net.PacketConn，用於測試 obfsPacketConn。
type memPacketConn struct {
	buf [][]byte
	max int
}

func (m *memPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if len(m.buf) == 0 {
		return 0, nil, io.EOF
	}
	b := m.buf[0]
	m.buf = m.buf[1:]
	n := copy(p, b)
	return n, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}, nil
}

func (m *memPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	c := make([]byte, len(p))
	copy(c, p)
	m.buf = append(m.buf, c)
	return len(p), nil
}

func (m *memPacketConn) Close() error                       { return nil }
func (m *memPacketConn) LocalAddr() net.Addr                { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (m *memPacketConn) RemoteAddr() net.Addr               { return &net.UDPAddr{} }
func (m *memPacketConn) SetDeadline(t time.Time) error      { return nil }
func (m *memPacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *memPacketConn) SetWriteDeadline(t time.Time) error { return nil }

func TestFrameRoundTrip(t *testing.T) {
	// (type, payload) 組合：覆蓋握手、keepalive（空）、以太網數據。
	cases := []struct {
		ft      byte
		payload []byte
	}{
		{frameTypeHandshake, []byte(`{"type":"request"}`)},
		{frameTypeKeepalive, nil},
		{frameTypeEthernet, []byte{}},
		{frameTypeEthernet, []byte{0x01, 0x02, 0x03}},
		{frameTypeEthernet, make([]byte, 1500)},
		{frameTypeEthernet, make([]byte, 2048)},
	}

	for _, tc := range cases {
		client, server := net.Pipe()
		go func() {
			writeFrame(client, tc.ft, tc.payload)
			client.Close()
		}()

		gotFT, got, err := readFrame(server, 4096)
		if err != nil {
			t.Fatalf("readFrame (type=%d len=%d): %v", tc.ft, len(tc.payload), err)
		}
		if gotFT != tc.ft {
			t.Fatalf("frame type mismatch: got %#x, want %#x", gotFT, tc.ft)
		}
		if !bytes.Equal(got, tc.payload) {
			t.Fatalf("frame mismatch (type=%d): got %d bytes, want %d", tc.ft, len(got), len(tc.payload))
		}
		server.Close()
	}
}

func TestFrameInvalidTypeRejected(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		// 非法幀類型 0x7F。
		client.Write([]byte{0x7F, 0x00, 0x00})
		client.Close()
	}()

	if _, _, err := readFrame(server, 100); err == nil {
		t.Fatal("expected invalid frame type error, got nil")
	}
	server.Close()
}

func TestFrameOversizeRejected(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		// 手動構造一個超大幀（長度 0xFFFF > maxLen=100）。
		buf := []byte{frameTypeEthernet, 0xFF, 0xFF}
		client.Write(buf)
		client.Close()
	}()

	_, _, err := readFrame(server, 100)
	if err == nil {
		t.Fatal("expected oversize frame error, got nil")
	}
	server.Close()
}

// TestKCPObfsIntegration 端到端：真實 UDP + KCP + obfs 包裝 + AES-GCM 認證加密，
// 驗證 S1（obfs 在 KCP 之外）與 S3（AEAD）全鏈路可用。無需 TAP/管理員權限。
func TestKCPObfsIntegration(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	block, err := kcp.NewAESGCMCrypt(key)
	if err != nil {
		t.Fatalf("NewAESGCMCrypt: %v", err)
	}

	// (fecD, fecP, payloadSize)：0/0 用小包驗證 obfs+AEAD；5/2 用大包填滿 FEC 組驗證 FEC+obfs 路徑。
	cases := []struct {
		fecD, fecP int
		payload    int
	}{
		{0, 0, 64},
		{5, 2, 8192},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("fec%d-%d", tc.fecD, tc.fecP), func(t *testing.T) {
			// 服務端：原始 UDP + obfs 包裝 + ServeConn。
			srvRaw, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatalf("ListenUDP: %v", err)
			}
			defer srvRaw.Close()
			srvObfs := newObfsPacketConn(srvRaw, "stun", key, 0)
			listener, err := kcp.ServeConn(block, tc.fecD, tc.fecP, srvObfs)
			if err != nil {
				t.Fatalf("ServeConn: %v", err)
			}
			defer listener.Close()

			go func() {
				sess, err := listener.AcceptKCP()
				if err != nil {
					return
				}
				defer sess.Close()
				sess.SetNoDelay(1, 10, 2, 1) // 與生產代碼一致，避免默認 100ms 更新間隔造成測試抖動
				sess.SetReadDeadline(time.Now().Add(5 * time.Second))
				ft, data, err := readFrame(sess, 16384)
				if err != nil {
					t.Errorf("server readFrame: %v", err)
					return
				}
				if ft != frameTypeEthernet {
					t.Errorf("server got frame type %#x, want Ethernet", ft)
					return
				}
				// 回顯為 Ethernet 類型。
				if err := writeFrame(sess, frameTypeEthernet, data); err != nil {
					t.Errorf("server writeFrame: %v", err)
				}
			}()

			// 客戶端：原始 UDP + obfs 包裝 + NewConn2。
			cliRaw, err := net.ListenUDP("udp4", nil)
			if err != nil {
				t.Fatalf("ListenUDP client: %v", err)
			}
			defer cliRaw.Close()
			cliObfs := newObfsPacketConn(cliRaw, "stun", key, 0)
			sess, err := kcp.NewConn2(srvRaw.LocalAddr(), block, tc.fecD, tc.fecP, cliObfs)
			if err != nil {
				t.Fatalf("NewConn2: %v", err)
			}
			defer sess.Close()
			sess.SetNoDelay(1, 10, 2, 1) // 與生產代碼一致

			sess.SetReadDeadline(time.Now().Add(8 * time.Second))
			payload := make([]byte, tc.payload)
			rand.Read(payload)
			if err := writeFrame(sess, frameTypeEthernet, payload); err != nil {
				t.Fatalf("client writeFrame: %v", err)
			}
			ft, got, err := readFrame(sess, 16384)
			if err != nil {
				t.Fatalf("client readFrame: %v", err)
			}
			if ft != frameTypeEthernet {
				t.Fatalf("client got frame type %#x, want Ethernet", ft)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("echo mismatch: got %d bytes want %d", len(got), len(payload))
			}
		})
	}
}
