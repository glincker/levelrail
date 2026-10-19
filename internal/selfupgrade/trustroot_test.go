package selfupgrade

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func waitIdle(t *testing.T, tr *TrustRoot) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		tr.mu.Lock()
		busy := tr.inflight
		tr.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("refresh did not finish")
}

func TestTrustRootStates(t *testing.T) {
	fail := errors.New("offline")
	tests := []struct {
		name  string
		err   error
		cache bool
		want  string
	}{
		{"loads", nil, false, TrustRootReady},
		{"loads with cache", nil, true, TrustRootReady},
		{"unreachable with cache", fail, true, TrustRootCached},
		{"unreachable without cache", fail, false, TrustRootUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "tuf")
			if tt.cache {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "root.json"), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			tr := newTrustRoot(dir, func(string) error { return tt.err }, nil)
			first := tr.State()
			if want := map[bool]string{true: TrustRootCached, false: TrustRootChecking}[tt.cache]; first != want {
				t.Fatalf("first state = %s, want %s", first, want)
			}
			waitIdle(t, tr)
			if got := tr.State(); got != tt.want {
				t.Fatalf("state = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTrustRootWarmDedupesAndCaches(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	tr := newTrustRoot(t.TempDir(), func(string) error {
		calls.Add(1)
		<-release
		return nil
	}, nil)
	for i := 0; i < 5; i++ {
		tr.Warm()
	}
	close(release)
	waitIdle(t, tr)
	tr.Warm()
	waitIdle(t, tr)
	if n := calls.Load(); n != 1 {
		t.Fatalf("loader ran %d times, want 1 (in-flight and fresh results are reused)", n)
	}
}

func TestTrustRootRetriesAfterBackoff(t *testing.T) {
	var calls atomic.Int32
	now := time.Now()
	tr := newTrustRoot(filepath.Join(t.TempDir(), "none"), func(string) error {
		calls.Add(1)
		return errors.New("offline")
	}, nil)
	tr.now = func() time.Time { return now }
	_ = tr.State()
	waitIdle(t, tr)
	_ = tr.State()
	waitIdle(t, tr)
	if calls.Load() != 1 {
		t.Fatalf("retried inside the backoff: %d calls", calls.Load())
	}
	now = now.Add(trustRootRetry + time.Second)
	_ = tr.State()
	waitIdle(t, tr)
	if calls.Load() != 2 {
		t.Fatalf("no retry after the backoff: %d calls", calls.Load())
	}
}

func TestTrustRootTimeoutDoesNotBlock(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	tr := newTrustRoot(filepath.Join(t.TempDir(), "none"), func(string) error { <-block; return nil }, nil)
	tr.timeout = 20 * time.Millisecond
	start := time.Now()
	_ = tr.State()
	if time.Since(start) > 15*time.Millisecond {
		t.Fatal("State blocked on the loader")
	}
	waitIdle(t, tr)
	if got := tr.State(); got != TrustRootUnavailable {
		t.Fatalf("state after timeout = %s", got)
	}
}
