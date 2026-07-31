package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
)

func runClient(tap *water.Interface, serverAddr string, block kcp.BlockCrypt, fecD, fecP int, reqV4, reqV6, reqMac string, fwmark int, sigCh chan os.Signal, mtu, sndwnd, rcvwnd, dscp int, obfsMode string, key []byte, padMax int) {
	tapChan := make(chan []byte, 100)

	go func() {
		buf := make([]byte, mtu*2)
		for {
			n, err := tap.Read(buf)
			if err != nil {
				sugar.Fatalf("TAP讀取失敗: %v", err)
			}
			packet := make([]byte, n)
			copy(packet, buf[:n])
			select {
			case tapChan <- packet:
			default:
			}
		}
	}()

	macAddr := getMacAddr(tap.Name(), reqMac)

	var currentGwV4, currentGwV6 string
	var cleanupRoutingOnce sync.Once

	doCleanRouting := func() {
		cleanupRoutingOnce.Do(func() {
			if currentGwV4 != "" || currentGwV6 != "" {
				cleanPolicyRouting(tap.Name(), fwmark, currentGwV4, currentGwV6)
			}
		})
	}

	go func() {
		<-sigCh
		sugar.Infof("🛑 收到關閉訊號，程式清理退出...")
		doCleanRouting()
		os.Exit(0)
	}()

	for {
		sugar.Infof("正在連接伺服器 %s...", serverAddr)

		kcpConn, err := kcp.DialWithOptions(serverAddr, block, fecD, fecP)
		if err != nil {
			sugar.Warnf("連接失敗，3秒後重試...")
			time.Sleep(3 * time.Second)
			continue
		}

		if dscp > 0 {
			kcpConn.SetDSCP(dscp)
		}
		kcpConn.SetReadBuffer(4 * 1024 * 1024)
		kcpConn.SetWriteBuffer(4 * 1024 * 1024)
		kcpConn.SetWindowSize(sndwnd, rcvwnd)
		kcpConn.SetNoDelay(1, 10, 2, 1)
		kcpConn.SetMtu(mtu)

		reqData, _ := json.Marshal(HandshakeMsg{
			Type: "request",
			MAC:  macAddr,
			IPv4: reqV4,
			IPv6: reqV6,
		})

		if err := writeFrame(kcpConn, reqData, obfsMode, key, padMax); err != nil {
			kcpConn.Close()
			continue
		}

		kcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		respBytes, err := readFrame(kcpConn, mtu, obfsMode, key)
		kcpConn.SetReadDeadline(time.Time{})

		if err != nil {
			sugar.Warnf("❌ 握手失敗: %v", err)
			kcpConn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		var resp HandshakeMsg
		if err := json.Unmarshal(respBytes, &resp); err != nil || resp.IPv4 == "" {
			kcpConn.Close()
			continue
		}

		clientV4CIDR := fmt.Sprintf("%s/%d", resp.IPv4, resp.IPv4Mask)
		clientV6CIDR := ""
		if resp.IPv6 != "" {
			clientV6CIDR = fmt.Sprintf("%s/%d", resp.IPv6, resp.IPv6Mask)
		}

		if err := setupInterface(tap.Name(), clientV4CIDR, clientV6CIDR); err != nil {
			sugar.Warnf("⚠️ 客戶端網卡配置警告: %v", err)
		}
		sugar.Infof("✅ 隧道建立！IP: %s, 網關: %s", clientV4CIDR, resp.GatewayV4)

		currentGwV4 = resp.GatewayV4
		currentGwV6 = resp.GatewayV6
		cleanupRoutingOnce = sync.Once{}

		setupPolicyRouting(tap.Name(), fwmark, resp.GatewayV4, resp.GatewayV6)

		dropChan := make(chan struct{})

		go func(c net.Conn) {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-dropChan:
					return
				case <-ticker.C:
					if err := writeFrame(c, nil, obfsMode, key, padMax); err != nil {
						return
					}
				}
			}
		}(kcpConn)

		go func(c net.Conn) {
			defer close(dropChan)
			for {
				c.SetReadDeadline(time.Now().Add(30 * time.Second))
				data, err := readFrame(c, mtu, obfsMode, key)
				if err != nil {
					sugar.Warnf("⚠️ 失去連線: %v", err)
					return
				}
				if len(data) == 0 {
					continue
				}
				tap.Write(data)
			}
		}(kcpConn)

	NetworkLoop:
		for {
			select {
			case <-dropChan:
				break NetworkLoop
			case packet := <-tapChan:
				if err := writeFrame(kcpConn, packet, obfsMode, key, padMax); err != nil {
					break NetworkLoop
				}
			}
		}

		doCleanRouting()
		kcpConn.Close()
		time.Sleep(3 * time.Second)
	}
}
