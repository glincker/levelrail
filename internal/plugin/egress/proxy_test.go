package egress

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const testSecret = "s3cret-value"

func testCreds() CredentialStore {
	return NewHashedCredentials(map[string]SecretHash{"mailer": HashSecret(testSecret), "other": HashSecret("other-secret")})
}

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	ips, ok := f[host]
	if !ok {
		return nil, fmt.Errorf("no such host")
	}
	var out []netip.Addr
	for _, s := range ips {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

type denials struct {
	mu   sync.Mutex
	list []string
}

func (d *denials) add(plugin, host string, port int, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.list = append(d.list, fmt.Sprintf("%s|%s|%d|%s", plugin, host, port, reason))
}

func (d *denials) last() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.list) == 0 {
		return ""
	}
	return d.list[len(d.list)-1]
}

func echoServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer func() { _ = c.Close() }(); _, _ = io.Copy(c, c) }()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func silentServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func newProxy(t *testing.T, known bool, allow StaticAllowlist, res fakeResolver, cfg Config, d *denials) *httptest.Server {
	t.Helper()
	creds := NewHashedCredentials(nil)
	if known {
		creds = testCreds().(*HashedCredentials)
	}
	p := New(Options{Credentials: creds, Allowlist: allow, Resolver: res, OnDeny: d.add, Config: cfg})
	s := httptest.NewServer(p)
	t.Cleanup(s.Close)
	return s
}

// connect opens a CONNECT tunnel and returns the status line, conn and reader.
func connect(t *testing.T, proxyURL, target string) (string, net.Conn, *bufio.Reader) {
	t.Helper()
	return connectAs(t, proxyURL, target, "mailer", testSecret)
}

func connectAs(t *testing.T, proxyURL, target, user, pass string) (string, net.Conn, *bufio.Reader) {
	t.Helper()
	u, _ := url.Parse(proxyURL)
	c, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	auth := ""
	if user != "" {
		auth = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n"
	}
	_, _ = fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", target, target, auth)
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(status), c, br
}

func TestConnectDecisions(t *testing.T) {
	port := echoServer(t)
	host := fmt.Sprintf("api.test:%d", port)
	target := host
	tests := []struct {
		name    string
		known   bool
		allow   StaticAllowlist
		ips     []string
		private bool
		target  string
		status  string
		reason  string
	}{
		{"unknown caller", false, StaticAllowlist{"mailer": {host}}, []string{"127.0.0.1"}, true, target, "407", ReasonUnknownCaller},
		{"host not listed", true, StaticAllowlist{"mailer": {"other.test"}}, []string{"127.0.0.1"}, true, target, "403", ReasonHostNotListed},
		{"other plugin list ignored", true, StaticAllowlist{"someone": {host}}, []string{"127.0.0.1"}, true, target, "403", ReasonHostNotListed},
		{"port not declared", true, StaticAllowlist{"mailer": {"api.test"}}, []string{"127.0.0.1"}, true, target, "403", ReasonHostNotListed},
		{"private not allowed", true, StaticAllowlist{"mailer": {host}}, []string{"127.0.0.1"}, false, target, "403", ReasonBlockedAddr},
		{"metadata blocked", true, StaticAllowlist{"mailer": {host}}, []string{"169.254.169.254"}, false, target, "403", ReasonBlockedAddr},
		{"rfc1918 blocked", true, StaticAllowlist{"mailer": {host}}, []string{"10.0.0.5"}, false, target, "403", ReasonBlockedAddr},
		{"unresolvable", true, StaticAllowlist{"mailer": {host}}, nil, false, target, "502", ReasonResolveFailed},
		{"case insensitive allowed", true, StaticAllowlist{"mailer": {host}}, []string{"127.0.0.1"}, true, strings.ToUpper("api.test") + fmt.Sprintf(":%d", port), "200", ""},
		{"private allowed works", true, StaticAllowlist{"mailer": {host}}, []string{"127.0.0.1"}, true, target, "200", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &denials{}
			res := fakeResolver{}
			if tc.ips != nil {
				res["api.test"] = tc.ips
			}
			s := newProxy(t, tc.known, tc.allow, res, Config{AllowPrivate: tc.private}, d)
			status, _, _ := connect(t, s.URL, tc.target)
			if !strings.Contains(status, " "+tc.status+" ") {
				t.Fatalf("status = %q, want %s", status, tc.status)
			}
			got := d.last()
			if tc.reason == "" && got != "" {
				t.Fatalf("unexpected denial %q", got)
			}
			if tc.reason != "" && !strings.HasSuffix(got, "|"+tc.reason) {
				t.Fatalf("denial = %q, want reason %s", got, tc.reason)
			}
		})
	}
}

func TestConnectDefaultPort443(t *testing.T) {
	a := StaticAllowlist{"mailer": {"api.test"}}
	if !a.Allowed("mailer", "API.test", 443) || a.Allowed("mailer", "api.test", 80) {
		t.Fatal("default port must be 443 only")
	}
	if !(StaticAllowlist{"mailer": {"api.test:8443"}}).Allowed("mailer", "api.test", 8443) {
		t.Fatal("declared port must be allowed")
	}
}

func TestConnectTunnelRoundTrip(t *testing.T) {
	port := echoServer(t)
	host := fmt.Sprintf("api.test:%d", port)
	s := newProxy(t, true, StaticAllowlist{"mailer": {host}}, fakeResolver{"api.test": {"127.0.0.1"}}, Config{AllowPrivate: true}, &denials{})
	status, c, br := connect(t, s.URL, host)
	if !strings.Contains(status, "200") {
		t.Fatalf("status = %q", status)
	}
	_, _ = br.ReadString('\n') // blank line ending the response head
	_, _ = c.Write([]byte("ping"))
	buf := make([]byte, 4)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(br, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q err %v", buf, err)
	}
}

func TestPlainHTTP(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) }))
	defer up.Close()
	port := up.Listener.Addr().(*net.TCPAddr).Port
	host := fmt.Sprintf("api.test:%d", port)
	for _, private := range []bool{true, false} {
		d := &denials{}
		s := newProxy(t, true, StaticAllowlist{"mailer": {host}}, fakeResolver{"api.test": {"127.0.0.1"}}, Config{AllowPrivate: private}, d)
		pu, _ := url.Parse(ProxyURL("mailer", testSecret, strings.TrimPrefix(s.URL, "http://")))
		cl := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
		resp, err := cl.Get("http://" + host + "/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if private && (resp.StatusCode != 200 || string(body) != "hello") {
			t.Fatalf("private allowed: %d %q", resp.StatusCode, body)
		}
		if !private && (resp.StatusCode != 403 || !strings.HasSuffix(d.last(), ReasonBlockedAddr)) {
			t.Fatalf("private denied: %d %q", resp.StatusCode, d.last())
		}
	}
}

func TestTunnelCap(t *testing.T) {
	port := echoServer(t)
	host := fmt.Sprintf("api.test:%d", port)
	d := &denials{}
	s := newProxy(t, true, StaticAllowlist{"mailer": {host}}, fakeResolver{"api.test": {"127.0.0.1"}}, Config{AllowPrivate: true, MaxTunnels: 1}, d)
	st, first, _ := connect(t, s.URL, host)
	if !strings.Contains(st, "200") {
		t.Fatalf("first = %q", st)
	}
	st, _, _ = connect(t, s.URL, host)
	if !strings.Contains(st, "429") || !strings.HasSuffix(d.last(), ReasonTunnelCap) {
		t.Fatalf("second = %q denial %q", st, d.last())
	}
	_ = first.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, c, _ := connect(t, s.URL, host)
		_ = c.Close()
		if strings.Contains(st, "200") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("slot was not released after the tunnel closed")
}

func TestIdleTimeoutClosesTunnel(t *testing.T) {
	port := silentServer(t)
	host := fmt.Sprintf("api.test:%d", port)
	s := newProxy(t, true, StaticAllowlist{"mailer": {host}}, fakeResolver{"api.test": {"127.0.0.1"}}, Config{AllowPrivate: true, IdleTimeout: 100 * time.Millisecond}, &denials{})
	_, c, br := connect(t, s.URL, host)
	_, _ = br.ReadString('\n')
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	if _, err := br.ReadByte(); err != io.EOF {
		t.Fatalf("want EOF after idle timeout, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("tunnel outlived the idle timeout")
	}
}

func TestBytesPerMinuteCap(t *testing.T) {
	port := echoServer(t)
	host := fmt.Sprintf("api.test:%d", port)
	d := &denials{}
	s := newProxy(t, true, StaticAllowlist{"mailer": {host}}, fakeResolver{"api.test": {"127.0.0.1"}}, Config{AllowPrivate: true, BytesPerMinute: 8}, d)
	_, c, br := connect(t, s.URL, host)
	_, _ = br.ReadString('\n')
	_, _ = c.Write([]byte("0123456789abcdef"))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadAll(br); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasSuffix(d.last(), ReasonRateLimit) {
		t.Fatalf("denial = %q", d.last())
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{
		EnvConnectTimeout: "3s", EnvIdleTimeout: "bogus", EnvMaxTunnels: "4",
		EnvBytesPerMinute: "1000", EnvAllowPrivate: "true",
	}
	c := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if c.ConnectTimeout != 3*time.Second || c.IdleTimeout != 0 || c.MaxTunnels != 4 || c.BytesPerMinute != 1000 || !c.AllowPrivate {
		t.Fatalf("config = %+v", c)
	}
	if d := c.withDefaults(); d.IdleTimeout != defaultIdle {
		t.Fatalf("default idle = %v", d.IdleTimeout)
	}
}
