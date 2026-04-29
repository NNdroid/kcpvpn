package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const MTU = 2048

var sugar *zap.SugaredLogger

// ================= 0. 日誌與工具 =================

func initLogger(levelStr string) {
	level := zap.InfoLevel
	switch strings.ToLower(levelStr) {
	case "debug":
		level = zap.DebugLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("初始化 Zap 日誌失敗: %v", err))
	}
	sugar = logger.Sugar()
}

// 解析 CIDR 並推算最大可用主機數
func parseNetworks(v4Str, v6Str string) (*net.IPNet, *net.IPNet, uint32, error) {
	ip4, v4Net, err := net.ParseCIDR(v4Str)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("IPv4 解析失敗: %v", err)
	}
	v4Net.IP = ip4.Mask(v4Net.Mask) // 確保對齊網路地址 base

	var v6Net *net.IPNet
	if v6Str != "" {
		ip6, parsedV6Net, err := net.ParseCIDR(v6Str)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("IPv6 解析失敗: %v", err)
		}
		v6Net = parsedV6Net
		v6Net.IP = ip6.Mask(v6Net.Mask)
	}

	ones, bits := v4Net.Mask.Size()
	if bits != 32 || ones > 30 {
		return nil, nil, 0, fmt.Errorf("無效的 IPv4 遮罩 (需小於等於 /30)")
	}
	
	maxHosts := (uint32(1) << (32 - ones)) - 2 // 扣除 Network 與 Broadcast
	return v4Net, v6Net, maxHosts, nil
}

// 根據偏移量計算 IPv4
func getIP4ByOffset(netIP *net.IPNet, offset uint32) string {
	if netIP == nil {
		return ""
	}
	base := binary.BigEndian.Uint32(netIP.IP.To4())
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, base+offset)
	return ip.String()
}

// 根據偏移量計算 IPv6
func getIP6ByOffset(netIP *net.IPNet, offset uint32) string {
	if netIP == nil {
		return ""
	}
	res := make(net.IP, len(netIP.IP))
	copy(res, netIP.IP)
	var carry uint32 = offset
	for i := len(res) - 1; i >= 0 && carry > 0; i-- {
		sum := uint32(res[i]) + carry
		res[i] = byte(sum)
		carry = sum >> 8
	}
	return res.String()
}

// ================= 1. 幀處理器 (Framer) =================

func writeFrame(conn net.Conn, data []byte) error {
	buf := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(buf[:2], uint16(len(data)))
	if len(data) > 0 {
		copy(buf[2:], data)
	}
	_, err := conn.Write(buf)
	return err
}

func readFrame(conn net.Conn) ([]byte, error) {
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(lenBuf)

	if length > MTU*2 {
		return nil, fmt.Errorf("數據幀過大: %d", length)
	}
	if length == 0 {
		return []byte{}, nil
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}

// ================= 2. 基礎組件 =================

type HandshakeMsg struct {
	Type      string `json:"type"`
	IPv4      string `json:"ipv4,omitempty"`
	IPv6      string `json:"ipv6,omitempty"`
	IPv4Mask  int    `json:"ipv4_mask,omitempty"` // 🌟 動態下發遮罩長度
	IPv6Mask  int    `json:"ipv6_mask,omitempty"`
	GatewayV4 string `json:"gateway_v4,omitempty"`
	GatewayV6 string `json:"gateway_v6,omitempty"`
	MAC       string `json:"mac,omitempty"`
}

type IPRecord struct {
	MAC  string
	Conn net.Conn
}

type IPAM struct {
	mu       sync.Mutex
	v4Net    *net.IPNet
	v6Net    *net.IPNet
	maxHosts uint32
	usedIDs  map[uint32]IPRecord
}

func NewIPAM(v4Net, v6Net *net.IPNet, maxHosts uint32) *IPAM {
	return &IPAM{
		v4Net:    v4Net,
		v6Net:    v6Net,
		maxHosts: maxHosts,
		usedIDs:  make(map[uint32]IPRecord),
	}
}

func (i *IPAM) Allocate(mac string, conn net.Conn) (uint32, string, string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// 1. 斷線接管
	for offset, rec := range i.usedIDs {
		if rec.MAC == mac {
			i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
			return offset, getIP4ByOffset(i.v4Net, offset), getIP6ByOffset(i.v6Net, offset), nil
		}
	}

	// 2. 尋找空閒 ID (從 2 到 maxHosts，1 保留給 Server)
	for offset := uint32(2); offset <= i.maxHosts; offset++ {
		if _, taken := i.usedIDs[offset]; !taken {
			i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
			return offset, getIP4ByOffset(i.v4Net, offset), getIP6ByOffset(i.v6Net, offset), nil
		}
	}
	return 0, "", "", fmt.Errorf("IP 地址池已耗盡 (最大容量: %d)", i.maxHosts-1)
}

func (i *IPAM) Reserve(v4Str string, mac string, conn net.Conn) (uint32, error) {
	reqIP := net.ParseIP(v4Str).To4()
	if reqIP == nil {
		return 0, fmt.Errorf("無效的 IPv4 格式: %s", v4Str)
	}

	if !i.v4Net.Contains(reqIP) {
		return 0, fmt.Errorf("請求的 IP (%s) 不在網段內", v4Str)
	}

	base := binary.BigEndian.Uint32(i.v4Net.IP.To4())
	reqInt := binary.BigEndian.Uint32(reqIP)
	offset := reqInt - base

	if offset < 2 || offset > i.maxHosts {
		return 0, fmt.Errorf("保留的 IP (%s) 是網段保留地址 (網關或廣播)", v4Str)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if rec, taken := i.usedIDs[offset]; taken {
		if rec.MAC == mac {
			i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
			return offset, nil
		}
		return 0, fmt.Errorf("指定的 IP (%s) 已被其他設備佔用", v4Str)
	}
	i.usedIDs[offset] = IPRecord{MAC: mac, Conn: conn}
	return offset, nil
}

func (i *IPAM) Release(offset uint32, conn net.Conn) {
	if offset <= 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if rec, taken := i.usedIDs[offset]; taken && rec.Conn == conn {
		delete(i.usedIDs, offset)
		sugar.Debugf("♻️ [IPAM] 已回收 IP 偏移量: +%d", offset)
	}
}

// [VSwitch 的代碼保持不變，省略細節以免過長，你原本寫得很好]
type VSwitch struct {
	tap      *water.Interface
	clients  map[net.Conn]chan []byte
	macTable map[string]net.Conn
	mu       sync.RWMutex
}

func NewVSwitch(tap *water.Interface) *VSwitch {
	return &VSwitch{
		tap:      tap,
		clients:  make(map[net.Conn]chan []byte),
		macTable: make(map[string]net.Conn),
	}
}

func (s *VSwitch) Register(mac string, conn net.Conn) {
	s.mu.Lock()
	if oldConn, exists := s.macTable[mac]; exists && oldConn != conn {
		sugar.Warnf("⚠️ 發現相同 MAC (%s) 登入，踢出舊連接", mac)
		oldConn.Close()
	}
	sendChan := make(chan []byte, 1024)
	s.clients[conn] = sendChan
	s.macTable[mac] = conn
	s.mu.Unlock()
	go func() {
		for packet := range sendChan {
			if err := writeFrame(conn, packet); err != nil {
				return
			}
		}
	}()
}

func (s *VSwitch) Unregister(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.clients[conn]; ok {
		close(ch)
		delete(s.clients, conn)
	}
	for mac, c := range s.macTable {
		if c == conn {
			delete(s.macTable, mac)
		}
	}
	conn.Close()
}

func (s *VSwitch) Forward(packet []byte, sender net.Conn) {
	if len(packet) < 14 {
		return
	}
	dstMAC := string(packet[0:6])
	srcMAC := string(packet[6:12])

	s.mu.Lock()
	if sender != nil {
		s.macTable[srcMAC] = sender
	}
	s.mu.Unlock()

	pktCopy := make([]byte, len(packet))
	copy(pktCopy, packet)

	s.mu.RLock()
	defer s.mu.RUnlock()

	isMulticast := (packet[0] & 1) == 1
	if isMulticast {
		for conn, ch := range s.clients {
			if conn != sender {
				select {
				case ch <- pktCopy:
				default:
				}
			}
		}
	} else {
		if targetConn, ok := s.macTable[dstMAC]; ok && targetConn != sender {
			if ch, exists := s.clients[targetConn]; exists {
				select {
				case ch <- pktCopy:
				default:
				}
			}
		} else {
			for conn, ch := range s.clients {
				if conn != sender {
					select {
					case ch <- pktCopy:
					default:
					}
				}
			}
		}
	}
}

// ================= 3. 系統輔助函數 =================

// 🌟 此處的 CIDR 已經包含了遮罩長度 (如 10.8.0.2/22)
func setupInterface(name, v4CIDR, v6CIDR string) {
	exec.Command("ip", "addr", "replace", v4CIDR, "dev", name).Run()
	if v6CIDR != "" {
		exec.Command("ip", "addr", "replace", v6CIDR, "dev", name).Run()
	}
	exec.Command("ip", "link", "set", name, "up").Run()
	sugar.Infof("網卡 %s 已配置: IPv4=%s, IPv6=%s", name, v4CIDR, v6CIDR)
}

func getMacAddr(name string, specified string) string {
	if specified != "" {
		if _, err := net.ParseMAC(specified); err == nil {
			return specified
		}
		sugar.Warnf("⚠️ 指定的 MAC (%s) 格式錯誤", specified)
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "02:00:00:00:00:01"
	}
	return iface.HardwareAddr.String()
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

// ================= 4. 主程式 =================

func main() {
	mode := flag.String("mode", "server", "server/client")
	addrStr := flag.String("addr", "127.0.0.1:4000", "listen/dial address")
	tapName := flag.String("tap", "tap0", "tap interface name")
	// 🌟 升級為接受 CIDR
	v4NetStr := flag.String("v4net", "10.0.0.0/24", "IPv4 CIDR (e.g. 10.8.1.0/22)")
	v6NetStr := flag.String("v6net", "fd00::/64", "IPv6 CIDR (e.g. fcee:1232:3334:4556::/96)")
	fecData := flag.Int("fec-d", 5, "FEC Data Shards")
	fecParity := flag.Int("fec-p", 2, "FEC Parity Shards")

	clientIP4 := flag.String("ip4", "", "Client specify IPv4")
	clientIP6 := flag.String("ip6", "", "Client specify IPv6")
	clientMAC := flag.String("mac", "", "Client specify MAC address")

	cryptMode := flag.String("crypt", "salsa20", "加密方式: aes 或 salsa20")
	fwmark := flag.Int("fwmark", 0, "設定 fwmark 啟用策略路由 (例如 1911)")
	logLevel := flag.String("loglevel", "warn", "日誌級別: debug, info, warn, error")
	psk := flag.String("psk", "HereIsYourSuperSecretPassword", "預共享密鑰")

	flag.Parse()
	initLogger(*logLevel)
	defer sugar.Sync()

	config := water.Config{DeviceType: water.TAP}
	config.Name = *tapName
	iface, err := water.New(config)
	if err != nil {
		sugar.Fatalf("創建 TAP 網卡失敗: %v", err)
	}

	key := sha256.Sum256([]byte(*psk))
	var block kcp.BlockCrypt
	if strings.ToLower(*cryptMode) == "salsa20" {
		block, _ = kcp.NewSalsa20BlockCrypt(key[:])
	} else if strings.ToLower(*cryptMode) == "aes" {
		block, _ = kcp.NewAESBlockCrypt(key[:])
	} else {
		block = nil // 完全不加密，用明文傳輸測試
		sugar.Warnf("⚠️ 警告：目前處於無加密測試模式")
	}
	// 🌟 動態解析網段與容量
	v4Net, v6Net, maxHosts, err := parseNetworks(*v4NetStr, *v6NetStr)
	if err != nil {
		sugar.Fatalf("網段配置錯誤: %v", err)
	}
	
	v4MaskSize, _ := v4Net.Mask.Size()
	v6MaskSize := 0
	if v6Net != nil {
		v6MaskSize, _ = v6Net.Mask.Size()
	}

	// 伺服器固定分配 Offset = 1
	serverV4 := getIP4ByOffset(v4Net, 1)
	serverV6 := getIP6ByOffset(v6Net, 1)

	// 組合含遮罩的字串給 ip addr 指令用
	serverV4CIDR := fmt.Sprintf("%s/%d", serverV4, v4MaskSize)
	serverV6CIDR := ""
	if v6Net != nil {
		serverV6CIDR = fmt.Sprintf("%s/%d", serverV6, v6MaskSize)
	}

	if *mode == "server" {
		setupInterface(*tapName, serverV4CIDR, serverV6CIDR)
		sugar.Infof("網段容量: %d 個可用客戶端 IP", maxHosts-1)
		runServer(iface, *addrStr, v4Net, v6Net, maxHosts, serverV4, serverV6, v4MaskSize, v6MaskSize, block, *fecData, *fecParity)
	} else {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			sugar.Infof("🛑 收到關閉訊號，程式準備退出...")
			os.Exit(0)
		}()
		runClient(iface, *addrStr, block, *fecData, *fecParity, *clientIP4, *clientIP6, *clientMAC, *fwmark)
	}
}

// ================= 5. 伺服器端 =================

func runServer(tap *water.Interface, addr string, v4Net, v6Net *net.IPNet, maxHosts uint32, srvV4, srvV6 string, v4Mask, v6Mask int, block kcp.BlockCrypt, fecD, fecP int) {
	listener, err := kcp.ListenWithOptions(addr, block, fecD, fecP)
	if err != nil {
		sugar.Fatalf("KCP 監聽失敗: %v", err)
	}

	listener.SetReadBuffer(4 * 1024 * 1024)
	listener.SetWriteBuffer(4 * 1024 * 1024)

	sugar.Infof("🚀 KCP VPN 伺服器運行中 (監聽: %s, 窗口: 4096)", addr)

	ipam := NewIPAM(v4Net, v6Net, maxHosts)
	vswitch := NewVSwitch(tap)

	go func() {
		buf := make([]byte, MTU)
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

			kcpConn.SetWindowSize(4096, 4096)
			kcpConn.SetNoDelay(1, 10, 2, 1)

			kcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			reqBytes, err := readFrame(kcpConn)
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

			// 🌟 下發 IP 及其子網路遮罩與 Gateway
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
			writeFrame(kcpConn, respData)

			sugar.Infof("✅ 客戶端連線: MAC=%s -> IP: %s (Offset: %d)", req.MAC, v4, offset)

			vswitch.Register(req.MAC, kcpConn)
			defer vswitch.Unregister(kcpConn)

			for {
				kcpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
				data, err := readFrame(kcpConn)
				if err != nil {
					sugar.Infof("🔌 連線中斷 (MAC=%s): %v", req.MAC, err)
					return
				}
				if len(data) == 0 {
					writeFrame(kcpConn, nil)
					continue
				}
				tap.Write(data)
				vswitch.Forward(data, kcpConn)
			}
		}(conn)
	}
}

// ================= 6. 客戶端 =================

func runClient(tap *water.Interface, serverAddr string, block kcp.BlockCrypt, fecD, fecP int, reqV4, reqV6, reqMac string, fwmark int) {
	tapChan := make(chan []byte, 100)

	go func() {
		buf := make([]byte, MTU)
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

	for {
		sugar.Infof("正在連接伺服器 %s...", serverAddr)

		kcpConn, err := kcp.DialWithOptions(serverAddr, block, fecD, fecP)
		if err != nil {
			sugar.Warnf("連接失敗，3秒後重試...")
			time.Sleep(3 * time.Second)
			continue
		}

		kcpConn.SetReadBuffer(4 * 1024 * 1024)
		kcpConn.SetWriteBuffer(4 * 1024 * 1024)
		kcpConn.SetWindowSize(4096, 4096)
		kcpConn.SetNoDelay(1, 10, 2, 1)

		reqData, _ := json.Marshal(HandshakeMsg{
			Type: "request",
			MAC:  macAddr,
			IPv4: reqV4,
			IPv6: reqV6,
		})

		if err := writeFrame(kcpConn, reqData); err != nil {
			kcpConn.Close()
			continue
		}

		kcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		respBytes, err := readFrame(kcpConn)
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

		// 🌟 使用伺服器下發的遮罩長度來配置網卡 (例如拼接成 10.8.0.2/22)
		clientV4CIDR := fmt.Sprintf("%s/%d", resp.IPv4, resp.IPv4Mask)
		clientV6CIDR := ""
		if resp.IPv6 != "" {
			clientV6CIDR = fmt.Sprintf("%s/%d", resp.IPv6, resp.IPv6Mask)
		}

		setupInterface(tap.Name(), clientV4CIDR, clientV6CIDR)
		sugar.Infof("✅ 隧道建立！IP: %s, 網關: %s", clientV4CIDR, resp.GatewayV4)

		setupPolicyRouting(tap.Name(), fwmark, resp.GatewayV4, resp.GatewayV6)
		defer cleanPolicyRouting(tap.Name(), fwmark, resp.GatewayV4, resp.GatewayV6)

		dropChan := make(chan struct{})

		go func(c net.Conn) {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-dropChan:
					return
				case <-ticker.C:
					if err := writeFrame(c, nil); err != nil {
						return
					}
				}
			}
		}(kcpConn)

		go func(c net.Conn) {
			defer close(dropChan)
			for {
				c.SetReadDeadline(time.Now().Add(30 * time.Second))
				data, err := readFrame(c)
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
				if err := writeFrame(kcpConn, packet); err != nil {
					break NetworkLoop
				}
			}
		}

		cleanPolicyRouting(tap.Name(), fwmark, resp.GatewayV4, resp.GatewayV6)
		kcpConn.Close()
		time.Sleep(3 * time.Second)
	}
}