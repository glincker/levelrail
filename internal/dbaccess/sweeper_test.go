package dbaccess

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeSweepStore struct {
	mu    sync.Mutex
	rows  map[string]*fakeRow
	order []string
}

type fakeRow struct {
	cred      TempCredential
	state     string
	claimedAt time.Time
}

func newFakeSweepStore(creds ...TempCredential) *fakeSweepStore {
	s := &fakeSweepStore{rows: map[string]*fakeRow{}}
	for _, c := range creds {
		s.rows[c.ID] = &fakeRow{cred: c, state: "active"}
		s.order = append(s.order, c.ID)
	}
	return s
}

func (s *fakeSweepStore) DueTempCredentials(_ context.Context, now time.Time) ([]TempCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []TempCredential
	for _, id := range s.order {
		r := s.rows[id]
		if r.state != "revoked" && !r.cred.ExpiresAt.After(now) {
			out = append(out, r.cred)
		}
	}
	return out, nil
}

func (s *fakeSweepStore) ClaimTempCredential(_ context.Context, id string, now time.Time, lease time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[id]
	switch {
	case r.state == "active", r.state == "revoking" && now.Sub(r.claimedAt) >= lease:
		r.state, r.claimedAt = "revoking", now
		return true, nil
	}
	return false, nil
}

func (s *fakeSweepStore) CompleteTempCredential(_ context.Context, id string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[id].state = "revoked"
	return nil
}

func (s *fakeSweepStore) ReleaseTempCredential(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[id].state = "active"
	return nil
}

type fakeRevoker struct {
	mu    sync.Mutex
	calls map[string]int
	fail  map[string]bool
}

func (r *fakeRevoker) RevokeTemp(_ context.Context, db, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls == nil {
		r.calls = map[string]int{}
	}
	r.calls[db+"/"+role]++
	if r.fail[role] {
		return errors.New("database unreachable")
	}
	return nil
}

type countAuditor struct{ n int }

func (a *countAuditor) RecordRevoked(context.Context, TempCredential) { a.n++ }

func newSweeper(st SweepStore, rv Revoker, now time.Time) (*Sweeper, *countAuditor) {
	a := &countAuditor{}
	return &Sweeper{Store: st, Revoker: rv, Auditor: a, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }, Lease: time.Minute}, a
}

func TestSweeperRevokesExpiredExactlyOnce(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	st := newFakeSweepStore(
		TempCredential{ID: "a", Database: "main", Role: "tmp_a", ExpiresAt: now.Add(-time.Minute)},
		TempCredential{ID: "b", Database: "main", Role: "tmp_b", ExpiresAt: now.Add(time.Hour)},
	)
	rv := &fakeRevoker{}
	sw, audit := newSweeper(st, rv, now)

	if n := sw.Sweep(context.Background()); n != 1 {
		t.Fatalf("first pass revoked %d, want 1", n)
	}
	if n := sw.Sweep(context.Background()); n != 0 {
		t.Fatalf("second pass revoked %d, want 0", n)
	}
	if rv.calls["main/tmp_a"] != 1 || rv.calls["main/tmp_b"] != 0 {
		t.Fatalf("calls = %v", rv.calls)
	}
	if audit.n != 1 {
		t.Fatalf("audit entries = %d, want 1", audit.n)
	}
}

func TestSweeperConcurrentPassesRevokeOnce(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	st := newFakeSweepStore(TempCredential{ID: "a", Database: "main", Role: "tmp_a", ExpiresAt: now.Add(-time.Second)})
	rv := &fakeRevoker{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sw, _ := newSweeper(st, rv, now)
			sw.Sweep(context.Background())
		}()
	}
	wg.Wait()
	if rv.calls["main/tmp_a"] != 1 {
		t.Fatalf("revoked %d times, want 1", rv.calls["main/tmp_a"])
	}
}

func TestSweeperRetriesAfterFailureAndAfterRestart(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	st := newFakeSweepStore(TempCredential{ID: "a", Database: "main", Role: "tmp_a", ExpiresAt: now.Add(-time.Second)})
	rv := &fakeRevoker{fail: map[string]bool{"tmp_a": true}}
	sw, _ := newSweeper(st, rv, now)
	if n := sw.Sweep(context.Background()); n != 0 {
		t.Fatalf("failing revoke counted as done: %d", n)
	}
	if st.rows["a"].state != "active" {
		t.Fatalf("failed revoke must release the claim, state = %s", st.rows["a"].state)
	}

	// A crash between claim and completion leaves a revoking row. A fresh
	// process (new Sweeper, same store) must retake it once the lease passes.
	st.rows["a"].state, st.rows["a"].claimedAt = "revoking", now
	rv.fail = nil
	later, _ := newSweeper(st, rv, now.Add(30*time.Second))
	if n := later.Sweep(context.Background()); n != 0 {
		t.Fatalf("claim inside the lease was retaken: %d", n)
	}
	boot, _ := newSweeper(st, rv, now.Add(2*time.Minute))
	if n := boot.Sweep(context.Background()); n != 1 {
		t.Fatalf("boot sweep after lease revoked %d, want 1", n)
	}
	if st.rows["a"].state != "revoked" {
		t.Fatalf("state = %s", st.rows["a"].state)
	}
}
