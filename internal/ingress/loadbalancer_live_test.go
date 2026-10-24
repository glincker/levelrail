package ingress

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// lbReplica is a counting httptest backend whose health path and app
// path can be flipped at runtime. Probe paths never count as traffic.
type lbReplica struct {
	*httptest.Server
	name      string
	hits      atomic.Int64
	appStatus atomic.Int32 // status for app requests
	hcStatus  atomic.Int32 // status for /healthz
	block     chan struct{}
}

var parkedReplicas sync.Map

func newLBReplica(t *testing.T, name string) *lbReplica {
	t.Helper()
	r := &lbReplica{name: name}
	r.appStatus.Store(http.StatusOK)
	r.hcStatus.Store(http.StatusOK)
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/healthz" {
			w.WriteHeader(int(r.hcStatus.Load()))
			return
		}
		r.hits.Add(1)
		if r.block != nil {
			select {
			case <-r.block:
			case <-req.Context().Done():
			}
		}
		w.WriteHeader(int(r.appStatus.Load()))
		_, _ = fmt.Fprint(w, name)
	}))
	// Caddy keeps upstream health state per dial address for the whole process, so
	// a closed port reused by a later test would inherit "unhealthy". Park the
	// listener instead of closing it; it dies with the test binary.
	parkedReplicas.Store(r.Server, struct{}{})
	return r
}

// startLiveLB applies a one-route LB config on a live Caddy and returns
// its listen address. Readiness is a TCP dial so no request is counted.
func startLiveLB(t *testing.T, lb *LBRoute, reps ...*lbReplica) string {
	t.Helper()
	return startLiveLBHardened(t, lb, nil, reps...)
}

// startLiveLBHardened is startLiveLB with optional edge hardening such as trusted proxies.
func startLiveLBHardened(t *testing.T, lb *LBRoute, h *Hardening, reps ...*lbReplica) string {
	t.Helper()
	skipUnderLoad(t)
	lb.Upstreams = nil
	for _, r := range reps {
		lb.Upstreams = append(lb.Upstreams, r.Listener.Addr().String())
	}
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "lb-live", ListenAddr: addr, StorageDir: t.TempDir(), Hardening: h,
		Routes: []ProxyRoute{{Hosts: []string{"127.0.0.1"}, LB: lb}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["lb-live"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(ctx) })
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close()
			return addr
		}
	}
	t.Fatalf("listener %s never came up", addr)
	return ""
}

// lbGet returns status and body, using a short client timeout.
func lbGet(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := c.Get(url) //nolint:noctx // test helper
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

var lbClient = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}

// waitFor polls cond until it holds or the window ends.
func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out after %s waiting for %s", within, what)
}

func TestDriver_LoadBalancer_WeightedSplit(t *testing.T) {
	a, b := newLBReplica(t, "a"), newLBReplica(t, "b")
	addr := startLiveLB(t, &LBRoute{Policy: LBPolicyWeighted, Weights: []int{3, 1}}, a, b)
	const n = 80
	for i := 0; i < n; i++ {
		lbGet(t, lbClient, "http://"+addr+"/")
	}
	// Expected 60/20; tolerance of 5 absorbs rotation phase and any dropped slot.
	if got := a.hits.Load(); got < 55 || got > 65 {
		t.Fatalf("weights [3,1] over %d requests: a=%d b=%d, want a in [55,65]", n, got, b.hits.Load())
	}
	if a.hits.Load()+b.hits.Load() != n {
		t.Fatalf("lost requests: a=%d b=%d", a.hits.Load(), b.hits.Load())
	}
}

func TestDriver_LoadBalancer_CookieSticky(t *testing.T) {
	reps := []*lbReplica{newLBReplica(t, "a"), newLBReplica(t, "b")}
	addr := startLiveLB(t, &LBRoute{Policy: LBPolicyCookie, CookieName: "lbsid"}, reps...)

	visitor := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Timeout: 5 * time.Second, Jar: jar}
	}
	first := map[string]bool{}
	for v := 0; v < 4; v++ {
		c := visitor()
		_, pinned := lbGet(t, c, "http://"+addr+"/")
		first[pinned] = true
		for i := 0; i < 8; i++ {
			if _, body := lbGet(t, c, "http://"+addr+"/"); body != pinned {
				t.Fatalf("visitor %d pinned to %q then reached %q on request %d", v, pinned, body, i)
			}
		}
	}
	// Cookieless visitors fall back to round robin, so four land on both replicas.
	if !first["a"] || !first["b"] {
		t.Fatalf("new visitors landed on %v, want both replicas", first)
	}
}

func TestDriver_LoadBalancer_URIHashStable(t *testing.T) {
	a, b := newLBReplica(t, "a"), newLBReplica(t, "b")
	addr := startLiveLB(t, &LBRoute{Policy: LBPolicyURIHash}, a, b)
	owner := map[string]string{}
	for round := 0; round < 3; round++ {
		for i := 0; i < 16; i++ {
			p := fmt.Sprintf("/item/%d", i)
			_, body := lbGet(t, lbClient, "http://"+addr+p)
			if prev, ok := owner[p]; ok && prev != body {
				t.Fatalf("path %s moved from %s to %s", p, prev, body)
			}
			owner[p] = body
		}
	}
	if a.hits.Load() == 0 || b.hits.Load() == 0 {
		t.Fatalf("16 distinct paths all hashed to one replica: a=%d b=%d", a.hits.Load(), b.hits.Load())
	}
}

func TestDriver_LoadBalancer_ClientIPHashTrustedProxies(t *testing.T) {
	forwarded := func(addr, xff string) string {
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
		req.Header.Set("X-Forwarded-For", xff)
		resp, err := lbClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	trusted := DefaultHardening()
	trusted.TrustedProxies = []string{"127.0.0.1/32"}
	cases := []struct {
		name     string
		hardened *Hardening
		spread   bool
	}{
		{"trusted peer spreads clients", &trusted, true},
		{"untrusted peer ignores header", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := newLBReplica(t, "a"), newLBReplica(t, "b")
			addr := startLiveLBHardened(t, &LBRoute{Policy: LBPolicyClientIPHash}, tc.hardened, a, b)
			owner := map[string]string{}
			seen := map[string]bool{}
			for round := 0; round < 3; round++ {
				for i := 0; i < 24; i++ {
					ip := fmt.Sprintf("198.51.100.%d", i+1)
					body := forwarded(addr, ip)
					if prev, ok := owner[ip]; ok && prev != body {
						t.Fatalf("client %s moved from %s to %s", ip, prev, body)
					}
					owner[ip] = body
					seen[body] = true
				}
			}
			if tc.spread && (!seen["a"] || !seen["b"]) {
				t.Fatalf("clients reached %v, want both replicas", seen)
			}
			if !tc.spread && len(seen) != 1 {
				t.Fatalf("header should be ignored, but clients reached %v", seen)
			}
		})
	}
}

func TestDriver_LoadBalancer_LeastConnAvoidsSlowReplica(t *testing.T) {
	slow, fast := newLBReplica(t, "slow"), newLBReplica(t, "fast")
	slow.block = make(chan struct{})
	addr := startLiveLB(t, &LBRoute{Policy: LBPolicyLeastConn}, slow, fast)

	// A request stuck on slow keeps its in-flight count at 1, so least_conn
	// must send every later request to fast (0 in flight).
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &http.Client{Timeout: 10 * time.Second}
			if resp, err := c.Get("http://" + addr + "/"); err == nil { //nolint:noctx // test
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
		time.Sleep(30 * time.Millisecond)
	}
	if got := slow.hits.Load(); got > 1 {
		t.Errorf("slow replica took %d requests while blocked, want at most 1", got)
	}
	if got := fast.hits.Load(); got < 19 {
		t.Errorf("fast replica took %d of 20, want at least 19", got)
	}
	close(slow.block)
	wg.Wait()
}

func TestDriver_LoadBalancer_PassiveHealthEjectsAndRejoins(t *testing.T) {
	a, b := newLBReplica(t, "a"), newLBReplica(t, "b")
	addr := startLiveLB(t, &LBRoute{
		PassiveHealth: &PassiveHealthChecks{FailDuration: "2s", MaxFails: 1, UnhealthyStatus: []int{500, 502, 503}},
	}, a, b)
	b.appStatus.Store(http.StatusBadGateway)

	// Round robin reaches b within two requests; its 5xx trips the passive
	// counter. Then wait for a clean run of a.
	sawFail := false
	for i := 0; i < 2 && !sawFail; i++ {
		st, _ := lbGet(t, lbClient, "http://"+addr+"/")
		sawFail = st == http.StatusBadGateway
	}
	if !sawFail {
		t.Fatal("never reached the failing replica")
	}
	waitFor(t, 3*time.Second, "b to be skipped", func() bool {
		_, body := lbGet(t, lbClient, "http://"+addr+"/")
		return body == "a"
	})
	for i := 0; i < 10; i++ {
		if st, body := lbGet(t, lbClient, "http://"+addr+"/"); st != http.StatusOK || body != "a" {
			t.Fatalf("request %d after ejection: status=%d body=%q, want 200 from a", i, st, body)
		}
	}

	b.appStatus.Store(http.StatusOK)
	waitFor(t, 8*time.Second, "b to rejoin after fail_duration", func() bool {
		_, body := lbGet(t, lbClient, "http://"+addr+"/")
		return body == "b"
	})
}

func TestDriver_LoadBalancer_ActiveHealthEjectsAndRejoins(t *testing.T) {
	a, b := newLBReplica(t, "a"), newLBReplica(t, "b")
	addr := startLiveLB(t, &LBRoute{
		ActiveHealth: &ActiveHealthChecks{URI: "/healthz", Interval: "200ms", Timeout: "200ms", Passes: 2, Fails: 2},
	}, a, b)
	b.hcStatus.Store(http.StatusServiceUnavailable)

	waitFor(t, 6*time.Second, "b to leave the pool", func() bool {
		for i := 0; i < 6; i++ {
			if _, body := lbGet(t, lbClient, "http://"+addr+"/"); body == "b" {
				return false
			}
		}
		return true
	})

	b.hcStatus.Store(http.StatusOK)
	waitFor(t, 6*time.Second, "b to rejoin", func() bool {
		_, body := lbGet(t, lbClient, "http://"+addr+"/")
		return body == "b"
	})
}

// Current behavior: when every upstream fails its active check Caddy does
// not fail open. With no retries configured the client gets 503 at once.
func TestDriver_LoadBalancer_AllUnhealthyReturns503NoFailOpen(t *testing.T) {
	a, b := newLBReplica(t, "a"), newLBReplica(t, "b")
	addr := startLiveLB(t, &LBRoute{
		ActiveHealth: &ActiveHealthChecks{URI: "/healthz", Interval: "200ms", Timeout: "200ms", Passes: 1, Fails: 1},
	}, a, b)
	a.hcStatus.Store(http.StatusServiceUnavailable)
	b.hcStatus.Store(http.StatusServiceUnavailable)

	waitFor(t, 6*time.Second, "503 once both replicas are marked down", func() bool {
		st, _ := lbGet(t, lbClient, "http://"+addr+"/")
		return st == http.StatusServiceUnavailable
	})
	// Requests before the first probe lands may still reach a replica, so
	// count only what arrives after the 503 was observed.
	before := a.hits.Load() + b.hits.Load()
	for i := 0; i < 5; i++ {
		if st, _ := lbGet(t, lbClient, "http://"+addr+"/"); st != http.StatusServiceUnavailable {
			t.Fatalf("request %d: status %d, want 503", i, st)
		}
	}
	if got := a.hits.Load() + b.hits.Load(); got != before {
		t.Errorf("%d requests reached unhealthy replicas, want fail closed", got-before)
	}
}

func TestDriver_LoadBalancer_RateLimit429(t *testing.T) {
	a := newLBReplica(t, "a")
	// rps 2, burst 5: the 1s burst zone allows 5 requests per second per client.
	addr := startLiveLB(t, &LBRoute{RateLimitRPS: 2, RateLimitBurst: 5}, a)
	ok, limited := 0, 0
	for i := 0; i < 12; i++ {
		switch st, _ := lbGet(t, lbClient, "http://"+addr+"/"); st {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("request %d: unexpected status %d", i, st)
		}
	}
	if limited < 4 || ok > 8 {
		t.Fatalf("12 rapid requests with burst 5: ok=%d limited=%d, want 429s past the burst", ok, limited)
	}
}
