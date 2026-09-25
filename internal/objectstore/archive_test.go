package objectstore

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type staticSecrets struct{}

func (staticSecrets) Resolve(context.Context, string, string) (string, error) { return "secret", nil }

type archiveEnv struct {
	db  *store.DB
	tel *telemetry.DB
	srv *objectstoretest.Server
	arc *Archiver
	now time.Time
}

func newArchiveEnv(t *testing.T) *archiveEnv {
	t.Helper()
	t.Setenv(netguard.AllowPrivateEnv, "true")
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tel, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("telemetry.Open: %v", err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	srv := objectstoretest.New("logs")
	t.Cleanup(srv.Close)

	if err := db.SaveBackupTarget(ctx, store.BackupTarget{ID: "bkt_1", Name: "t", Provider: "custom", Endpoint: srv.URL, Region: "auto", Bucket: "logs", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveBackupTarget: %v", err)
	}
	if err := db.SaveStorageOptions(ctx, store.StorageOptions{TargetID: "bkt_1", Preset: PresetCustom, PathStyle: true}); err != nil {
		t.Fatalf("SaveStorageOptions: %v", err)
	}

	e := &archiveEnv{db: db, tel: tel, srv: srv, now: time.Date(2026, 9, 24, 12, 30, 0, 0, time.UTC)}
	opts := DefaultOptions()
	opts.Lag = 0
	e.arc = NewArchiver(db, &Resolver{Store: db, Secrets: staticSecrets{}, MaxAttempts: 1}, tel, slog.New(slog.NewTextHandler(io.Discard, nil)), opts)
	e.arc.Now = func() time.Time { return e.now }
	return e
}

func (e *archiveEnv) writeLogs(t *testing.T, resource string, at ...time.Time) {
	t.Helper()
	entries := make([]telemetry.LogEntry, len(at))
	for i, ts := range at {
		entries[i] = telemetry.LogEntry{ResourceID: resource, Stream: "stdout", Timestamp: ts, Message: "line " + ts.Format(time.RFC3339Nano)}
	}
	if err := e.tel.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatalf("WriteLogBatch: %v", err)
	}
}

func (e *archiveEnv) policy(t *testing.T, app string, wmAgo time.Duration) store.LogArchivePolicy {
	t.Helper()
	p := store.LogArchivePolicy{ID: "lap_" + app, AppName: app, TargetID: "bkt_1", Enabled: true, IntervalSeconds: 3600, WatermarkNs: e.now.Add(-wmAgo).UnixNano(), CreatedAt: "2026-09-24T00:00:00Z"}
	if err := e.db.UpsertLogArchivePolicy(context.Background(), p); err != nil {
		t.Fatalf("UpsertLogArchivePolicy: %v", err)
	}
	return p
}

func (e *archiveEnv) readLines(t *testing.T) int {
	t.Helper()
	total := 0
	for _, k := range e.srv.Keys() {
		data, _ := e.srv.Data(k)
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("gunzip %s: %v", k, err)
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			var l archiveLine
			if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
				t.Fatalf("bad ndjson in %s: %v", k, err)
			}
			total++
		}
	}
	return total
}

func TestRunPolicyPerApp(t *testing.T) {
	e := newArchiveEnv(t)
	base := e.now.Add(-90 * time.Minute)
	e.writeLogs(t, "service:web", base, base.Add(10*time.Minute), base.Add(70*time.Minute))
	e.writeLogs(t, "service:api", base.Add(time.Minute))
	p := e.policy(t, "web", 2*time.Hour)

	if err := e.arc.RunPolicy(context.Background(), p); err != nil {
		t.Fatalf("RunPolicy: %v", err)
	}
	keys := e.srv.Keys()
	if len(keys) != 2 {
		t.Fatalf("keys = %v, want 2 hour partitions for web only", keys)
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, "log-archive/service/web/2026/09/24/") || !strings.HasSuffix(k, ".ndjson.gz") {
			t.Fatalf("unexpected key layout %q", k)
		}
	}
	if got := e.readLines(t); got != 3 {
		t.Fatalf("archived lines = %d, want 3", got)
	}

	got, err := e.db.GetLogArchivePolicy(context.Background(), "web")
	if err != nil || got.WatermarkNs != e.now.UnixNano() || got.LastError != "" || got.LastSuccessAt == "" {
		t.Fatalf("policy after run = %+v, err %v", got, err)
	}

	if err := e.arc.RunPolicy(context.Background(), got); err != nil {
		t.Fatalf("second RunPolicy: %v", err)
	}
	if len(e.srv.Keys()) != 2 {
		t.Fatalf("second run added objects: %v", e.srv.Keys())
	}
}

func TestRunPolicyGlobalCoversEveryResource(t *testing.T) {
	e := newArchiveEnv(t)
	at := e.now.Add(-10 * time.Minute)
	e.writeLogs(t, "service:web", at)
	e.writeLogs(t, "database:pg", at)
	p := e.policy(t, "", time.Hour)

	if err := e.arc.RunPolicy(context.Background(), p); err != nil {
		t.Fatalf("RunPolicy: %v", err)
	}
	joined := strings.Join(e.srv.Keys(), " ")
	if !strings.Contains(joined, "log-archive/service/web/") || !strings.Contains(joined, "log-archive/database/pg/") {
		t.Fatalf("keys = %v", e.srv.Keys())
	}
}

func TestRunPolicyRetriesFailedWindowWithoutDuplicates(t *testing.T) {
	e := newArchiveEnv(t)
	e.writeLogs(t, "service:web", e.now.Add(-10*time.Minute), e.now.Add(-5*time.Minute))
	p := e.policy(t, "web", time.Hour)

	e.srv.FailPuts = true
	if err := e.arc.RunPolicy(context.Background(), p); err == nil {
		t.Fatal("expected failure while puts fail")
	}
	failed, _ := e.db.GetLogArchivePolicy(context.Background(), "web")
	if failed.LastError == "" || failed.WatermarkNs >= e.now.UnixNano() {
		t.Fatalf("failed run must record the error and stop short of now: %+v", failed)
	}

	e.srv.FailPuts = false
	e.now = e.now.Add(5 * time.Minute)
	if err := e.arc.RunPolicy(context.Background(), failed); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := len(e.srv.Keys()); got != 1 {
		t.Fatalf("keys = %v, want 1 (retry overwrites)", e.srv.Keys())
	}
	if got := e.readLines(t); got != 2 {
		t.Fatalf("lines = %d, want 2", got)
	}
	ok, _ := e.db.GetLogArchivePolicy(context.Background(), "web")
	if ok.LastError != "" {
		t.Fatalf("error not cleared: %q", ok.LastError)
	}
}

func TestRunPolicySplitsOnLineCapWithoutLosingLines(t *testing.T) {
	e := newArchiveEnv(t)
	e.arc.Opts.MaxLinesPerObject = 2
	start := time.Date(2026, 9, 24, 12, 0, 1, 0, time.UTC)
	e.writeLogs(t, "service:web", start, start, start, start.Add(time.Second), start.Add(2*time.Second), start.Add(3*time.Second))
	p := e.policy(t, "web", time.Hour)

	if err := e.arc.RunPolicy(context.Background(), p); err != nil {
		t.Fatalf("RunPolicy: %v", err)
	}
	if got := e.readLines(t); got != 6 {
		t.Fatalf("lines = %d, want 6 across %v", got, e.srv.Keys())
	}
	if len(e.srv.Keys()) < 3 {
		t.Fatalf("expected the cap to force several objects, got %v", e.srv.Keys())
	}
}

func TestStartDump(t *testing.T) {
	e := newArchiveEnv(t)
	e.writeLogs(t, "service:web", e.now.Add(-3*time.Hour), e.now.Add(-2*time.Hour))

	tests := []struct {
		name    string
		req     DumpRequest
		wantErr bool
	}{
		{name: "reversed range", req: DumpRequest{TargetID: "bkt_1", AppName: "web", From: e.now, To: e.now.Add(-time.Hour)}, wantErr: true},
		{name: "range too long", req: DumpRequest{TargetID: "bkt_1", AppName: "web", From: e.now.Add(-90 * 24 * time.Hour), To: e.now}, wantErr: true},
		{name: "unknown target", req: DumpRequest{TargetID: "nope", AppName: "web", From: e.now.Add(-time.Hour), To: e.now}, wantErr: true},
		{name: "ok", req: DumpRequest{TargetID: "bkt_1", AppName: "web", From: e.now.Add(-4 * time.Hour), To: e.now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := e.arc.StartDump(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			e.arc.Wait()
			runs, _ := e.db.ListLogArchiveRuns(context.Background(), "web", false, 10)
			if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != "succeeded" || runs[0].Lines != 2 || runs[0].Kind != "manual" {
				t.Fatalf("runs = %+v", runs)
			}
		})
	}
}

func TestPruneRespectsRetentionAndOwnPolicies(t *testing.T) {
	e := newArchiveEnv(t)
	old := e.now.Add(-10 * 24 * time.Hour)
	e.srv.Put("log-archive/service/web/2026/09/14/00/1.ndjson.gz", []byte("x"), old)
	e.srv.Put("log-archive/service/web/2026/09/24/00/2.ndjson.gz", []byte("x"), e.now)
	e.srv.Put("log-archive/service/api/2026/09/14/00/3.ndjson.gz", []byte("x"), old)
	e.policy(t, "api", time.Hour)
	global := e.policy(t, "", time.Hour)
	global.RetentionDays = 7

	cl, _, err := e.arc.Clients.Client(context.Background(), "bkt_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.arc.prune(context.Background(), cl, global); err != nil {
		t.Fatalf("prune: %v", err)
	}
	keys := strings.Join(e.srv.Keys(), " ")
	if strings.Contains(keys, "/1.ndjson.gz") || !strings.Contains(keys, "/2.ndjson.gz") || !strings.Contains(keys, "/3.ndjson.gz") {
		t.Fatalf("keys after prune = %s", keys)
	}
}

func TestObjectKeyAndWindows(t *testing.T) {
	ns := time.Date(2026, 9, 24, 5, 7, 0, 0, time.UTC).UnixNano()
	if got, want := ObjectKey("p", "service:my app", ns), "p/service/my_app/2026/09/24/05/"; !strings.HasPrefix(got, want) {
		t.Fatalf("key %q lacks prefix %q", got, want)
	}
	wins := hourWindows(ns, time.Date(2026, 9, 24, 7, 30, 0, 0, time.UTC).UnixNano(), 0)
	if len(wins) != 3 {
		t.Fatalf("windows = %v, want 3", wins)
	}
	if got := hourWindows(ns, time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC).UnixNano(), 2); len(got) != 2 {
		t.Fatalf("cap not applied: %v", got)
	}
	if !IsArchiveKey("p", "p/a/b") || IsArchiveKey("p", "q/a") || IsArchiveKey("p", "p/../x") {
		t.Fatal("IsArchiveKey mismatch")
	}
}
