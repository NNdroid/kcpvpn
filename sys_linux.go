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

func setupInterface(name, v4CIDR, v6CIDR string) error {
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

	cmdUp := exec.Command("ip", "link", "set", name, "up")
	if output, err := cmdUp.CombinedOutput(); err != nil {
		return fmt.Errorf("啟用網卡失敗 (%s): %v, 輸出: %s", name, err, string(output))
	}

	sugar.Infof("網卡 %s 已配置: IPv4=%s, IPv6=%s", name, v4CIDR, v6CIDR)
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
