package selfheartbeat

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
)

type fakeSettings struct{ url, interval string }

func (f *fakeSettings) String(_ context.Context, key string) string {
	if key == runtimesettings.KeyAlertingSelfHeartbeatURL {
		return f.url
	}
	return ""
}

func (f *fakeSettings) Int(_ context.Context, _ string) int {
	n, _ := strconv.Atoi(f.interval)
	return n
}

func newTestPinger(set Settings, buf *bytes.Buffer) *Pinger {
	p := New(Config{Timeout: time.Second, LogEvery: time.Hour}, set, slog.New(slog.NewTextHandler(buf, nil)))
	p.client = http.DefaultClient
	p.validate = func(context.Context, string) error { return nil }
	t := time.Unix(0, 0)
	p.now = func() time.Time { return t }
	return p
}

func TestOffWhenURLEmpty(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	p := newTestPinger(&fakeSettings{interval: "10"}, &bytes.Buffer{})
	p.Tick(context.Background())
	if hits.Load() != 0 {
		t.Fatal("pinged with empty url")
	}
}

func TestPingFiresAtLiveInterval(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		hits.Add(1)
	}))
	defer srv.Close()
	set := &fakeSettings{url: srv.URL, interval: "7"}
	p := newTestPinger(set, &bytes.Buffer{})
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "true")
	if got := p.Interval(context.Background()); got != time.Minute {
		t.Fatalf("out of range interval should fall back, got %s", got)
	}
	set.interval = "30"
	if got := p.Interval(context.Background()); got != 30*time.Second {
		t.Fatalf("interval not live: %s", got)
	}
	p.Tick(context.Background())
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestFailureLogIsRateLimitedAndHidesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "true")
	var buf bytes.Buffer
	p := newTestPinger(&fakeSettings{url: srv.URL + "/ping/SECRETTOKEN", interval: "10"}, &buf)
	for i := 0; i < 5; i++ {
		p.Tick(context.Background())
	}
	if n := strings.Count(buf.String(), "ping failed"); n != 1 {
		t.Fatalf("want 1 log line, got %d", n)
	}
	if strings.Contains(buf.String(), "SECRETTOKEN") {
		t.Fatal("token leaked into log")
	}
}

func TestUnreachableDoesNotLeakURL(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "true")
	var buf bytes.Buffer
	p := newTestPinger(&fakeSettings{url: "http://127.0.0.1:1/tok-SECRETTOKEN", interval: "10"}, &buf)
	p.Tick(context.Background())
	if !strings.Contains(buf.String(), "ping failed") || strings.Contains(buf.String(), "SECRETTOKEN") {
		t.Fatalf("log: %s", buf.String())
	}
}

func TestPollArmsWithinIntervalChange(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	defer srv.Close()
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "true")
	set := &fakeSettings{interval: "86400"}
	p := newTestPinger(set, &bytes.Buffer{})
	now := time.Unix(1000000, 0)
	p.now = func() time.Time { return now }
	ctx := context.Background()
	p.Poll(ctx)
	set.url = srv.URL
	p.Poll(ctx)
	if hits.Load() != 1 {
		t.Fatalf("saving a url should arm at once, hits %d", hits.Load())
	}
	now = now.Add(15 * time.Second)
	p.Poll(ctx)
	if hits.Load() != 1 {
		t.Fatal("pinged before the long interval")
	}
	set.interval = "10"
	p.Poll(ctx)
	if hits.Load() != 2 {
		t.Fatalf("lowered interval should ping, hits %d", hits.Load())
	}
}

func TestRedirectToHTTPRefused(t *testing.T) {
	p := New(Config{}, &fakeSettings{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	req := httptest.NewRequest(http.MethodGet, "http://monitor.example/x", nil)
	if err := p.client.CheckRedirect(req, nil); err == nil {
		t.Fatal("http redirect must be refused")
	}
	req = httptest.NewRequest(http.MethodGet, "https://monitor.example/x", nil)
	if err := p.client.CheckRedirect(req, nil); err != nil {
		t.Fatalf("https redirect refused: %v", err)
	}
}
