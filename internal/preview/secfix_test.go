package preview

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestPoolKey_SeparatesPinnedHosts(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	a := docker.BrowserSpec{Network: publicNetwork, Command: []string{"--host-resolver-rules=MAP a.example 1.1.1.1"}}
	b := docker.BrowserSpec{Network: publicNetwork, Command: []string{"--host-resolver-rules=MAP b.example 2.2.2.2"}}
	_, relA, err := h.m.acquireBrowser(ctx, a, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	relA(true)
	_, relB, err := h.m.acquireBrowser(ctx, b, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	relB(true)
	if h.run.started != 2 {
		t.Errorf("started %d browsers, want 2: a pinned host must not be reused for another app", h.run.started)
	}
	_, relA2, _ := h.m.acquireBrowser(ctx, b, time.Minute)
	relA2(true)
	if h.run.started != 2 {
		t.Errorf("same spec must reuse the warm browser, started %d", h.run.started)
	}
	h.m.pool.closeAll()
}

func TestRequestAllowed(t *testing.T) {
	cases := map[string]bool{
		"https://shop.example.com/a":    true,
		"https://SHOP.example.com:8443": true,
		"https://other.example.com/":    false,
		"http://169.254.169.254/latest": false,
		"http://127.0.0.1:8080/":        false,
		"data:image/png;base64,AAAA":    true,
		"file:///etc/passwd":            false,
		"ftp://shop.example.com/":       false,
	}
	for u, want := range cases {
		if got := requestAllowed("shop.example.com", u); got != want {
			t.Errorf("requestAllowed(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestRender_RedirectSkipClearsStatus(t *testing.T) {
	h := shotHarness(t, nil)
	h.m.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	h.res.target = Target{DeploymentID: "dep_1", Image: "img:1", PublicURL: "https://shop.example.com"}
	h.shoot.result = &ShotResult{HTTPStatus: 200, FinalURL: "http://10.0.0.5/admin", PNG: stripedPNG(t)}
	h.m.enqueue(job{app: "web", force: true})
	h.waitIdle(t, "web")
	r, ok := h.record("dep_1")
	if !ok || r.Reason != ReasonRedirect || r.HTTPStatus != 0 {
		t.Errorf("record = %+v ok=%v, want redirect skip with no stored status", r, ok)
	}
	if got := h.shoot.reqs[0].OnlyHost; got != "shop.example.com" {
		t.Errorf("OnlyHost = %q, want the pinned host", got)
	}
}

func TestCDPShooter_OnlyHostFailsOtherRequests(t *testing.T) {
	f := &fakeChrome{status: 200, png: stripedPNG(t), paused: []string{"https://shop.example.com/", "http://169.254.169.254/x"}}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	_, err := CDPShooter{}.Shoot(context.Background(), strings.TrimPrefix(srv.URL, "http://"), ShotRequest{
		URL: "https://shop.example.com/", OnlyHost: "shop.example.com", Width: 800, Height: 600,
		Settle: 3 * time.Second, Deadline: time.Now().Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("Shoot: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(f.fetchCalls, ",")
	if joined != "Fetch.continueRequest:p0,Fetch.failRequest:p1" {
		t.Errorf("fetch calls = %s", joined)
	}
}

func TestCardStates_ResolveErrorBacksOff(t *testing.T) {
	h := shotHarness(t, func(c *Config) { c.MinInterval = time.Hour })
	h.res.err = errors.New("resolve down")
	h.states(t, "web")
	h.waitIdle(t, "web")
	calls := h.res.calls
	h.states(t, "web")
	h.waitIdle(t, "web")
	if h.res.calls != calls {
		t.Errorf("resolve retried inside the backoff: %d -> %d calls", calls, h.res.calls)
	}
}
