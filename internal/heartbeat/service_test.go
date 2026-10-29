package heartbeat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type memState struct {
	mu sync.Mutex
	st store.HeartbeatState
}

func (m *memState) GetHeartbeatState(context.Context) (store.HeartbeatState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st, nil
}

func (m *memState) SaveHeartbeatState(_ context.Context, s store.HeartbeatState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.st = s
	return nil
}

type memSecrets map[string]string

func (m memSecrets) SetValue(_ context.Context, svc, k, v string) error { m[svc+"/"+k] = v; return nil }
func (m memSecrets) Resolve(_ context.Context, svc, k string) (string, error) {
	return m[svc+"/"+k], nil
}
func (m memSecrets) Exists(_ context.Context, svc, k string) (bool, error) {
	_, ok := m[svc+"/"+k]
	return ok, nil
}
func (m memSecrets) DeleteValue(_ context.Context, svc, k string) error {
	delete(m, svc+"/"+k)
	return nil
}

func testConfig(url string) Config {
	return Config{URL: url, Interval: time.Minute, Timeout: 2 * time.Second, Method: http.MethodPost, AllowPrivate: true}
}

func TestTickPingsWithStatusOnlyBody(t *testing.T) {
	var hits atomic.Int32
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b := make([]byte, 512)
		n, _ := r.Body.Read(b)
		body = string(b[:n])
	}))
	defer srv.Close()
	st := &memState{st: store.HeartbeatState{BodyEnabled: true}}
	svc := New(Options{Config: testConfig(srv.URL + "/secretkey"), State: st, Version: "v1"})
	svc.Tick(context.Background())
	if hits.Load() != 1 || !strings.Contains(body, `"status":"ok"`) || strings.Contains(body, "127.0.0.1") {
		t.Fatalf("hits=%d body=%q", hits.Load(), body)
	}
	if st.st.LastResult != ResultOK {
		t.Fatalf("result = %q", st.st.LastResult)
	}
}

func TestUnhealthyStaysSilentWithoutFailURL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	st := &memState{}
	svc := New(Options{Config: testConfig(srv.URL), State: st, Health: func(context.Context) (bool, string) { return false, "store" }})
	svc.Tick(context.Background())
	if hits.Load() != 0 || st.st.LastResult != ResultUnhealthy || st.st.LastError != "store" {
		t.Fatalf("hits=%d state=%+v", hits.Load(), st.st)
	}
}

func TestUnhealthyHitsFailURL(t *testing.T) {
	var path atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { path.Store(r.URL.Path) }))
	defer srv.Close()
	cfg := testConfig(srv.URL + "/ok")
	cfg.FailURL = srv.URL + "/fail"
	svc := New(Options{Config: cfg, State: &memState{}, Health: func(context.Context) (bool, string) { return false, "agent" }})
	svc.Tick(context.Background())
	if path.Load() != "/fail" {
		t.Fatalf("path = %v", path.Load())
	}
}

func TestRedirectToOtherHostRefusedAndErrorHidesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:1/x", http.StatusFound)
	}))
	defer srv.Close()
	st := &memState{}
	svc := New(Options{Config: testConfig(srv.URL + "/topsecretkey"), State: st})
	svc.Tick(context.Background())
	if st.st.LastResult != ResultSendFailed || strings.Contains(st.st.LastError, "topsecretkey") || !strings.Contains(st.st.LastError, "another host") {
		t.Fatalf("state = %+v", st.st)
	}
}

func TestPrivateDestinationBlockedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cfg := testConfig(strings.Replace(srv.URL, "127.0.0.1", "localhost", 1))
	cfg.AllowPrivate = false
	st := &memState{}
	New(Options{Config: cfg, State: st}).Tick(context.Background())
	if st.st.LastResult != ResultSendFailed {
		t.Fatalf("state = %+v", st.st)
	}
	if _, err := ValidateURL("http://10.0.0.5/x", false); err == nil {
		t.Fatal("private literal accepted")
	}
}

func TestApplyStoresURLAndStatusShowsHostOnly(t *testing.T) {
	sec := memSecrets{}
	svc := New(Options{Config: testConfig(""), Secrets: sec, State: &memState{st: store.HeartbeatState{BodyEnabled: true}}})
	u := "https://hc-ping.com/abc-secret"
	if err := svc.Apply(context.Background(), Update{URL: &u}); err != nil {
		t.Fatal(err)
	}
	s, err := svc.Status(context.Background())
	if err != nil || !s.Configured || s.Host != "hc-ping.com" || s.Source != SourceDashboard {
		t.Fatalf("status = %+v err=%v", s, err)
	}
	bad := "ftp://x"
	if err := svc.Apply(context.Background(), Update{URL: &bad}); err == nil {
		t.Fatal("ftp accepted")
	}
	empty := ""
	if err := svc.Apply(context.Background(), Update{URL: &empty}); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Status(context.Background()); s.Configured {
		t.Fatal("still configured after clear")
	}
}

func TestApplyRefusedWhenEnvManaged(t *testing.T) {
	svc := New(Options{Config: testConfig("https://x.example/p"), Secrets: memSecrets{}})
	u := "https://y.example/p"
	if err := svc.Apply(context.Background(), Update{URL: &u}); !errors.Is(err, ErrEnvManaged) {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigFromEnvRejectsBadValues(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	if _, err := ConfigFromEnv(env(map[string]string{EnvURL: "nope"})); err == nil {
		t.Fatal("bad url accepted")
	}
	if _, err := ConfigFromEnv(env(map[string]string{EnvInterval: "1s"})); err == nil {
		t.Fatal("tiny interval accepted")
	}
	c, err := ConfigFromEnv(env(map[string]string{EnvURL: "https://hc-ping.com/k", EnvBody: "off"}))
	if err != nil || !c.BodyOff || c.Interval != time.Minute {
		t.Fatalf("cfg = %+v err=%v", c, err)
	}
}
