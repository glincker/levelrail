package ddns

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/heartbeat"
	"time"
)

func fixedFetch(answers map[string]string) FetchFunc {
	return func(_ context.Context, u string) (string, error) {
		a, ok := answers[u]
		if !ok {
			return "", errors.New("down")
		}
		return a, nil
	}
}

func TestDetectAgreement(t *testing.T) {
	d := &Detector{URLs: []string{"a", "b", "c"}, Quorum: 2}
	d.Fetch = fixedFetch(map[string]string{"a": "8.8.8.8", "b": "8.8.8.8", "c": "1.1.1.1"})
	if ip, err := d.Detect(context.Background()); err != nil || ip != "8.8.8.8" {
		t.Fatalf("got %q %v", ip, err)
	}
	d.Fetch = fixedFetch(map[string]string{"a": "8.8.8.8", "b": "1.1.1.1", "c": "9.9.9.9"})
	if _, err := d.Detect(context.Background()); !errors.Is(err, ErrNoQuorum) {
		t.Fatalf("want no quorum, got %v", err)
	}
	d.Fetch = fixedFetch(map[string]string{"a": "10.0.0.5", "b": "10.0.0.5"})
	if _, err := d.Detect(context.Background()); !errors.Is(err, ErrNonPublic) {
		t.Fatalf("want non public, got %v", err)
	}
	d.Fetch = fixedFetch(map[string]string{"a": "100.64.1.1", "b": "100.64.1.1"})
	if _, err := d.Detect(context.Background()); !errors.Is(err, ErrNonPublic) {
		t.Fatalf("cgnat must be refused, got %v", err)
	}
}

func testCfg() Config {
	return Config{Interval: time.Minute, Quorum: 1, Timeout: 2 * time.Second, MinUpdateGap: time.Minute,
		Reassert: time.Hour, FailAlertAfter: 2, AllowPrivate: true, AllowInsecure: true}
}

func TestURLSubstitutionAndDuckPreset(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()
	p, err := NewHTTPProvider(srv.URL+"/u?host=home&k=s3cretvalue&ip={ip}", testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Update(context.Background(), "8.8.4.4"); err != nil || !strings.Contains(got, "ip=8.8.4.4") {
		t.Fatalf("query %q err %v", got, err)
	}
	if _, err := NewHTTPProvider(srv.URL+"/u", testCfg(), nil); err == nil {
		t.Fatal("missing placeholder must fail")
	}
	duck, err := NewDuckDNS("a,b", "tok", testCfg())
	if err != nil || !strings.Contains(duck.template, "domains=a%2Cb&token=tok&ip={ip}") {
		t.Fatalf("preset %v", err)
	}
	if duck.accept("KO") || !duck.accept("OK\n") {
		t.Fatal("duckdns body check wrong")
	}
}

func TestTokenNeverInErrorsOrLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }))
	url := srv.URL + "/u?token=SUPERSECRETTOKEN&ip={ip}"
	p, err := NewHTTPProvider(url, testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	err1 := p.Update(context.Background(), "8.8.8.8")
	srv.Close()
	err2 := p.Update(context.Background(), "8.8.8.8")
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	u := New(testCfg(), &Detector{URLs: []string{"a"}, Quorum: 1, Fetch: fixedFetch(map[string]string{"a": "8.8.8.8"})}, p, log, Hooks{})
	u.Tick(context.Background())
	for _, s := range []string{err1.Error(), err2.Error(), buf.String()} {
		if strings.Contains(s, "SUPERSECRETTOKEN") {
			t.Fatalf("token leaked: %s", s)
		}
	}
}

func TestPrivateDestinationsRefused(t *testing.T) {
	cfg := testCfg()
	cfg.AllowPrivate = false
	if _, err := NewHTTPProvider("http://127.0.0.1/u?ip={ip}", cfg, nil); err == nil {
		t.Fatal("loopback literal must be refused")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	p := &HTTPProvider{template: srv.URL + "/?ip={ip}", client: heartbeat.NewClient(cfg.AllowPrivate, cfg.Timeout)}
	if err := p.Update(context.Background(), "8.8.8.8"); err == nil {
		t.Fatal("guarded client must refuse loopback dial")
	}
}

type failing struct{ n int }

func (f *failing) Update(context.Context, string) error { f.n++; return errors.New("boom") }

type panicking struct{}

func (panicking) Update(context.Context, string) error { panic("x") }

func TestFailureAlertsAndNeverPanics(t *testing.T) {
	alerts := 0
	det := &Detector{URLs: []string{"a"}, Quorum: 1, Fetch: fixedFetch(map[string]string{"a": "8.8.8.8"})}
	u := New(testCfg(), det, &failing{}, nil, Hooks{OnAlert: func(string) { alerts++ }})
	for range 4 {
		u.Tick(context.Background())
	}
	if alerts != 1 || u.Status().Failures != 4 {
		t.Fatalf("alerts %d failures %d", alerts, u.Status().Failures)
	}
	New(testCfg(), det, panicking{}, nil, Hooks{}).Tick(context.Background())
}

func TestMinGapAndChange(t *testing.T) {
	now := time.Unix(1000, 0)
	ip := "8.8.8.8"
	det := &Detector{URLs: []string{"a"}, Quorum: 1, Fetch: func(context.Context, string) (string, error) { return ip, nil }}
	f := &okCounter{}
	u := New(testCfg(), det, f, nil, Hooks{})
	u.now = func() time.Time { return now }
	u.Tick(context.Background())
	u.Tick(context.Background())
	ip = "8.8.4.4"
	now = now.Add(10 * time.Second)
	u.Tick(context.Background())
	if f.n != 1 {
		t.Fatalf("gap must defer, writes %d", f.n)
	}
	now = now.Add(2 * time.Minute)
	u.Tick(context.Background())
	if f.n != 2 {
		t.Fatalf("writes %d", f.n)
	}
}

type okCounter struct{ n int }

func (o *okCounter) Update(context.Context, string) error { o.n++; return nil }

func TestEnabledOnlyByEnv(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if Enabled(env(nil)) || !Enabled(env(map[string]string{EnvEnabled: "on"})) {
		t.Fatal("enable flag")
	}
	if Enabled(env(map[string]string{EnvEnabled: "on", envPublicIPDetect: "off"})) {
		t.Fatal("detect off must disable")
	}
}
