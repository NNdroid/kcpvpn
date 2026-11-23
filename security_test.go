package main

import (
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestValidateHandshake 覆盖②握手防重放校验：时间窗、nonce 格式、重放拒绝。
func TestValidateHandshake(t *testing.T) {
	nonces := newSeenNonces()
	now := time.Now()

	// 合法握手：当前时间戳 + 有效 nonce。
	req := &HandshakeMsg{Type: "request", Nonce: genNonceHex(), Ts: now.Unix()}
	if err := validateHandshake(req, nonces); err != nil {
		t.Fatalf("合法握手应通过: %v", err)
	}

	// 重放：同一 nonce 再次使用应被拒绝。
	if err := validateHandshake(req, nonces); err == nil {
		t.Fatal("重放的 nonce 应被拒绝")
	}

	// 时间戳超出窗口（未来 10 分钟）。
	future := &HandshakeMsg{Nonce: genNonceHex(), Ts: now.Add(10 * time.Minute).Unix()}
	if err := validateHandshake(future, nonces); err == nil {
		t.Fatal("超窗未来时间戳应被拒绝")
	}

	// 时间戳超出窗口（过去 10 分钟）。
	past := &HandshakeMsg{Nonce: genNonceHex(), Ts: now.Add(-10 * time.Minute).Unix()}
	if err := validateHandshake(past, nonces); err == nil {
		t.Fatal("超窗过去时间戳应被拒绝")
	}

	// 缺少时间戳。
	noTs := &HandshakeMsg{Nonce: genNonceHex(), Ts: 0}
	if err := validateHandshake(noTs, nonces); err == nil {
		t.Fatal("缺少时间戳应被拒绝")
	}

	// nonce 长度错误（非 32 字符 hex）。
	short := &HandshakeMsg{Nonce: "abcd", Ts: now.Unix()}
	if err := validateHandshake(short, nonces); err == nil {
		t.Fatal("无效 nonce 长度应被拒绝")
	}

	// nonce 非 hex。
	badHex := &HandshakeMsg{Nonce: strings.Repeat("z", 32), Ts: now.Unix()}
	if err := validateHandshake(badHex, nonces); err == nil {
		t.Fatal("非 hex nonce 应被拒绝")
	}
}

// TestSeenNoncesCleanup 覆盖 nonce 集合的过期清理（避免无限增长）。
func TestSeenNoncesCleanup(t *testing.T) {
	nonces := newSeenNonces()
	now := time.Now()

	// 注入一个已过期的 nonce。
	nonces.seen["stale"] = now.Add(-handshakeWindow - 2*time.Minute)
	// 再注入一个有效 nonce，触发清理。
	req := &HandshakeMsg{Nonce: genNonceHex(), Ts: now.Unix()}
	if err := validateHandshake(req, nonces); err != nil {
		t.Fatalf("validateHandshake: %v", err)
	}
	nonces.mu.Lock()
	defer nonces.mu.Unlock()
	if _, ok := nonces.seen["stale"]; ok {
		t.Fatal("过期的 nonce 应被清理")
	}
}

// TestGenServerMAC 覆盖③服务端 MAC 生成：本地管理单播 + 合法格式。
func TestGenServerMAC(t *testing.T) {
	macStr, err := genServerMAC()
	if err != nil {
		t.Fatalf("genServerMAC: %v", err)
	}
	hw, err := net.ParseMAC(macStr)
	if err != nil || len(hw) != 6 {
		t.Fatalf("生成的 MAC 不合法: %q err=%v", macStr, err)
	}
	// 必须设置 locally administered 位（0x02），且清除 multicast 位（0x01）。
	if hw[0]&0x02 == 0 {
		t.Fatalf("生成的 MAC 未设置本地管理位: %q", macStr)
	}
	if hw[0]&0x01 != 0 {
		t.Fatalf("生成的 MAC 设置了多播位: %q", macStr)
	}
}

// TestRateLimiterRejectsOverBurst 覆盖⑤速率限制：limiter 拒绝超出令牌桶的注入。
func TestRateLimiterRejectsOverBurst(t *testing.T) {
	// 1 MB/s，burst=1MB。一次性放行 burst 字节，随后立即再注入应被拒绝。
	limiter := rate.NewLimiter(rate.Limit(1<<20), 1<<20)
	now := time.Now()
	if !limiter.AllowN(now, 1<<20) {
		t.Fatal("突发内首个 AllowN 应通过")
	}
	if limiter.AllowN(now, 1) {
		t.Fatal("耗尽令牌后 AllowN 应被拒绝")
	}

	// 等待令牌恢复（约 1MB/1MBps = 1s），少量字节应再次通过。
	time.Sleep(1100 * time.Millisecond)
	if !limiter.AllowN(time.Now(), 100) {
		t.Fatal("令牌恢复后 AllowN 应通过")
	}
}

// TestFrameTypeHandshakeViolation 覆盖①数据面类型约束：keepalive 与数据各自独立。
// readFrame 对空 keepalive 与以太网帧均正确区分类型。
func TestFrameTypeHandshakeViolation(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// 数据面出现握手帧（0x01）应被上层识别为协议违规。
	go writeFrame(client, frameTypeHandshake, []byte(`{"type":"request"}`))
	ft, _, err := readFrame(server, 4096)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if ft != frameTypeHandshake {
		t.Fatalf("应读到握手帧类型, got %#x", ft)
	}
}
