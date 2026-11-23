package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveBindAddrs(t *testing.T) {
	cfg := &Config{WebBind: "local", ServerV4: "10.0.0.1", ServerV6: "fd00::1"}
	tunnel := func() []string { return []string{cfg.ServerV4, cfg.ServerV6} }

	if got := resolveBindAddrs(cfg, tunnel); len(got) != 2 || got[0] != "127.0.0.1" || got[1] != "::1" {
		t.Fatalf("local addrs wrong: %v", got)
	}

	cfg.WebBind = "tunnel"
	if got := resolveBindAddrs(cfg, tunnel); len(got) != 2 || got[0] != "10.0.0.1" || got[1] != "fd00::1" {
		t.Fatalf("tunnel addrs wrong: %v", got)
	}

	cfg.WebBind = "public"
	if got := resolveBindAddrs(cfg, tunnel); len(got) != 2 || got[0] != "0.0.0.0" || got[1] != "::" {
		t.Fatalf("public addrs wrong: %v", got)
	}

	// tunnel 无 IP 时回退 localhost。
	cfg.WebBind = "tunnel"
	if got := resolveBindAddrs(cfg, func() []string { return nil }); len(got) != 1 || got[0] != "127.0.0.1" {
		t.Fatalf("tunnel empty fallback wrong: %v", got)
	}
}

func TestWebPort(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want int
		ok   bool
	}{
		{":8080", 8080, true},
		{"127.0.0.1:9090", 9090, true},
		{"8080", 8080, true},
		{"[::1]:7000", 7000, true},
		{"nope", 0, false},
		{":0", 0, false},
		{":99999", 0, false},
	} {
		got, err := webPort(tc.addr)
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("webPort(%q) = %d, %v; want %d", tc.addr, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Fatalf("webPort(%q) should error", tc.addr)
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	// token 为空 → 放行。
	uiNoToken := &webUI{cfg: &Config{}}
	rr := httptest.NewRecorder()
	uiNoToken.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})).ServeHTTP(rr, httptest.NewRequest("GET", "/api/status", nil))
	if rr.Code != 200 {
		t.Fatalf("empty token should pass, got %d", rr.Code)
	}

	// token 设置 → 无 Authorization 401。
	ui := &webUI{cfg: &Config{WebToken: "secret"}}
	rr = httptest.NewRecorder()
	ui.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})).ServeHTTP(rr, httptest.NewRequest("GET", "/api/status", nil))
	if rr.Code != 401 {
		t.Fatalf("missing auth should 401, got %d", rr.Code)
	}

	// 错误 token 401。
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	ui.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})).ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("wrong token should 401, got %d", rr.Code)
	}

	// 正确 token 200。
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	ui.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})).ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("correct token should pass, got %d", rr.Code)
	}
}

func TestSanitizeConfig(t *testing.T) {
	cfg := &Config{PSK: "supersecret", WebToken: "mytoken", Mode: "server"}
	m := sanitizeConfig(cfg)
	if m["psk"] != "***" {
		t.Fatalf("psk should be masked, got %v", m["psk"])
	}
	if m["web-token"] != "***" {
		t.Fatalf("web-token should be masked, got %v", m["web-token"])
	}
	if m["mode"] != "server" {
		t.Fatalf("mode should be preserved, got %v", m["mode"])
	}
}

func TestHandleConfigRejectsBadJSON(t *testing.T) {
	ui := &webUI{cfg: &Config{}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/config", strings.NewReader(`not-json`))
	ui.handleConfig(rr, req)
	if rr.Code != 400 {
		t.Fatalf("bad json should 400, got %d", rr.Code)
	}
}

func TestHandleStatusServer(t *testing.T) {
	ui := &webUI{cfg: &Config{MaxClients: 10}, mode: "server", tapName: "tap0"}
	rr := httptest.NewRecorder()
	ui.handleStatus(rr, httptest.NewRequest("GET", "/api/status", nil))
	if rr.Code != 200 {
		t.Fatalf("status should 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"mode":"server"`) || !strings.Contains(body, `"tap":"tap0"`) {
		t.Fatalf("status body unexpected: %s", body)
	}
}

// TestStartWebUIEndToEnd 启动真实 HTTP 服务器（bind=local, 带 token），
// 验证鉴权、状态、首页与配置校验失败路径（不触发重启）。
func TestStartWebUIEndToEnd(t *testing.T) {
	// 选一个空闲端口。
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot probe port: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	cfg := &Config{
		Mode:       "server",
		WebBind:    "local",
		WebAddr:    "127.0.0.1:" + strconv.Itoa(port),
		WebToken:   "testtoken",
		ServerV4:   "10.0.0.1",
		ServerV6:   "fd00::1",
		MaxClients: 8,
	}
	quitCh := make(chan struct{})
	defer close(quitCh)

	globalConfigPath = "" // 避免误写真实路径
	if err := startWebUI(cfg, "tap0", NewIPAM(nil, nil, 8), NewVSwitch(nil), nil, quitCh); err != nil {
		t.Fatalf("startWebUI: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // 等待 listener 就绪

	base := "http://127.0.0.1:" + strconv.Itoa(port)

	// 无 token → 401。
	resp, err := http.Get(base + "/api/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("no token should 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 带 token → 200 且含 mode。
	req, _ := http.NewRequest("GET", base+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer testtoken")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET status with token: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"mode":"server"`) {
		t.Fatalf("status with token failed: %d %s", resp.StatusCode, body)
	}

	// 首页 HTML：含标题、外部 CSS/JS 与 chart.umd.js 引用。
	resp, err = http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	html, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(html), "KCPVPN") {
		t.Fatalf("index html missing, status=%d", resp.StatusCode)
	}
	for _, asset := range []string{"style.css", "app.js", "chart.umd.js"} {
		if !strings.Contains(string(html), asset) {
			t.Fatalf("index html missing %s reference", asset)
		}
	}

	// 静态资源（CSS/JS/图表库）应实际可访问且非空。
	for _, asset := range []string{"style.css", "app.js", "chart.umd.js"} {
		resp, err = http.Get(base + "/" + asset)
		if err != nil {
			t.Fatalf("GET /%s: %v", asset, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || len(b) == 0 {
			t.Fatalf("GET /%s failed: status=%d len=%d", asset, resp.StatusCode, len(b))
		}
	}

	// /api/chart：等待 sampler 至少采样一次，返回 labels+series。
	time.Sleep(1100 * time.Millisecond)
	req, _ = http.NewRequest("GET", base+"/api/chart", nil)
	req.Header.Set("Authorization", "Bearer testtoken")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET chart: %v", err)
	}
	chartBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(chartBody), `"labels"`) ||
		!strings.Contains(string(chartBody), `"rx_bps"`) {
		t.Fatalf("chart failed: %d %s", resp.StatusCode, chartBody)
	}

	// PUT 非法配置（mode 切换）→ 校验失败，不触发重启。
	badReq, _ := http.NewRequest("PUT", base+"/api/config", strings.NewReader(`{"mode":"client","psk":"x"}`))
	badReq.Header.Set("Authorization", "Bearer testtoken")
	resp, err = http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatalf("PUT bad config: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("bad config should return 200 with error field, got %d", resp.StatusCode)
	}
}

func TestVSwitchTrafficCounts(t *testing.T) {
	vs := NewVSwitch(nil)
	macA := "aa:bb:cc:dd:ee:ff"
	aReader, aWriter := net.Pipe()
	defer aReader.Close()
	vs.Register(macA, aWriter)
	defer vs.Unregister(aWriter)

	hwA, _ := net.ParseMAC(macA)
	frame := make([]byte, 60)
	copy(frame[0:6], hwA)  // dst = A
	copy(frame[6:12], hwA) // src = A（已注册）

	// 客户端→TAP（sender != nil）：入站 rx。
	if !vs.Forward(frame, aWriter) {
		t.Fatal("client->tap forward should succeed")
	}
	// TAP→客户端（sender == nil）：出站 tx。
	if !vs.Forward(frame, nil) {
		t.Fatal("tap->client forward should succeed")
	}

	tr := vs.Traffic()
	if tr.RxPkts != 1 || tr.RxBytes != 60 {
		t.Fatalf("rx mismatch: %+v", tr)
	}
	if tr.TxPkts != 1 || tr.TxBytes != 60 {
		t.Fatalf("tx mismatch: %+v", tr)
	}
}

func TestTrafficSamplerServer(t *testing.T) {
	vs := NewVSwitch(nil)
	ipam := NewIPAM(nil, nil, 8)
	s := newTrafficSampler("server", ipam, vs, nil, 3)

	base := time.Now()
	vs.rxBytes.Store(1000)
	vs.txBytes.Store(2000)
	vs.drops.Store(10)
	s.tick(base)

	vs.rxBytes.Store(1500) // +500
	vs.drops.Store(14)     // +4
	s.tick(base.Add(time.Second))

	vs.rxBytes.Store(1600) // +100
	s.tick(base.Add(2 * time.Second))

	ch := s.snapshot()
	if len(ch.Labels) != 3 || len(ch.RxBps) != 3 || len(ch.TxBps) != 3 || len(ch.DropsPerS) != 3 {
		t.Fatalf("series length mismatch: %d/%d/%d/%d", len(ch.Labels), len(ch.RxBps), len(ch.TxBps), len(ch.DropsPerS))
	}
	if ch.RxBps[0] != 0 {
		t.Fatalf("first tick rx should be 0 (no prev), got %v", ch.RxBps[0])
	}
	if ch.RxBps[1] != 500 {
		t.Fatalf("second tick rx should be 500 B/s, got %v", ch.RxBps[1])
	}
	if ch.DropsPerS[1] != 4 {
		t.Fatalf("second tick drops should be 4/s, got %v", ch.DropsPerS[1])
	}
}

func TestTrafficSamplerClient(t *testing.T) {
	st := &clientState{}
	s := newTrafficSampler("client", nil, nil, st, 2)
	base := time.Now()

	st.rxBytes.Store(500)
	s.tick(base)
	st.rxBytes.Store(700) // +200
	st.setUp(true, nil, "")
	s.tick(base.Add(time.Second))

	ch := s.snapshot()
	if len(ch.RxBps) != 2 || ch.RxBps[1] != 200 {
		t.Fatalf("client rx series wrong: %v", ch.RxBps)
	}
	if ch.Clients[1] != 1 {
		t.Fatalf("client up should sample 1, got %v", ch.Clients[1])
	}
}

func TestHandleChart(t *testing.T) {
	vs := NewVSwitch(nil)
	ipam := NewIPAM(nil, nil, 8)
	s := newTrafficSampler("server", ipam, vs, nil, 5)
	s.tick(time.Now())

	ui := &webUI{cfg: &Config{}, mode: "server", sampler: s}
	rr := httptest.NewRecorder()
	ui.handleChart(rr, httptest.NewRequest("GET", "/api/chart", nil))
	if rr.Code != 200 {
		t.Fatalf("chart should 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"labels"`) || !strings.Contains(body, `"rx_bps"`) || !strings.Contains(body, `"drops_per_s"`) {
		t.Fatalf("chart body missing series keys: %s", body)
	}
}
