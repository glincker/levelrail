package auditstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/store"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) SetValue(_ context.Context, svc, key, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[svc+"/"+key] = v
	return nil
}

func (s *memSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[svc+"/"+key], nil
}

func (s *memSecrets) DeleteAll(context.Context, string) error { return nil }

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func addEntries(t *testing.T, db *store.DB, from, to int) {
	t.Helper()
	for i := from; i <= to; i++ {
		e := store.AuditEntry{
			ID: fmt.Sprintf("aud_%03d", i), ActorType: "session", ActorID: "u", ActorName: "n",
			Ability: "write", Method: "POST", Path: "/p", StatusCode: 200,
			CreatedAt: store.FormatAuditTime(time.Now()),
		}
		if err := db.SaveAuditEntry(context.Background(), e); err != nil {
			t.Fatalf("SaveAuditEntry error = %v", err)
		}
	}
}

func TestWebhookSinkDeliversSignedBatchAndAdvancesCursor(t *testing.T) {
	t.Setenv(dataexport.AllowPrivateEnv, "true")
	db := openDB(t)
	secrets := &memSecrets{m: map[string]string{}}

	var mu sync.Mutex
	var bodies [][]byte
	var sigs []string
	fail := true
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		bodies = append(bodies, b)
		sigs = append(sigs, r.Header.Get("X-Test-Signature"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	now := time.Now()
	cfg := DefaultConfig()
	cfg.BackoffBase = time.Second
	svc := NewService(Deps{
		Store: db, Secrets: secrets, Config: cfg, Brand: "test", HTTPClient: srv.Client(),
		Now: func() time.Time { return now },
	})
	ctx := context.Background()

	addEntries(t, db, 1, 2)
	sink, secret, err := svc.Create(ctx, Input{Name: "siem", Kind: store.AuditSinkWebhook, Enabled: true, Config: SinkConfig{URL: srv.URL}, Backfill: true})
	if err != nil || secret == "" {
		t.Fatalf("Create = %v secret %q", err, secret)
	}

	svc.StreamOnce(ctx)
	got, _ := db.GetAuditSink(ctx, sink.ID)
	if got.CursorSeq != 0 || got.ConsecutiveFailures != 1 || got.LastError == "" {
		t.Fatalf("after failure: %+v, want cursor 0 and 1 failure", got)
	}

	svc.StreamOnce(ctx)
	got, _ = db.GetAuditSink(ctx, sink.ID)
	if got.ConsecutiveFailures != 1 {
		t.Fatalf("sink retried inside its backoff window: %+v", got)
	}

	mu.Lock()
	fail = false
	mu.Unlock()
	now = now.Add(time.Minute)
	svc.StreamOnce(ctx)

	got, _ = db.GetAuditSink(ctx, sink.ID)
	if got.CursorSeq != 2 || got.ConsecutiveFailures != 0 || got.LastError != "" {
		t.Fatalf("after success: %+v, want cursor 2 and no failures", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("got %d bodies, want 1", len(bodies))
	}
	if err := dataexport.VerifySignature(secret, sigs[0], bodies[0], now, time.Minute); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
	var payload struct {
		FromSeq int64             `json:"from_seq"`
		ToSeq   int64             `json:"to_seq"`
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(bodies[0], &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.FromSeq != 1 || payload.ToSeq != 2 || len(payload.Entries) != 2 {
		t.Fatalf("payload = %+v, want seq 1-2 with 2 entries", payload)
	}
	hist, _ := db.ListAuditSinkDeliveries(ctx, sink.ID, 10)
	if len(hist) != 2 {
		t.Fatalf("history rows = %d, want a failed and a delivered attempt", len(hist))
	}
}

func TestNewSinkStreamsOnlyNewEntriesByDefault(t *testing.T) {
	t.Setenv(dataexport.AllowPrivateEnv, "true")
	db := openDB(t)
	svc := NewService(Deps{Store: db, Brand: "test"})
	addEntries(t, db, 1, 3)
	sink, _, err := svc.Create(context.Background(), Input{Name: "sl", Kind: store.AuditSinkSyslog, Enabled: true, Config: SinkConfig{Address: "127.0.0.1:6514"}})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if sink.CursorSeq != 3 {
		t.Fatalf("CursorSeq = %d, want 3 (head at creation)", sink.CursorSeq)
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	cfg := Config{BackoffBase: time.Second, BackoffMax: 5 * time.Second}
	for failures, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 5 * time.Second, 9: 5 * time.Second} {
		if got := Backoff(cfg, failures); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", failures, got, want)
		}
	}
}

func TestCreateRejectsInvalidAndNeedsSecrets(t *testing.T) {
	db := openDB(t)
	svc := NewService(Deps{Store: db, Brand: "test"})
	ctx := context.Background()
	if _, _, err := svc.Create(ctx, Input{Name: "x", Kind: "carrier-pigeon"}); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, _, err := svc.Create(ctx, Input{Name: "x", Kind: store.AuditSinkS3, Config: SinkConfig{Bucket: "b"}, AccessKeyID: "a", SecretAccessKey: "s"}); err == nil {
		t.Fatal("s3 sink accepted without a master key")
	}
}
