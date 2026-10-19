package safemode

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPolicyBackoffIsBoundedExponential(t *testing.T) {
	p := PolicyFromEnv(func(string) string { return "" })
	want := []time.Duration{0, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for failures, w := range want {
		if got := p.Backoff(failures); got != w {
			t.Fatalf("failures %d: want %s got %s", failures, w, got)
		}
	}
	if got := p.Backoff(50); got != time.Minute {
		t.Fatalf("backoff must cap at %s, got %s", time.Minute, got)
	}
	if p.SafeMode(2) || !p.SafeMode(3) {
		t.Fatal("safe mode must start at the third failed boot")
	}
}

func TestPolicyEnvOverrides(t *testing.T) {
	env := map[string]string{EnvAfterFailures: "5", EnvBackoffBase: "1s", EnvBackoffMax: "3s"}
	p := PolicyFromEnv(func(k string) string { return env[k] })
	if p.Threshold != 5 || p.Backoff(9) != 3*time.Second || p.Backoff(1) != time.Second {
		t.Fatalf("overrides ignored: %+v", p)
	}
}

func TestLedgerCountsUnhealthyBootsOnly(t *testing.T) {
	dir := t.TempDir()
	boot := func() State { return NewLedger(dir).Begin() }
	if st := boot(); st.Failures != 0 {
		t.Fatalf("first ever boot has no failures: %+v", st)
	}
	if st := boot(); st.Failures != 1 {
		t.Fatalf("a boot that never became healthy counts: %+v", st)
	}
	if st := boot(); st.Failures != 2 {
		t.Fatalf("failures accumulate: %+v", st)
	}
	l := NewLedger(dir)
	l.Begin()
	l.MarkHealthy()
	if st := NewLedger(dir).Begin(); st.Failures != 0 {
		t.Fatalf("a healthy boot clears the count: %+v", st)
	}
	clean := t.TempDir()
	l = NewLedger(clean)
	l.Begin()
	l.MarkClean()
	if st := NewLedger(clean).Begin(); st.Failures != 0 {
		t.Fatalf("a deliberate stop is not a crash: %+v", st)
	}
}

func TestLedgerRemembersErrorAndResets(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	l.Begin()
	l.NoteError(errors.New("docker unreachable"))
	st := NewLedger(dir).Begin()
	if st.LastError != "docker unreachable" || st.Failures != 1 {
		t.Fatalf("error not kept: %+v", st)
	}
	l.Reset()
	if st := NewLedger(dir).Begin(); st.Failures != 0 || st.LastError != "" {
		t.Fatalf("reset must clear: %+v", st)
	}
}

func newTestServer(t *testing.T, dir string, restarts *atomic.Int64) *Server {
	t.Helper()
	s, err := New(Config{
		DataDir: dir, Brand: "Test", Version: "v0", Entry: Entry{Reason: ReasonDBCorrupt, Detail: "page 4 is damaged", Failures: 3},
		Backups: cpbackup.NewManager(nil, dir), Ledger: NewLedger(dir), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Restart: func() { restarts.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func do(h http.Handler, method, path, code, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if code != "" {
		req.Header.Set(tokenHeader, code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServerLocksDetailsAndActionsBehindTheCode(t *testing.T) {
	var restarts atomic.Int64
	s := newTestServer(t, t.TempDir(), &restarts)
	h := s.Handler()

	public := do(h, http.MethodGet, "/api/v1/safe-mode", "", "")
	if public.Code != 200 || strings.Contains(public.Body.String(), "page 4") || !strings.Contains(public.Body.String(), ReasonDBCorrupt) {
		t.Fatalf("public status must show the reason but not the detail: %d %s", public.Code, public.Body.String())
	}
	if rec := do(h, http.MethodPost, "/api/v1/safe-mode/recover", "wrong", `{"action":"retry"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong code must be refused, got %d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/api/v1/safe-mode", s.Token(), ""); !strings.Contains(rec.Body.String(), "page 4 is damaged") {
		t.Fatalf("unlocked status must include the detail: %s", rec.Body.String())
	}
	if rec := do(h, http.MethodGet, "/api/v1/apps", "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("other API routes answer 503 in safe mode, got %d", rec.Code)
	}
	page := do(h, http.MethodGet, "/", "", "")
	if page.Code != 200 || !strings.Contains(page.Header().Get("Content-Security-Policy"), "nonce-") || !strings.Contains(page.Body.String(), "Test is in safe mode") {
		t.Fatalf("recovery page wrong: %d %v", page.Code, page.Header())
	}
	if restarts.Load() != 0 {
		t.Fatal("nothing above may trigger a restart")
	}
}

func TestServerLocksOutAfterManyWrongCodes(t *testing.T) {
	var restarts atomic.Int64
	s := newTestServer(t, t.TempDir(), &restarts)
	for i := 0; i < maxBadCodes; i++ {
		s.noteBadCode()
	}
	if rec := do(s.Handler(), http.MethodPost, "/api/v1/safe-mode/recover", s.Token(), `{"action":"retry"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a locked out server must refuse even the right code, got %d", rec.Code)
	}
}

func TestServerRetryResetsLedgerAndRestarts(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 4; i++ {
		NewLedger(dir).Begin()
	}
	var restarts atomic.Int64
	s := newTestServer(t, dir, &restarts)
	rec := do(s.Handler(), http.MethodPost, "/api/v1/safe-mode/recover", s.Token(), `{"action":"retry"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for restarts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if restarts.Load() != 1 {
		t.Fatal("restart was not triggered")
	}
	if st := NewLedger(dir).Begin(); st.Failures != 0 {
		t.Fatalf("recovery must clear the failure count so the next boot is normal: %+v", st)
	}
}

func TestServerRestoreStagesVerifiedSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := cpbackup.NewManager(db, dir).Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	var restarts atomic.Int64
	s := newTestServer(t, dir, &restarts)
	if rec := do(s.Handler(), http.MethodPost, "/api/v1/safe-mode/recover", s.Token(), `{"action":"restore"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("restore failed: %d %s", rec.Code, rec.Body.String())
	}
	pending, err := integrity.ReadPending(dir)
	if err != nil || pending == nil || pending.Name != info.Name {
		t.Fatalf("snapshot not staged: %+v %v", pending, err)
	}
	if rec := do(s.Handler(), http.MethodPost, "/api/v1/safe-mode/recover", s.Token(), `{"action":"restore","name":"../x"}`); rec.Code != http.StatusConflict {
		t.Fatalf("bad snapshot name must be refused, got %d", rec.Code)
	}
}
