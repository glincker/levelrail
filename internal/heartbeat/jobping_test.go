package heartbeat

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type jobMemSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *jobMemSecrets) SetValue(_ context.Context, svc, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[svc+"/"+k] = v
	return nil
}
func (s *jobMemSecrets) Resolve(_ context.Context, svc, k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[svc+"/"+k], nil
}
func (s *jobMemSecrets) Exists(_ context.Context, svc, k string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[svc+"/"+k]
	return ok, nil
}
func (s *jobMemSecrets) DeleteValue(_ context.Context, svc, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, svc+"/"+k)
	return nil
}

func newTestPinger(t *testing.T, allowPrivate bool, logs *bytes.Buffer) (*JobPinger, *jobMemSecrets) {
	t.Helper()
	sec := &jobMemSecrets{}
	cfg := JobPingConfig{Timeout: 2 * time.Second, AllowPrivate: allowPrivate, MaxAttempts: 3, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond, Queue: 4}
	p := NewJobPinger(cfg, sec, slog.New(slog.NewTextHandler(logs, nil)))
	p.sleep = func(context.Context, time.Duration) error { return nil }
	return p, sec
}

func TestJobPingSuccessAndFailURL(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
	}))
	defer srv.Close()
	var logs bytes.Buffer
	p, _ := newTestPinger(t, true, &logs)
	ctx := context.Background()
	if err := p.Set(ctx, "t1", srv.URL+"/tok123?x=1", ""); err != nil {
		t.Fatal(err)
	}
	p.send(ctx, jobEvent{"t1", "r1", store.ScheduledTaskStatusSuccess})
	p.send(ctx, jobEvent{"t1", "r2", store.ScheduledTaskStatusTimeout})
	want := []string{"GET /tok123?x=1", "GET /tok123/fail?x=1"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	host, ok := p.Host(ctx, "t1")
	if !ok || host != strings.TrimPrefix(srv.URL, "http://") {
		t.Fatalf("host = %q %v", host, ok)
	}
}

func TestJobPingExplicitFailURLAndSkippedSendNothing(t *testing.T) {
	hits := map[string]int{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
	}))
	defer srv.Close()
	var logs bytes.Buffer
	p, _ := newTestPinger(t, true, &logs)
	ctx := context.Background()
	if err := p.Set(ctx, "t1", srv.URL+"/ok", srv.URL+"/bad"); err != nil {
		t.Fatal(err)
	}
	p.Notify("t1", "r1", store.ScheduledTaskStatusSkippedConcurrency)
	p.Notify("t1", "r2", store.ScheduledTaskStatusReplaced)
	if len(p.queue) != 0 {
		t.Fatalf("skipped and replaced runs must not queue, got %d", len(p.queue))
	}
	p.Notify("t1", "r3", store.ScheduledTaskStatusFailed)
	ev := <-p.queue
	p.send(ctx, ev)
	if hits["/bad"] != 1 || hits["/ok"] != 0 {
		t.Fatalf("hits = %v", hits)
	}
}

func TestJobPingRetriesServerErrorsNotClientErrors(t *testing.T) {
	var calls int
	code := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(code)
	}))
	defer srv.Close()
	var logs bytes.Buffer
	p, _ := newTestPinger(t, true, &logs)
	if err := p.deliver(context.Background(), srv.URL+"/a"); err == nil || calls != 3 {
		t.Fatalf("5xx: err=%v calls=%d", err, calls)
	}
	calls, code = 0, http.StatusNotFound
	if err := p.deliver(context.Background(), srv.URL+"/a"); err == nil || calls != 1 {
		t.Fatalf("4xx: err=%v calls=%d", err, calls)
	}
}

func TestJobPingPrivateRefusedAndSecretNeverLogged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	var logs bytes.Buffer
	p, sec := newTestPinger(t, false, &logs)
	ctx := context.Background()
	if err := p.Set(ctx, "t1", srv.URL+"/supersecrettoken", ""); err == nil {
		t.Fatal("private address must be refused at Set")
	}
	if strings.Contains(err2s(p.Set(ctx, "t1", srv.URL+"/supersecrettoken", "")), "supersecrettoken") {
		t.Fatal("validation error leaked the token")
	}
	// A stored URL that resolves to loopback is still refused at dial time.
	_ = sec.SetValue(ctx, jobSecretsService, jobKey("t2", "url"), srv.URL+"/supersecrettoken")
	p.send(ctx, jobEvent{"t2", "r1", store.ScheduledTaskStatusSuccess})
	if logs.Len() == 0 {
		t.Fatal("expected a warning for the refused ping")
	}
	if strings.Contains(logs.String(), "supersecrettoken") {
		t.Fatalf("log leaked the token: %s", logs.String())
	}
}

func err2s(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
