package functions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeQueueStore struct {
	status string
	next   time.Time
	code   int
}

func (f *fakeQueueStore) ClaimFunctionInvocation(context.Context, time.Time) (store.FunctionInvocation, bool, error) {
	return store.FunctionInvocation{}, false, nil
}

func (f *fakeQueueStore) SettleFunctionInvocation(_ context.Context, _, status string, next time.Time, code int, _ string, _ time.Time) error {
	f.status, f.next, f.code = status, next, code
	return nil
}

func (f *fakeQueueStore) RequeueRunningFunctionInvocations(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeQueueStore) PruneFunctionInvocations(context.Context, time.Time) (int64, error) {
	return 0, nil
}

type fakeApps struct{ missing bool }

func (a fakeApps) GetDesiredService(_ context.Context, name string) (*store.DesiredService, error) {
	if a.missing {
		return nil, errors.New("nope")
	}
	return &store.DesiredService{Name: name}, nil
}

type fixedTarget struct{ url string }

func (t fixedTarget) Prepare(context.Context, *store.DesiredService) (string, error) {
	return t.url, nil
}

func testConfig() TriggerConfig {
	return TriggerConfig{BackoffBase: time.Second, BackoffMax: 8 * time.Second, AttemptTimeout: time.Second, PollInterval: time.Second, Workers: 1}
}

func TestDeliverOutcomes(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		code     int
		attempts int
		missing  bool
		want     string
		delay    time.Duration
	}{
		{"success", 204, 1, false, store.FunctionInvocationDone, 0},
		{"server error retries with backoff", 503, 2, false, store.FunctionInvocationQueued, 2 * time.Second},
		{"attempts exhausted goes dead", 503, 3, false, store.FunctionInvocationDead, 0},
		{"client error goes dead at once", 400, 1, false, store.FunctionInvocationDead, 0},
		{"missing function goes dead", 0, 1, true, store.FunctionInvocationDead, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.code) }))
			defer srv.Close()
			st := &fakeQueueStore{}
			q := &Queue{Store: st, Apps: fakeApps{missing: tt.missing}, Target: fixedTarget{srv.URL}, Config: testConfig(),
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }}
			q.deliver(context.Background(), store.FunctionInvocation{ID: "i", Function: "f", Method: "POST", Path: "/", Attempts: tt.attempts, MaxAttempts: 3})
			if st.status != tt.want {
				t.Fatalf("status = %q, want %q", st.status, tt.want)
			}
			if tt.want == store.FunctionInvocationQueued && st.next.Sub(now) != tt.delay {
				t.Fatalf("next attempt in %v, want %v", st.next.Sub(now), tt.delay)
			}
		})
	}
}

func TestVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"a":1}`)
	sig := Sign("s3cret", now.Unix(), body)
	window := time.Minute
	if _, err := Verify("s3cret", "1800000000", sig, body, window, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if _, err := Verify("s3cret", "1800000000", sig, []byte("tampered"), window, now); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("tampered body err = %v", err)
	}
	if _, err := Verify("other", "1800000000", sig, body, window, now); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("wrong secret err = %v", err)
	}
	if _, err := Verify("s3cret", "1800000000", sig, body, window, now.Add(2*time.Minute)); !errors.Is(err, ErrSignatureStale) {
		t.Fatalf("stale err = %v", err)
	}
	if _, err := Verify("s3cret", "", "", body, window, now); !errors.Is(err, ErrSignatureMissing) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestBackoffCapped(t *testing.T) {
	c := testConfig()
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 8 * time.Second, 9: 8 * time.Second} {
		if got := c.Backoff(attempt); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}
