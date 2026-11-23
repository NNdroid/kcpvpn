package main

import (
	"net"
	"testing"
	"time"
)

func TestVSwitchSrcMACEnforcement(t *testing.T) {
	vs := NewVSwitch(nil)
	macA := "aa:bb:cc:dd:ee:ff"
	macB := "11:22:33:44:55:66"

	bReader, bWriter := net.Pipe()
	vs.Register(macB, bWriter)
	defer vs.Unregister(bWriter)
	defer bWriter.Close()

	aReader, aWriter := net.Pipe()
	vs.Register(macA, aWriter)
	defer vs.Unregister(aWriter)
	defer aReader.Close()

	hwA, _ := net.ParseMAC(macA)
	hwB, _ := net.ParseMAC(macB)

	frame := make([]byte, 60)
	copy(frame[0:6], hwB)  // dst = B
	copy(frame[6:12], hwA) // src = A（已註冊）

	// 合法幀：A 發往 B，應送達。
	if !vs.Forward(frame, aWriter) {
		t.Fatal("合法幀應返回 true")
	}
	bReader.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := bReader.Read(make([]byte, 1024))
	if err != nil || n == 0 {
		t.Fatalf("合法幀未送達 B: n=%d err=%v", n, err)
	}

	// 偽造幀：A 冒用 B 的源 MAC，應被丟棄。
	copy(frame[6:12], hwB)
	if vs.Forward(frame, aWriter) {
		t.Fatal("偽造源 MAC 的幀應返回 false")
	}
	bReader.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, err = bReader.Read(make([]byte, 1024)); err == nil {
		t.Fatalf("偽造源 MAC 的幀未被丟棄 (n=%d)", n)
	}
}

func TestIPAMReserveBoundaries(t *testing.T) {
	_, v4Net, err := net.ParseCIDR("10.0.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	v4Net.IP = v4Net.IP.Mask(v4Net.Mask)
	ipam := NewIPAM(v4Net, nil, 254)

	// 網關地址（+1）不可保留。
	if _, err := ipam.Reserve("10.0.0.1", "aa:bb:cc:dd:ee:ff", nil); err == nil {
		t.Fatal("網關地址應被拒絕")
	}
	// 廣播地址（+255）不可保留。
	if _, err := ipam.Reserve("10.0.0.255", "aa:bb:cc:dd:ee:ff", nil); err == nil {
		t.Fatal("廣播地址應被拒絕")
	}
	// 網段外。
	if _, err := ipam.Reserve("10.0.1.5", "aa:bb:cc:dd:ee:ff", nil); err == nil {
		t.Fatal("網段外地址應被拒絕")
	}
	// 合法保留。
	if _, err := ipam.Reserve("10.0.0.2", "aa:bb:cc:dd:ee:ff", nil); err != nil {
		t.Fatalf("合法保留失敗: %v", err)
	}
	// 已被其他 MAC 佔用。
	if _, err := ipam.Reserve("10.0.0.2", "11:22:33:44:55:66", nil); err == nil {
		t.Fatal("已被佔用的 IP 應被拒絕")
	}
}

func TestIPAMAllocateExhaust(t *testing.T) {
	_, v4Net, _ := net.ParseCIDR("10.0.0.0/30")
	v4Net.IP = v4Net.IP.Mask(v4Net.Mask)
	ipam := NewIPAM(v4Net, nil, 2)

	off1, _, _, err := ipam.Allocate("aa:bb:cc:dd:ee:ff", nil)
	if err != nil || off1 != 2 {
		t.Fatalf("Allocate 應分配 offset=2, got %d err=%v", off1, err)
	}
	// 同一 MAC 重用同一 offset。
	off2, _, _, err := ipam.Allocate("aa:bb:cc:dd:ee:ff", nil)
	if err != nil || off2 != off1 {
		t.Fatalf("同一 MAC 應重用 offset, got %d err=%v", off2, err)
	}
	// 其他 MAC 導致耗盡。
	if _, _, _, err := ipam.Allocate("11:22:33:44:55:66", nil); err == nil {
		t.Fatal("地址池耗盡應返回錯誤")
	}
}

func TestValidMAC(t *testing.T) {
	if !validMAC("aa:bb:cc:dd:ee:ff") {
		t.Fatal("合法 MAC 應通過")
	}
	if validMAC("invalid") {
		t.Fatal("非法 MAC 應拒絕")
	}
	if validMAC("aa:bb:cc:dd:ee:ff:11:22") {
		t.Fatal("8 字節 MAC 應拒絕")
	}
}

func TestComputeKCPMTU(t *testing.T) {
	// AES-GCM + FEC，obfs=none：理想 1576，不分片上限 1471 → 取 1471。
	if got := computeKCPMTU(1500, 0, 0, "aes-gcm", true); got != 1471 {
		t.Fatalf("kcpMTU(aes-gcm) = %d, want 1471", got)
	}
	// Salsa20 無 FEC，obfs=stun(20)：理想 1560，上限 1451 → 取 1451。
	if got := computeKCPMTU(1500, 20, 0, "salsa20", false); got != 1451 {
		t.Fatalf("kcpMTU(salsa20+stun) = %d, want 1451", got)
	}
	// 小 MTU：理想值低於上限 → 取理想值。
	if got := computeKCPMTU(1280, 0, 0, "aes-gcm", true); got != 1357 {
		t.Fatalf("kcpMTU(1280) = %d, want 1357", got)
	}
	// 上限不得低於 576。
	if got := computeKCPMTU(1500, 20, 255, "salsa20", false); got < 576 {
		t.Fatalf("kcpMTU 下限違反: %d", got)
	}
}
