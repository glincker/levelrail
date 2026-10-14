package eventhook

import (
	"context"
	"errors"
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

func TestMatches(t *testing.T) {
	tests := []struct {
		name       string
		subscribed []string
		event      string
		want       bool
	}{
		{"empty selects all", nil, "apps.deploy", true},
		{"exact", []string{"apps.deploy"}, "apps.deploy", true},
		{"other type", []string{"apps.create"}, "apps.deploy", false},
		{"family wildcard", []string{"apps.*"}, "apps.deploy", true},
		{"wrong family", []string{"users.*"}, "apps.deploy", false},
		{"star", []string{"*"}, "x.y", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(tt.subscribed, tt.event); got != tt.want {
				t.Fatalf("Matches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecideAndBackoff(t *testing.T) {
	cfg := Config{MaxAttempts: 3, BackoffBase: time.Second, BackoffMax: 3 * time.Second}
	tests := []struct {
		name     string
		attempts int
		status   int
		err      error
		state    string
		wait     time.Duration
	}{
		{"2xx delivers", 1, 204, nil, store.EventDeliveryDelivered, 0},
		{"5xx retries", 1, 503, nil, store.EventDeliveryPending, time.Second},
		{"second retry doubles", 2, 500, nil, store.EventDeliveryPending, 2 * time.Second},
		{"429 retries", 1, 429, nil, store.EventDeliveryPending, time.Second},
		{"transport error retries", 1, 0, errors.New("dial"), store.EventDeliveryPending, time.Second},
		{"400 is permanent", 1, 400, nil, store.EventDeliveryDead, 0},
		{"budget exhausted", 3, 500, nil, store.EventDeliveryDead, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(cfg, tt.attempts, tt.status, tt.err)
			if got.State != tt.state || got.NextAfter != tt.wait {
				t.Fatalf("Decide = %+v, want state %s wait %s", got, tt.state, tt.wait)
			}
		})
	}
	if got := Backoff(cfg, 10); got != 3*time.Second {
		t.Fatalf("backoff not capped: %s", got)
	}
}

func TestTypeForRequest(t *testing.T) {
	tests := []struct{ method, path, action, want string }{
		{"POST", "/api/v1/apps", "", "apps.create"},
		{"DELETE", "/api/v1/apps/web", "", "apps.delete"},
		{"PATCH", "/api/v1/apps/web", "", "apps.update"},
		{"POST", "/api/v1/apps/web/deploy", "", "apps.deploy"},
		{"GET", "/api/v1/apps", "", ""},
		{"POST", "/api/v1/apps", "custom.thing", "custom.thing"},
	}
	for _, tt := range tests {
		if got := TypeForRequest(tt.method, tt.path, tt.action); got != tt.want {
			t.Errorf("TypeForRequest(%s %s) = %q, want %q", tt.method, tt.path, got, tt.want)
		}
	}
}

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
	v, ok := s.m[svc+"/"+key]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}

func (s *memSecrets) DeleteAll(_ context.Context, svc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, svc+"/"+signingKey)
	return nil
}

const signingKey = secretEnvKey

// failOnceStore fails the first delivery result write, simulating a crash
// after the receiver already accepted the event.
type failOnceStore struct {
	*store.DB
	failed bool
}

func (f *failOnceStore) UpdateEventDelivery(ctx context.Context, d store.EventWebhookDelivery) error {
	if !f.failed && d.State == store.EventDeliveryDelivered {
		f.failed = true
		return errors.New("disk full")
	}
	return f.DB.UpdateEventDelivery(ctx, d)
}

func newTestService(t *testing.T, wrap func(*store.DB) Store) (*Service, *store.DB) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var st Store = db
	if wrap != nil {
		st = wrap(db)
	}
	cfg := DefaultConfig()
	cfg.BackoffBase = time.Millisecond
	svc := NewService(Deps{Store: st, Secrets: &memSecrets{m: map[string]string{}}, Config: cfg, Brand: "acme"})
	return svc, db
}

func TestDeliverSignsAndHalfSucceededIsResentWithSameEventID(t *testing.T) {
	t.Setenv(dataexport.AllowPrivateEnv, "true")
	var mu sync.Mutex
	var ids []string
	var secret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		ids = append(ids, r.Header.Get("X-Acme-Event-Id"))
		s := secret
		mu.Unlock()
		if err := dataexport.VerifySignature(s, r.Header.Get("X-Acme-Signature"), body, time.Now(), time.Minute); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, db := newTestService(t, func(db *store.DB) Store { return &failOnceStore{DB: db} })
	ctx := context.Background()
	hook, sec, err := svc.Create(ctx, Input{Name: "siem", URL: srv.URL, Enabled: true, Events: []string{"apps.*"}})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	secret = sec
	mu.Unlock()

	svc.fanOut(ctx, Event{Type: "users.create"})
	if ds, _ := db.ListEventDeliveries(ctx, hook.ID, 10); len(ds) != 0 {
		t.Fatalf("unsubscribed event queued %d deliveries", len(ds))
	}
	svc.fanOut(ctx, Event{Type: "apps.deploy", Data: map[string]any{"app": "web"}})
	svc.deliverDue(ctx)
	svc.deliverDue(ctx)

	ds, _ := db.ListEventDeliveries(ctx, hook.ID, 10)
	if len(ds) != 1 || ds[0].State != store.EventDeliveryDelivered || ds[0].Attempts != 1 {
		t.Fatalf("deliveries = %+v", ds)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ids) != 2 || ids[0] != ids[1] || ids[0] == "" {
		t.Fatalf("expected one event id sent twice, got %v", ids)
	}
}

func TestDeadLetterAndReplay(t *testing.T) {
	t.Setenv(dataexport.AllowPrivateEnv, "true")
	healthy := false
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		ok := healthy
		mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, db := newTestService(t, nil)
	svc.d.Config.MaxAttempts = 2
	ctx := context.Background()
	hook, _, err := svc.Create(ctx, Input{Name: "ops", URL: srv.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	svc.fanOut(ctx, Event{Type: "apps.create"})
	for i := 0; i < 3; i++ {
		time.Sleep(5 * time.Millisecond)
		svc.deliverDue(ctx)
	}
	ds, _ := db.ListEventDeliveries(ctx, hook.ID, 10)
	if len(ds) != 1 || ds[0].State != store.EventDeliveryDead || ds[0].ResponseStatus != 502 {
		t.Fatalf("expected dead 502, got %+v", ds)
	}

	mu.Lock()
	healthy = true
	mu.Unlock()
	if _, err := svc.Replay(ctx, ds[0].ID); err != nil {
		t.Fatal(err)
	}
	svc.deliverDue(ctx)
	got, _ := db.GetEventDelivery(ctx, ds[0].ID)
	if got.State != store.EventDeliveryDelivered {
		t.Fatalf("replayed delivery state = %s", got.State)
	}
}

func TestEmitNeverBlocks(t *testing.T) {
	svc, _ := newTestService(t, nil)
	for i := 0; i < svc.d.Config.QueueSize+10; i++ {
		svc.Emit(Event{Type: "apps.create"})
	}
	if svc.Dropped() != 10 {
		t.Fatalf("dropped = %d, want 10", svc.Dropped())
	}
}
