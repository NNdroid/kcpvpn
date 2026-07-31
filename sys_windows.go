//go:build windows

package main

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/songgao/water"
)

func createTAP(tapName string) (*water.Interface, error) {
	config := water.Config{DeviceType: water.TAP}
	config.ComponentID = "tap0901"
	if tapName != "" {
		config.InterfaceName = tapName
	}
	iface, err := water.New(config)
	if err != nil && tapName != "" {
		sugar.Warnf("⚠️ 未找到名稱為 '%s' 的 TAP 網卡，嘗試自動匹配系统可用 TAP 網卡...", tapName)
		config.InterfaceName = ""
		return water.New(config)
	}
	return iface, err
}

func setupInterface(name, v4CIDR, v6CIDR string) error {
	ip, ipNet, err := net.ParseCIDR(v4CIDR)
	if err != nil {
		return fmt.Errorf("解析 IPv4 CIDR 失敗: %v", err)
	}
	mask := net.IP(ipNet.Mask).String()

	cmd4 := exec.Command("netsh", "interface", "ip", "set", "address", fmt.Sprintf("name=%s", name), "static", ip.String(), mask)
	if output, err := cmd4.CombinedOutput(); err != nil {
		sugar.Warnf("配置 Windows IPv4 警告 (%s): %v, 輸出: %s", v4CIDR, err, strings.TrimSpace(string(output)))
	}

	if v6CIDR != "" {
		cmd6 := exec.Command("netsh", "interface", "ipv6", "add", "address", fmt.Sprintf("interface=%s", name), v6CIDR)
		if output, err := cmd6.CombinedOutput(); err != nil {
			sugar.Warnf("配置 Windows IPv6 警告 (%s): %v, 輸出: %s", v6CIDR, err, strings.TrimSpace(string(output)))
		}
	}

	sugar.Infof("網卡 %s 已配置 (Windows): IPv4=%s, IPv6=%s", name, v4CIDR, v6CIDR)
	return nil
}

func setupPolicyRouting(tapName string, mark int, gwV4, gwV6 string) {
	if mark <= 0 {
		return
	}
	sugar.Warnf("⚠️ Windows 平台不支援 fwmark (%d) 策略路由", mark)
}

func cleanPolicyRouting(tapName string, mark int, gwV4, gwV6 string) {
	if mark <= 0 {
		return
	}
}
