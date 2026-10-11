package dataexport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type sink struct {
	mu     sync.Mutex
	bodies []string
	heads  []http.Header
	fail   bool
}

func (s *sink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	s.bodies = append(s.bodies, string(b))
	s.heads = append(s.heads, r.Header.Clone())
	w.WriteHeader(http.StatusNoContent)
}

func (s *sink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, b := range s.bodies {
		for _, l := range strings.Split(strings.TrimSpace(b), "\n") {
			if l != "" {
				out = append(out, l)
			}
		}
	}
	return out
}

func (s *sink) setFail(v bool) {
	s.mu.Lock()
	s.fail = v
	s.mu.Unlock()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func newTestService(t *testing.T, db *store.DB, b *telemetry.LogBroadcaster) *Service {
	t.Helper()
	return newTestServiceWith(t, db, b, &memSecrets{})
}

func newTestServiceWith(t *testing.T, db *store.DB, b *telemetry.LogBroadcaster, sec Secrets) *Service {
	t.Helper()
	t.Setenv(AllowPrivateEnv, "true")
	return NewService(Deps{Store: db, Secrets: sec, Logs: b, Config: fastConfig(), Brand: "plat"})
}

func saveAudit(t *testing.T, db *store.DB, id string, at time.Time, path string) {
	t.Helper()
	if err := db.SaveAuditEntry(context.Background(), store.AuditEntry{
		ID: id, ActorType: "token", ActorID: "t", ActorName: "ci", Ability: "write", Method: "POST",
		Path: path, StatusCode: 200, CreatedAt: store.FormatAuditTime(at), ClientKind: "api",
	}); err != nil {
		t.Fatalf("SaveAuditEntry: %v", err)
	}
}

func TestCreateValidationAndWriteOnlySecrets(t *testing.T) {
	db := openDB(t)
	svc := newTestService(t, db, nil)
	ctx := context.Background()

	bad := []Input{
		{Name: "", Kind: store.ExportKindLogsHTTP, Endpoint: "http://x"},
		{Name: "a", Kind: "nope", Endpoint: "http://x"},
		{Name: "a", Kind: store.ExportKindLogsLoki, Endpoint: "ftp://x"},
		{Name: "a", Kind: store.ExportKindLogsLoki, Endpoint: "http://x", MinLevel: "loud"},
		{Name: "a", Kind: store.ExportKindMetricsOTLP, Endpoint: "http://x", Apps: []string{"web"}},
		{Name: "a", Kind: store.ExportKindAuditS3},
		{Name: "a", Kind: store.ExportKindLogsHTTP, Endpoint: "http://x", Headers: map[string]string{"Host": "evil"}},
		{Name: "a", Kind: store.ExportKindLogsHTTP, Endpoint: "http://x", HMACKey: "0123456789abcdef"},
		{Name: "a", Kind: store.ExportKindAuditWebhook, Endpoint: "http://x", HMACKey: "short"},
		{Name: "a", Kind: store.ExportKindLogsLoki, Endpoint: "http://x", Options: store.ExportOptions{Labels: map[string]string{"app": "x"}}},
	}
	for i, in := range bad {
		if _, err := svc.Create(ctx, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: err = %v, want ErrInvalid", i, err)
		}
	}

	d, err := svc.Create(ctx, Input{
		Name: "audit", Kind: store.ExportKindAuditWebhook, Endpoint: "http://127.0.0.1:9", Enabled: true,
		Headers: map[string]string{"X-Token": "super-secret"}, HMACKey: "0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := svc.Get(ctx, d.ID)
	if len(got.Options.HeaderNames) != 1 || got.Options.HeaderNames[0] != "X-Token" || !got.Options.HasHMACKey {
		t.Fatalf("stored options = %+v", got.Options)
	}
	blob, err := svc.readSecrets(ctx, got)
	if err != nil || blob.Headers["X-Token"] != "super-secret" || blob.HMACKey != "0123456789abcdef" {
		t.Fatalf("secrets round trip failed: %+v %v", blob, err)
	}
	if _, err := svc.Create(ctx, Input{Name: "audit", Kind: store.ExportKindLogsHTTP, Endpoint: "http://x"}); !errors.Is(err, store.ErrExportDestinationNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}

	empty := map[string]string{}
	if _, err := svc.Update(ctx, d.ID, Patch{Headers: &empty}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = svc.Get(ctx, d.ID)
	if len(got.Options.HeaderNames) != 0 || !got.Options.HasHMACKey {
		t.Fatalf("headers should be cleared and key kept: %+v", got.Options)
	}
	if err := svc.Delete(ctx, d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, d.ID); !errors.Is(err, store.ErrExportDestinationNotFound) {
		t.Fatalf("after delete err = %v", err)
	}
}

func TestLogDestinationDeliversFilteredLinesAndSurvivesOutage(t *testing.T) {
	db := openDB(t)
	b := telemetry.NewLogBroadcaster()
	svc := newTestService(t, db, b)
	rcv := &sink{}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := svc.Create(ctx, Input{
		Name: "http", Kind: store.ExportKindLogsHTTP, Endpoint: srv.URL, Enabled: true,
		Apps: []string{"web"}, MinLevel: "info",
	}); err != nil {
		t.Fatal(err)
	}
	go svc.Run(ctx)
	waitFor(t, "worker start", func() bool { svc.mu.Lock(); defer svc.mu.Unlock(); return len(svc.workers) == 1 })

	rcv.setFail(true)
	now := time.Now()
	b.Publish(telemetry.LogEntry{ResourceID: "service:other", Stream: "stdout", Timestamp: now, Message: "filtered out"})
	b.Publish(telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Timestamp: now, Message: "during outage"})
	time.Sleep(50 * time.Millisecond)
	rcv.setFail(false)
	b.Publish(telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Timestamp: now, Message: "after recovery"})

	waitFor(t, "recovery delivery", func() bool {
		for _, l := range rcv.lines() {
			if strings.Contains(l, "after recovery") {
				return true
			}
		}
		return false
	})
	for _, l := range rcv.lines() {
		if strings.Contains(l, "filtered out") {
			t.Fatalf("app filter leaked a line: %s", l)
		}
	}
	st, err := svc.Status(ctx, svc.mustID(t, "http"))
	if err != nil || st.SentTotal < 1 {
		t.Fatalf("status = %+v err=%v", st, err)
	}
}

func (s *Service) mustID(t *testing.T, name string) string {
	t.Helper()
	all, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range all {
		if d.Name == name {
			return d.ID
		}
	}
	t.Fatalf("destination %q not found", name)
	return ""
}

func TestAuditCursorIsRestartSafeAndAtLeastOnce(t *testing.T) {
	db := openDB(t)
	rcv := &sink{}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	base := time.Now().UTC().Add(-time.Hour)
	saveAudit(t, db, "a1", base, "/api/v1/apps")
	saveAudit(t, db, "a2", base.Add(time.Second), "/api/v1/apps/web/deploy")

	sec := &memSecrets{}
	run := func() (*Service, context.CancelFunc) {
		svc := newTestServiceWith(t, db, nil, sec)
		ctx, cancel := context.WithCancel(context.Background())
		go svc.Run(ctx)
		return svc, cancel
	}

	svc := newTestServiceWith(t, db, nil, sec)
	d, err := svc.Create(context.Background(), Input{
		Name: "audit", Kind: store.ExportKindAuditWebhook, Endpoint: srv.URL, Enabled: true,
		HMACKey: "0123456789abcdef", Options: store.ExportOptions{Backfill: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s1, stop1 := run()
	t.Cleanup(stop1)
	waitFor(t, "backfill", func() bool { return len(rcv.lines()) == 2 })
	waitFor(t, "cursor persisted", func() bool {
		st, _ := db.GetExportStatus(context.Background(), d.ID)
		return st.CursorID == "a2"
	})
	rcv.mu.Lock()
	sig := rcv.heads[0].Get("X-Plat-Signature")
	body := rcv.bodies[0]
	rcv.mu.Unlock()
	if err := VerifySignature("0123456789abcdef", sig, []byte(body), time.Now(), time.Minute); err != nil {
		t.Fatalf("signature does not verify: %v (header %q)", err, sig)
	}
	stop1()
	s1.stopAll()

	saveAudit(t, db, "a3", time.Now().UTC(), "/api/v1/tokens")
	_, stop2 := run()
	t.Cleanup(stop2)
	waitFor(t, "new row", func() bool { return len(rcv.lines()) == 3 })
	time.Sleep(100 * time.Millisecond)
	if n := len(rcv.lines()); n != 3 {
		t.Fatalf("lines = %d, want exactly 3 (no duplicates after restart): %v", n, rcv.lines())
	}
}

func TestAuditFailureHoldsCursor(t *testing.T) {
	db := openDB(t)
	rcv := &sink{fail: true}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	saveAudit(t, db, "b1", time.Now().UTC().Add(time.Minute), "/x")

	svc := newTestService(t, db, nil)
	d, err := svc.Create(context.Background(), Input{
		Name: "audit", Kind: store.ExportKindAuditWebhook, Endpoint: srv.URL, Enabled: true,
		Options: store.ExportOptions{Backfill: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitFor(t, "failures recorded", func() bool {
		st, _ := svc.Status(ctx, d.ID)
		return st.FailuresTotal >= 2 && st.LastError != ""
	})
	if st, _ := svc.Status(ctx, d.ID); st.CursorID != "" {
		t.Fatalf("cursor advanced despite failures: %+v", st)
	}
	rcv.setFail(false)
	waitFor(t, "delivery after recovery", func() bool { return len(rcv.lines()) >= 1 })
}

func TestStreamAuditRangeAndRedaction(t *testing.T) {
	db := openDB(t)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	saveAudit(t, db, "r1", base, "/a")
	saveAudit(t, db, "r2", base.Add(time.Minute), "/api/v1/hooks/ghp_abcdefghijklmnopqrstuvwxyz0123456789")
	saveAudit(t, db, "r3", base.Add(2*time.Minute), "/c")
	cfg := fastConfig()
	cfg.AuditBatch = 1
	svc := NewService(Deps{Store: db, Config: cfg, Brand: "plat"})

	var ids []string
	var path string
	err := svc.StreamAudit(context.Background(), base.Add(time.Minute), base.Add(2*time.Minute), func(r AuditRecord) error {
		ids = append(ids, r.ID)
		path = r.Path
		return nil
	})
	if err != nil || len(ids) != 1 || ids[0] != "r2" {
		t.Fatalf("ids = %v err = %v, want [r2] (from inclusive, to exclusive)", ids, err)
	}
	if strings.Contains(path, "ghp_") {
		t.Fatalf("path not redacted: %s", path)
	}
}

func TestTestSendsProbeWithoutTouchingCounters(t *testing.T) {
	db := openDB(t)
	rcv := &sink{}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	svc := newTestService(t, db, nil)
	d, err := svc.Create(context.Background(), Input{Name: "loki", Kind: store.ExportKindLogsLoki, Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Test(context.Background(), d.ID); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if len(rcv.lines()) != 1 || !strings.Contains(rcv.lines()[0], "export destination test") {
		t.Fatalf("probe not received: %v", rcv.lines())
	}
	rcv.setFail(true)
	if err := svc.Test(context.Background(), d.ID); err == nil {
		t.Fatal("Test should surface the destination error")
	}
}
