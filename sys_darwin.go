//go:build darwin

package main

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/songgao/water"
)

func createTAP(tapName string) (*water.Interface, error) {
	config := water.Config{DeviceType: water.TAP}
	config.Name = tapName
	return water.New(config)
}

func setupInterface(name, v4CIDR, v6CIDR string, mtu int) error {
	ip, ipNet, err := net.ParseCIDR(v4CIDR)
	if err != nil {
		return fmt.Errorf("解析 IPv4 CIDR 失敗: %v", err)
	}
	mask := net.IP(ipNet.Mask).String()

	cmd4 := exec.Command("ifconfig", name, ip.String(), ip.String(), "netmask", mask, "up")
	if output, err := cmd4.CombinedOutput(); err != nil {
		sugar.Warnf("配置 macOS IPv4 警告 (%s): %v, 輸出: %s", v4CIDR, err, strings.TrimSpace(string(output)))
	}

	if err := setInterfaceMTU(name, mtu); err != nil {
		sugar.Warnf("設置 macOS MTU 警告 (%s): %v", name, err)
	}

	if v6CIDR != "" {
		cmd6 := exec.Command("ifconfig", name, "inet6", v6CIDR, "add")
		if output, err := cmd6.CombinedOutput(); err != nil {
			sugar.Warnf("配置 macOS IPv6 警告 (%s): %v, 輸出: %s", v6CIDR, err, strings.TrimSpace(string(output)))
		}
	}

	sugar.Infof("網卡 %s 已配置 (macOS): IPv4=%s, IPv6=%s, MTU=%d", name, v4CIDR, v6CIDR, mtu)
	return nil
}

// setInterfaceMTU 設置網卡 MTU（macOS）。
func setInterfaceMTU(name string, mtu int) error {
	cmdMTU := exec.Command("ifconfig", name, "mtu", strconv.Itoa(mtu))
	if output, err := cmdMTU.CombinedOutput(); err != nil {
		return fmt.Errorf("%v, 輸出: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// setInterfaceMAC 設置網卡 MAC（macOS）。
func setInterfaceMAC(name, mac string) error {
	cmd := exec.Command("ifconfig", name, "ether", mac)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v, 輸出: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func setupPolicyRouting(tapName string, mark int, gwV4, gwV6 string) {
	if mark <= 0 {
		return
	}
	sugar.Warnf("⚠️ macOS 平台不支援 fwmark (%d) 策略路由", mark)
}

func cleanPolicyRouting(tapName string, mark int, gwV4, gwV6 string) {
	if mark <= 0 {
		return
	}
}
