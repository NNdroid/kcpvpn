package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigValid(t *testing.T) {
	path := writeTempConfig(t, `{
		"mode":"client",
		"addr":"10.0.0.1:4000",
		"tap":"tap1",
		"v4net":"10.8.0.0/22",
		"fec-d":5,"fec-p":2,
		"crypt":"aes-gcm",
		"psk":"supersecret",
		"obfs":"stun",
		"pad-max":128,
		"mtu":1500,
		"max-clients":32,
		"ip4":"10.8.0.10"
	}`)
	cfg, warns, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warns: %v", warns)
	}
	if cfg.Mode != "client" || cfg.Addr != "10.0.0.1:4000" || cfg.TapName != "tap1" {
		t.Fatalf("basic fields wrong: %+v", cfg)
	}
	if cfg.V4NetStr != "10.8.0.0/22" || cfg.ObfsMode != "stun" || cfg.PadMax != 128 {
		t.Fatalf("network/obfs fields wrong: %+v", cfg)
	}
	if cfg.MaxClients != 32 {
		t.Fatalf("max-clients wrong: %d", cfg.MaxClients)
	}
	// 派生字段
	if cfg.Key == nil || len(cfg.Key) != 32 {
		t.Fatalf("derived key wrong: len=%d", len(cfg.Key))
	}
	if cfg.Block == nil {
		t.Fatal("derived block is nil")
	}
	if cfg.ServerV4 != "10.8.0.1" {
		t.Fatalf("ServerV4 wrong: %s", cfg.ServerV4)
	}
	if cfg.MaxHosts != (1<<10)-2 {
		t.Fatalf("MaxHosts wrong: %d", cfg.MaxHosts)
	}
	if cfg.KCPMTU <= 0 || cfg.KCPMTU > 1500 {
		t.Fatalf("KCPMTU out of range: %d", cfg.KCPMTU)
	}
	if cfg.MaxFrameLen != cfg.MTU+32 {
		t.Fatalf("MaxFrameLen wrong: %d", cfg.MaxFrameLen)
	}
	if cfg.ReqV4 != "10.8.0.10" {
		t.Fatalf("ReqV4 wrong: %q", cfg.ReqV4)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// 只給 psk，其餘全部用默認值。
	path := writeTempConfig(t, `{"psk":"secret"}`)
	cfg, _, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Mode != "server" || cfg.Addr != "127.0.0.1:4000" || cfg.TapName != "tap0" {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.Crypt != "aes-gcm" || cfg.ObfsMode != "none" || cfg.MTU != 1500 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.FECData != 5 || cfg.FECParity != 2 {
		t.Fatalf("default fec wrong: %d/%d", cfg.FECData, cfg.FECParity)
	}
	if cfg.MaxClients != 256 {
		t.Fatalf("default max-clients wrong: %d", cfg.MaxClients)
	}
	if cfg.WebEnabled {
		t.Fatal("default web-enabled should be false")
	}
	if cfg.WebBind != "local" || cfg.WebAddr != ":8080" {
		t.Fatalf("default web fields wrong: bind=%q addr=%q", cfg.WebBind, cfg.WebAddr)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	cases := []struct {
		name, content string
	}{
		{"missing psk", `{}`},
		{"empty psk", `{"psk":""}`},
		{"bad mode", `{"psk":"x","mode":"bogus"}`},
		{"bad obfs", `{"psk":"x","obfs":"bogus"}`},
		{"bad crypt", `{"psk":"x","crypt":"bogus"}`},
		{"plaintext tunnel", `{"psk":"x","crypt":"none","obfs":"none"}`},
		{"mtu too small", `{"psk":"x","mtu":100}`},
		{"mtu too big", `{"psk":"x","mtu":10000}`},
		{"fec mismatch", `{"psk":"x","fec-d":0,"fec-p":2}`},
		{"pad too big", `{"psk":"x","pad-max":300}`},
		{"max-clients zero", `{"psk":"x","max-clients":0}`},
		{"bad web-bind", `{"psk":"x","web-enabled":true,"web-bind":"bogus"}`},
		{"public without token", `{"psk":"x","web-enabled":true,"web-bind":"public"}`},
		{"bad web-addr", `{"psk":"x","web-enabled":true,"web-addr":"nope"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.content)
			if _, _, err := loadConfig(path); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	if _, _, err := loadConfig(filepath.Join(t.TempDir(), "nonexistent.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfigFECDisabled(t *testing.T) {
	// fec-d/fec-p 同時為 0 合法（停用）。
	path := writeTempConfig(t, `{"psk":"x","fec-d":0,"fec-p":0}`)
	cfg, _, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.FECData != 0 || cfg.FECParity != 0 {
		t.Fatalf("fec should be disabled: %d/%d", cfg.FECData, cfg.FECParity)
	}
}

func TestLoadConfigDefaultPSKWarns(t *testing.T) {
	path := writeTempConfig(t, `{"psk":"HereIsYourSuperSecretPassword"}`)
	_, warns, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(warns) == 0 {
		t.Fatal("expected warning for default PSK")
	}
}

func TestComputeVersionFormat(t *testing.T) {
	// 在 git 倉庫內，computeVersion 應符合 v1.0.<yyyyMMdd>-<hash7>-<N> 格式。
	got := computeVersion()
	if got == version && version == "dev" {
		t.Skip("不在 git 倉庫或 git 不可用，跳過格式斷言")
	}
	if len(got) < len("v1.0.00000000-0000000-1") {
		t.Fatalf("版本號過短: %q", got)
	}
	if got[:5] != "v1.0." {
		t.Fatalf("版本號應以 v1.0. 開頭: %q", got)
	}
	// 手動解析: v1.0.<8位日期>-<7位hash>-<N>
	rest := got[5:]
	date := rest[:8]
	rest = rest[9:] // 跳過 '-'
	hash := rest[:7]
	rest = rest[8:] // 跳過 '-'
	n := rest
	if len(date) != 8 {
		t.Fatalf("日期段應為 8 位: %q", got)
	}
	if len(hash) != 7 {
		t.Fatalf("hash 段應為 7 位: %q", got)
	}
	if _, err := strconv.Atoi(n); err != nil {
		t.Fatalf("tag 計數段應為整數: %q (%v)", got, err)
	}
}
