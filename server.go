package main

import (
	"encoding/json"
	"net"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
)

func runServer(tap *water.Interface, addr string, v4Net, v6Net *net.IPNet, maxHosts uint32, srvV4, srvV6 string, v4Mask, v6Mask int, block kcp.BlockCrypt, fecD, fecP int, mtu, sndwnd, rcvwnd, dscp int, obfsMode string, key []byte, padMax int) {
	listener, err := kcp.ListenWithOptions(addr, block, fecD, fecP)
	if err != nil {
		sugar.Fatalf("KCP 監聽失敗: %v", err)
	}

	if dscp > 0 {
		listener.SetDSCP(dscp)
	}
	listener.SetReadBuffer(4 * 1024 * 1024)
	listener.SetWriteBuffer(4 * 1024 * 1024)

	sugar.Infof("🚀 KCP VPN 伺服器運行中 (監聽: %s, 窗口: %d, MTU: %d, 偽裝模式: %s, 最大填充: %d B)", addr, sndwnd, mtu, obfsMode, padMax)

	ipam := NewIPAM(v4Net, v6Net, maxHosts)
	vswitch := NewVSwitch(tap)

	go func() {
		buf := make([]byte, mtu*2)
		for {
			n, err := tap.Read(buf)
			if err == nil {
				vswitch.Forward(buf[:n], nil)
			}
		}
	}()

	for {
		conn, err := listener.AcceptKCP()
		if err != nil {
			continue
		}

		go func(kcpConn *kcp.UDPSession) {
			defer func() {
				if r := recover(); r != nil {
					sugar.Errorf("協程崩潰: %v", r)
				}
			}()

			kcpConn.SetWindowSize(sndwnd, rcvwnd)
			kcpConn.SetNoDelay(1, 10, 2, 1)
			kcpConn.SetMtu(mtu)

			kcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			reqBytes, err := readFrame(kcpConn, mtu, obfsMode, key)
			kcpConn.SetReadDeadline(time.Time{})
			if err != nil {
				kcpConn.Close()
				return
			}

			var req HandshakeMsg
			if err := json.Unmarshal(reqBytes, &req); err != nil || req.Type != "request" {
				kcpConn.Close()
				return
			}

			var offset uint32
			var v4, v6 string

			if req.IPv4 != "" {
				v4, v6 = req.IPv4, req.IPv6
				offset, err = ipam.Reserve(v4, req.MAC, kcpConn)
				if err != nil {
					sugar.Warnf("⚠️ 拒絕連線 (%s): %v", req.MAC, err)
					kcpConn.Close()
					return
				}
			} else {
				offset, v4, v6, err = ipam.Allocate(req.MAC, kcpConn)
				if err != nil {
					sugar.Warnf("⚠️ 拒絕連線 (%s): %v", req.MAC, err)
					kcpConn.Close()
					return
				}
			}

			defer ipam.Release(offset, kcpConn)

			resp := HandshakeMsg{
				Type:      "response",
				IPv4:      v4,
				IPv6:      v6,
				IPv4Mask:  v4Mask,
				IPv6Mask:  v6Mask,
				GatewayV4: srvV4,
				GatewayV6: srvV6,
			}
			respData, _ := json.Marshal(resp)
			if err := writeFrame(kcpConn, respData, obfsMode, key, padMax); err != nil {
				sugar.Warnf("⚠️ 發送握手響應失敗: %v", err)
				kcpConn.Close()
				return
			}

			sugar.Infof("✅ 客戶端連線: MAC=%s -> IP: %s (Offset: %d)", req.MAC, v4, offset)

			vswitch.Register(req.MAC, kcpConn, obfsMode, key, padMax)
			defer vswitch.Unregister(kcpConn)

			for {
				kcpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
				data, err := readFrame(kcpConn, mtu, obfsMode, key)
				if err != nil {
					sugar.Infof("🔌 連線中斷 (MAC=%s): %v", req.MAC, err)
					return
				}
				if len(data) == 0 {
					writeFrame(kcpConn, nil, obfsMode, key, padMax)
					continue
				}
				tap.Write(data)
				vswitch.Forward(data, kcpConn)
			}
		}(conn)
	}
}
