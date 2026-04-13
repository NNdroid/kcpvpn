package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
)

const MTU = 2048

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
	
	// 🌟 長度為 0 的幀被視為 Keepalive 幀
	if length == 0 {
		return []byte{}, nil
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}

// ================= 2. 基礎組件 (IPAM 升級版、VSwitch) =================

type HandshakeMsg struct {
	Type string `json:"type"`
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
	MAC  string `json:"mac"`
}

type IPRecord struct {
	MAC  string
	Conn net.Conn
}

// 🌟 優化後的 IPAM (支援 MAC 綁定與斷線接管)
type IPAM struct {
	mu         sync.Mutex
	ipv4Prefix string
	ipv6Prefix string
	usedIDs    map[int]IPRecord // 記錄 2~254 中 IP 的歸屬者 (MAC 與 連線)
}

func NewIPAM(v4, v6 string) *IPAM {
	return &IPAM{
		ipv4Prefix: v4,
		ipv6Prefix: v6,
		usedIDs:    make(map[int]IPRecord),
	}
}

// Allocate 動態分配。若該 MAC 已有分配紀錄，直接返回舊 IP 實現無縫接管
func (i *IPAM) Allocate(mac string, conn net.Conn) (int, string, string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// 1. 檢查是否是斷線重連，直接接管舊 IP
	for id, rec := range i.usedIDs {
		if rec.MAC == mac {
			i.usedIDs[id] = IPRecord{MAC: mac, Conn: conn} // 更新連線指針
			v4 := fmt.Sprintf("%s.%d", i.ipv4Prefix, id)
			v6 := fmt.Sprintf("%s%x", i.ipv6Prefix, id)
			return id, v4, v6, nil
		}
	}

	// 2. 分配新 IP
	for id := 2; id <= 254; id++ {
		if _, taken := i.usedIDs[id]; !taken {
			i.usedIDs[id] = IPRecord{MAC: mac, Conn: conn}
			v4 := fmt.Sprintf("%s.%d", i.ipv4Prefix, id)
			v6 := fmt.Sprintf("%s%x", i.ipv6Prefix, id)
			return id, v4, v6, nil
		}
	}
	return 0, "", "", fmt.Errorf("IP 地址池已耗盡")
}

// Reserve 保留指定 IP。如果該 IP 被佔用但 MAC 匹配，允許搶佔
func (i *IPAM) Reserve(v4 string, mac string, conn net.Conn) (int, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	parts := strings.Split(v4, ".")
	if len(parts) == 4 {
		id, err := strconv.Atoi(parts[3])
		if err == nil && id >= 2 && id <= 254 {
			if rec, taken := i.usedIDs[id]; taken {
				if rec.MAC == mac {
					// MAC 匹配，允許搶佔並更新連線指針
					i.usedIDs[id] = IPRecord{MAC: mac, Conn: conn}
					return id, nil
				}
				return -1, fmt.Errorf("指定的 IP (%s) 已被其他設備佔用", v4)
			}
			i.usedIDs[id] = IPRecord{MAC: mac, Conn: conn}
			return id, nil
		}
	}
	return -1, nil // 不在範圍內的自定義網段
}

// Release 釋放 IP。為了防止搶佔後，舊協程崩潰錯誤釋放新連接的 IP，只允許"現任連接"釋放資源
func (i *IPAM) Release(id int, conn net.Conn) {
	if id <= 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if rec, taken := i.usedIDs[id]; taken {
		if rec.Conn == conn { // 🌟 嚴格檢查：只有當前持有人可以釋放
			delete(i.usedIDs, id)
			log.Printf("♻️ [IPAM] 已回收 IP 結尾: .%d，返回地址池", id)
		}
	}
}

// 🌟 包含獨立發送通道與自動踢人機制的 VSwitch
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
	// 🌟 如果 MAC 已存在於交換機，主動關閉舊的假死連接 (Kick off)
	if oldConn, exists := s.macTable[mac]; exists {
		if oldConn != conn {
			log.Printf("⚠️ 發現相同 MAC (%s) 重新登入，踢出舊連接", mac)
			oldConn.Close()
		}
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
	// 只有當 MAC 對應的依然是此連接時，才清理路由
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
		if targetConn, ok := s.macTable[dstMAC]; ok {
			if targetConn != sender {
				if ch, exists := s.clients[targetConn]; exists {
					select {
					case ch <- pktCopy:
					default:
					}
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

// ================= 3. 系統與加密輔助函數 =================

func generateCertificate() tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"access.139.com"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 3650),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	return tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}
}

func setupInterface(name, ipv4, ipv6 string) {
	exec.Command("ip", "addr", "replace", ipv4+"/24", "dev", name).Run()
	if ipv6 != "" {
		exec.Command("ip", "addr", "replace", ipv6+"/64", "dev", name).Run()
	}
	exec.Command("ip", "link", "set", name, "up").Run()
	log.Printf("網卡 %s 已配置: IPv4=%s, IPv6=%s\n", name, ipv4, ipv6)
}

func getMacAddr(name string, specified string) string {
	if specified != "" {
		if _, err := net.ParseMAC(specified); err == nil {
			return specified
		}
		log.Printf("⚠️ 指定的 MAC (%s) 格式錯誤，將使用網卡硬體 MAC", specified)
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "02:00:00:00:00:01" // Fallback MAC
	}
	return iface.HardwareAddr.String()
}

// ================= 4. 主程序 =================

func main() {
	mode := flag.String("mode", "server", "server/client")
	addrStr := flag.String("addr", "127.0.0.1:4000", "listen/dial address")
	tapName := flag.String("tap", "tap0", "tap interface name")
	v4Net := flag.String("v4net", "10.0.0", "IPv4 Prefix (Server)")
	v6Net := flag.String("v6net", "fd00::", "IPv6 Prefix (Server)")
	fecData := flag.Int("fec-d", 5, "FEC Data Shards")
	fecParity := flag.Int("fec-p", 2, "FEC Parity Shards")

	clientIP4 := flag.String("ip4", "", "Client specify IPv4")
	clientIP6 := flag.String("ip6", "", "Client specify IPv6")
	clientMAC := flag.String("mac", "", "Client specify MAC address (e.g. 02:11:22:33:44:55)")
	flag.Parse()

	config := water.Config{DeviceType: water.TAP}
	config.Name = *tapName
	iface, _ := water.New(config)

	if *mode == "server" {
		serverV4 := *v4Net + ".1"
		serverV6 := *v6Net + "1"
		setupInterface(*tapName, serverV4, serverV6)
		runServer(iface, *addrStr, *v4Net, *v6Net, *fecData, *fecParity)
	} else {
		runClient(iface, *addrStr, *fecData, *fecParity, *clientIP4, *clientIP6, *clientMAC)
	}
}

// ================= 5. 伺服器端 =================

func runServer(tap *water.Interface, addr, v4p, v6p string, fecD, fecP int) {
	listener, err := kcp.ListenWithOptions(addr, nil, fecD, fecP)
	if err != nil {
		log.Fatalf("KCP 監聽失敗: %v", err)
	}

	log.Printf("🛡️ KCP+TLS (ARQ+FEC) 伺服器運行中 (監聽: %s)", addr)

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{generateCertificate()}}
	tlsListener := tls.NewListener(listener, tlsConfig)

	ipam := NewIPAM(v4p, v6p)
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
		conn, err := tlsListener.Accept()
		if err != nil {
			continue
		}

		go func(c net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("協程崩潰: %v", r)
				}
			}()

			if kcpConn, ok := c.(*tls.Conn).NetConn().(*kcp.UDPSession); ok {
				kcpConn.SetNoDelay(1, 10, 2, 1)
				kcpConn.SetWindowSize(1024, 1024)
			}

			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			reqBytes, err := readFrame(c)
			c.SetReadDeadline(time.Time{})

			if err != nil {
				c.Close()
				return
			}

			var req HandshakeMsg
			if err := json.Unmarshal(reqBytes, &req); err != nil || req.Type != "request" {
				c.Close()
				return
			}

			var ipID int
			var v4, v6 string

			if req.IPv4 != "" {
				v4 = req.IPv4
				v6 = req.IPv6
				ipID, err = ipam.Reserve(v4, req.MAC, c)
				if err != nil {
					log.Printf("⚠️ 拒絕連線 (%s): %v", req.MAC, err)
					c.Close()
					return
				}
				log.Printf("🔧 客戶端請求自定義 IP: %s", v4)
			} else {
				ipID, v4, v6, err = ipam.Allocate(req.MAC, c)
				if err != nil {
					log.Printf("⚠️ 拒絕連線 (%s): %v", req.MAC, err)
					c.Close()
					return
				}
			}

			// 傳入 c 作為驗證，確保只有這個連接結束時能釋放這組 IP
			defer ipam.Release(ipID, c)

			resp := HandshakeMsg{Type: "response", IPv4: v4, IPv6: v6}
			respData, _ := json.Marshal(resp)
			writeFrame(c, respData)

			log.Printf("✅ 客戶端連線成功: MAC=%s -> IP: %s\n", req.MAC, v4)

			vswitch.Register(req.MAC, c)
			defer vswitch.Unregister(c)

			// 🌟 讀取循環，加入 30 秒 Keepalive 絕對超時
			for {
				c.SetReadDeadline(time.Now().Add(30 * time.Second))
				data, err := readFrame(c)
				if err != nil {
					log.Printf("🔌 連線中斷 (MAC=%s): %v", req.MAC, err)
					return // 觸發 defer 進行資源回收
				}
				
				// 收到空數據包代表 Keepalive Ping，立刻回覆 Pong
				if len(data) == 0 {
					writeFrame(c, nil) 
					continue
				}

				tap.Write(data)
				vswitch.Forward(data, c)
			}
		}(conn)
	}
}

// ================= 6. 客戶端 =================

func runClient(tap *water.Interface, serverAddr string, fecD, fecP int, reqV4, reqV6, reqMac string) {
	tapChan := make(chan []byte, 100)

	go func() {
		buf := make([]byte, MTU)
		for {
			n, err := tap.Read(buf)
			if err != nil {
				log.Fatalf("TAP讀取失敗: %v", err)
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
	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	for {
		log.Printf("正在連接伺服器 %s...", serverAddr)

		kcpConn, err := kcp.DialWithOptions(serverAddr, nil, fecD, fecP)
		if err != nil {
			log.Printf("連接失敗，3秒後重試...")
			time.Sleep(3 * time.Second)
			continue
		}

		kcpConn.SetNoDelay(1, 10, 2, 1)
		kcpConn.SetWindowSize(1024, 1024)

		conn := tls.Client(kcpConn, tlsConfig)

		reqData, _ := json.Marshal(HandshakeMsg{
			Type: "request",
			MAC:  macAddr,
			IPv4: reqV4,
			IPv6: reqV6,
		})

		if err := writeFrame(conn, reqData); err != nil {
			conn.Close()
			continue
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		respBytes, err := readFrame(conn)
		conn.SetReadDeadline(time.Time{})

		if err != nil {
			log.Printf("❌ 握手失敗: %v", err)
			conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		var resp HandshakeMsg
		if err := json.Unmarshal(respBytes, &resp); err != nil || resp.IPv4 == "" {
			conn.Close()
			continue
		}

		setupInterface(tap.Name(), resp.IPv4, resp.IPv6)
		log.Printf("✅ 隧道建立成功！MAC: %s -> IP: %s\n", macAddr, resp.IPv4)

		dropChan := make(chan struct{})

		// 🌟 啟動 Keepalive Ping 協程：每 10 秒發送一次空幀
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
		}(conn)

		// 讀取伺服器資料
		go func(c net.Conn) {
			defer close(dropChan) // 一旦斷開，通知其他協程停止
			for {
				// 客戶端同樣具備 30 秒讀取超時機制 (如果伺服器死機/網路斷開，客戶端會自動重撥)
				c.SetReadDeadline(time.Now().Add(30 * time.Second))
				data, err := readFrame(c)
				if err != nil {
					log.Printf("⚠️ 失去連線: %v", err)
					return 
				}
				
				// 收到了伺服器回覆的 Pong，重置了 ReadDeadline，不處理
				if len(data) == 0 {
					continue
				}
				
				tap.Write(data)
			}
		}(conn)

	NetworkLoop:
		for {
			select {
			case <-dropChan:
				break NetworkLoop
			case packet := <-tapChan:
				if err := writeFrame(conn, packet); err != nil {
					break NetworkLoop
				}
			}
		}

		conn.Close()
		time.Sleep(3 * time.Second) // 準備自動遷移/重連
	}
}