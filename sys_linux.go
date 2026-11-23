//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/songgao/water"
)

func createTAP(tapName string) (*water.Interface, error) {
	config := water.Config{DeviceType: water.TAP}
	config.Name = tapName
	return water.New(config)
}

func setupInterface(name, v4CIDR, v6CIDR string, mtu int) error {
	cmd4 := exec.Command("ip", "addr", "replace", v4CIDR, "dev", name)
	if output, err := cmd4.CombinedOutput(); err != nil {
		return fmt.Errorf("配置 IPv4 失敗 (%s): %v, 輸出: %s", v4CIDR, err, string(output))
	}

	if v6CIDR != "" {
		cmd6 := exec.Command("ip", "addr", "replace", v6CIDR, "dev", name)
		if output, err := cmd6.CombinedOutput(); err != nil {
			return fmt.Errorf("配置 IPv6 失敗 (%s): %v, 輸出: %s", v6CIDR, err, string(output))
		}
	}

	if err := setInterfaceMTU(name, mtu); err != nil {
		return err
	}

	cmdUp := exec.Command("ip", "link", "set", name, "up")
	if output, err := cmdUp.CombinedOutput(); err != nil {
		return fmt.Errorf("啟用網卡失敗 (%s): %v, 輸出: %s", name, err, string(output))
	}

	sugar.Infof("網卡 %s 已配置: IPv4=%s, IPv6=%s, MTU=%d", name, v4CIDR, v6CIDR, mtu)
	return nil
}

// setInterfaceMTU 設置網卡 MTU（Linux）。
func setInterfaceMTU(name string, mtu int) error {
	cmdMTU := exec.Command("ip", "link", "set", "dev", name, "mtu", strconv.Itoa(mtu))
	if output, err := cmdMTU.CombinedOutput(); err != nil {
		return fmt.Errorf("設置 MTU 失敗 (%s): %v, 輸出: %s", name, err, string(output))
	}
	return nil
}

// setInterfaceMAC 設置網卡 MAC（Linux，需網卡處於 down 狀態）。
func setInterfaceMAC(name, mac string) error {
	cmd := exec.Command("ip", "link", "set", "dev", name, "address", mac)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("設置 MAC 失敗 (%s): %v, 輸出: %s", name, err, string(output))
	}
	return nil
}

func setupPolicyRouting(tapName string, mark int, gwV4, gwV6 string) {
	if mark <= 0 {
		return
	}
	m := strconv.Itoa(mark)

	if gwV4 != "" {
		exec.Command("ip", "-4", "rule", "del", "fwmark", m, "table", m).Run()
		exec.Command("ip", "-4", "rule", "add", "fwmark", m, "table", m).Run()
		exec.Command("ip", "-4", "route", "replace", "default", "via", gwV4, "dev", tapName, "table", m).Run()
	}
	if gwV6 != "" {
		exec.Command("ip", "-6", "rule", "del", "fwmark", m, "table", m).Run()
		exec.Command("ip", "-6", "rule", "add", "fwmark", m, "table", m).Run()
		exec.Command("ip", "-6", "route", "replace", "default", "via", gwV6, "dev", tapName, "table", m).Run()
	}
	sugar.Infof("🔀 策略路由已設定 (fwmark: %s) -> 網關 V4: %s, 網關 V6: %s", m, gwV4, gwV6)
}

func cleanPolicyRouting(tapName string, mark int, gwV4, gwV6 string) {
	if mark <= 0 {
		return
	}
	m := strconv.Itoa(mark)
	if gwV4 != "" {
		exec.Command("ip", "-4", "rule", "del", "fwmark", m, "table", m).Run()
		exec.Command("ip", "-4", "route", "del", "default", "via", gwV4, "dev", tapName, "table", m).Run()
	}
	if gwV6 != "" {
		exec.Command("ip", "-6", "rule", "del", "fwmark", m, "table", m).Run()
		exec.Command("ip", "-6", "route", "del", "default", "via", gwV6, "dev", tapName, "table", m).Run()
	}
	sugar.Infof("🧹 策略路由已清理 (fwmark: %s)", m)
}
