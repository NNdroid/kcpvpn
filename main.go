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
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/xtaci/kcp-go/v5"
)

const MTU = 2048

// ================= 1. 幀處理器 (Framer) =================
// 因為 KCP+TLS 是連續的數據流(Stream)，我們需要在每個數據包前加上 2 bytes 的長度
// 這樣接收端才知道一個完整的以太網幀在哪裡結束

func writeFrame(conn net.Conn, data []byte) error {
	buf := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(buf[:2], uint16(len(data)))
	copy(buf[2:], data)
	_, err := conn.Write(buf)
	return err
}

func readFrame(conn net.Conn) ([]byte, error) {
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(lenBuf)
	
	// 避免惡意包導致內存溢出
	if length > MTU*2 {
		return nil, fmt.Errorf("數據幀過大: %d", length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}

// ================= 2. 基礎組件 (握手、IPAM、VSwitch) =================

type HandshakeMsg struct {
	Type string `json:"type"`
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
	MAC  string `json:"mac"`
}

type IPAM struct {
	mu         sync.Mutex
	ipv4Prefix string
	ipv6Prefix string
	nextID     int
}

func NewIPAM(v4, v6 string) *IPAM { return &IPAM{ipv4Prefix: v4, ipv6Prefix: v6, nextID: 2} }
func (i *IPAM) Allocate() (string, string) {
	i.mu.Lock(); defer i.mu.Unlock()
	v4 := fmt.Sprintf("%s.%d", i.ipv4Prefix, i.nextID)
	v6 := fmt.Sprintf("%s%x", i.ipv6Prefix, i.nextID)
	i.nextID++; return v4, v6
}

type VSwitch struct {
	tap      *water.Interface
	clients  map[net.Conn]bool
	macTable map[string]net.Conn
	mu       sync.RWMutex
}

func NewVSwitch(tap *water.Interface) *VSwitch {
	return &VSwitch{tap: tap, clients: make(map[net.Conn]bool), macTable: make(map[string]net.Conn)}
}
func (s *VSwitch) Register(mac string, conn net.Conn) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.clients[conn] = true; s.macTable[mac] = conn
}
func (s *VSwitch) Unregister(conn net.Conn) {
	s.mu.Lock(); defer s.mu.Unlock()
	delete(s.clients, conn)
	for mac, c := range s.macTable { if c == conn { delete(s.macTable, mac) } }
	conn.Close()
}
func (s *VSwitch) Forward(packet []byte, sender net.Conn) {
	if len(packet) < 14 { return }
	dstMAC := string(packet[0:6])
	s.mu.RLock(); defer s.mu.RUnlock()

	isMulticast := (packet[0] & 1) == 1
	if isMulticast {
		for conn := range s.clients { if conn != sender { writeFrame(conn, packet) } }
	} else {
		if target, ok := s.macTable[dstMAC]; ok {
			if target != sender { writeFrame(target, packet) }
		} else {
			for conn := range s.clients { if conn != sender { writeFrame(conn, packet) } }
		}
	}
}

// ================= 3. 系統與加密輔助函數 =================

func generateCertificate() tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{Organization: []string{"access.139.com"}},
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
	exec.Command("ip", "addr", "add", ipv4+"/24", "dev", name).Run()
	exec.Command("ip", "addr", "add", ipv6+"/64", "dev", name).Run()
	exec.Command("ip", "link", "set", name, "up").Run()
	log.Printf("網卡 %s 已配置: IPv4=%s, IPv6=%s\n", name, ipv4, ipv6)
}

func getMacAddr(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil { return "00:00:00:00:00:00" }
	return iface.HardwareAddr.String()
}

// ================= 4. 主程序 =================

func main() {
	mode := flag.String("mode", "server", "server/client")
	addrStr := flag.String("addr", "127.0.0.1:4000", "listen/dial address")
	tapName := flag.String("tap", "tap0", "tap interface name")
	v4Net := flag.String("v4net", "10.0.0", "IPv4 Prefix")
	v6Net := flag.String("v6net", "fd00::", "IPv6 Prefix")
	
	// KCP & FEC 參數
	fecData := flag.Int("fec-d", 5, "FEC Data Shards (原始數據包)")
	fecParity := flag.Int("fec-p", 2, "FEC Parity Shards (冗餘包)")
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
		runClient(iface, *addrStr, *fecData, *fecParity)
	}
}

// ================= 5. 伺服器端 =================

func runServer(tap *water.Interface, addr, v4p, v6p string, fecD, fecP int) {
	// 1. 建立 KCP 監聽 (底層自動處理 UDP 分發、ARQ、FEC、流控)
	listener, err := kcp.ListenWithOptions(addr, nil, fecD, fecP)
	if err != nil { log.Fatalf("KCP 監聽失敗: %v", err) }
	
	log.Printf("🛡️ KCP+TLS (ARQ+FEC) 伺服器運行中 (監聽: %s)", addr)

	// 2. 套用標準 TLS 安全層
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{generateCertificate()}}
	tlsListener := tls.NewListener(listener, tlsConfig)

	ipam := NewIPAM(v4p, v6p)
	vswitch := NewVSwitch(tap)

	// TAP 讀取轉發協程
	go func() {
		buf := make([]byte, MTU)
		for {
			n, err := tap.Read(buf)
			if err == nil { vswitch.Forward(buf[:n], nil) }
		}
	}()

	for {
		conn, err := tlsListener.Accept()
		if err != nil { continue }
		
		go func(c net.Conn) {
			defer func() { if r := recover(); r != nil { log.Printf("協程崩潰: %v", r) } }()

			// 設定 KCP 極速模式 (關閉延遲，啟用激進 ARQ 策略)
			if kcpConn, ok := c.(*tls.Conn).NetConn().(*kcp.UDPSession); ok {
				kcpConn.SetNoDelay(1, 10, 2, 1) 
				kcpConn.SetWindowSize(1024, 1024)
			}

			// 握手階段 (使用 Framer 讀取)
			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			reqBytes, err := readFrame(c)
			c.SetReadDeadline(time.Time{})

			if err != nil { c.Close(); return }

			var req HandshakeMsg
			if err := json.Unmarshal(reqBytes, &req); err != nil || req.Type != "request" {
				c.Close(); return
			}

			// 分配 IP 並回應
			v4, v6 := ipam.Allocate()
			resp := HandshakeMsg{Type: "response", IPv4: v4, IPv6: v6}
			respData, _ := json.Marshal(resp)
			writeFrame(c, respData)

			log.Printf("✅ 客戶端連線成功: MAC=%s -> 分配 IP: %s\n", req.MAC, v4)

			// 註冊並進入轉發迴圈
			vswitch.Register(req.MAC, c)
			defer vswitch.Unregister(c)

			for {
				data, err := readFrame(c)
				if err != nil { return } // 網路斷開，結束協程
				tap.Write(data)
				vswitch.Forward(data, c)
			}
		}(conn)
	}
}

// ================= 6. 客戶端 =================

func runClient(tap *water.Interface, serverAddr string, fecD, fecP int) {
	tapChan := make(chan []byte, 100)
	
	// 獨立的 TAP 讀取器
	go func() {
		buf := make([]byte, MTU)
		for {
			n, err := tap.Read(buf)
			if err != nil { log.Fatalf("TAP讀取失敗: %v", err) }
			packet := make([]byte, n); copy(packet, buf[:n])
			select { case tapChan <- packet: default: }
		}
	}()

	clientMAC := getMacAddr(tap.Name())
	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	for {
		log.Printf("正在連接伺服器 %s...", serverAddr)
		
		// 1. 建立 KCP 連線 (自帶 ARQ 與 FEC)
		kcpConn, err := kcp.DialWithOptions(serverAddr, nil, fecD, fecP)
		if err != nil {
			log.Printf("連接失敗，3秒後重試..."); time.Sleep(3 * time.Second); continue
		}

		// 設定極速重傳與視窗大小
		kcpConn.SetNoDelay(1, 10, 2, 1)
		kcpConn.SetWindowSize(1024, 1024)

		// 2. 升級為 TLS 連線
		conn := tls.Client(kcpConn, tlsConfig)

		// 3. 發起 VPN 握手
		reqData, _ := json.Marshal(HandshakeMsg{Type: "request", MAC: clientMAC})
		if err := writeFrame(conn, reqData); err != nil {
			conn.Close(); continue
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		respBytes, err := readFrame(conn)
		conn.SetReadDeadline(time.Time{})

		if err != nil {
			log.Printf("❌ 握手失敗: %v", err)
			conn.Close(); time.Sleep(3 * time.Second); continue
		}

		var resp HandshakeMsg
		if err := json.Unmarshal(respBytes, &resp); err != nil || resp.IPv4 == "" {
			conn.Close(); continue
		}

		// 自動配置網卡
		setupInterface(tap.Name(), resp.IPv4, resp.IPv6)
		log.Printf("✅ KCP 隧道建立成功！分配 IP: %s\n", resp.IPv4)

		dropChan := make(chan struct{})
		
		// 接收協程
		go func(c net.Conn) {
			defer close(dropChan)
			for {
				data, err := readFrame(c)
				if err != nil { log.Printf("⚠️ 連接斷開: %v", err); return }
				tap.Write(data)
			}
		}(conn)

		// 發送迴圈
	NetworkLoop:
		for {
			select {
			case <-dropChan:
				break NetworkLoop
			case packet := <-tapChan:
				if err := writeFrame(conn, packet); err != nil { break NetworkLoop }
			}
		}
		
		conn.Close()
		time.Sleep(3 * time.Second)
	}
}