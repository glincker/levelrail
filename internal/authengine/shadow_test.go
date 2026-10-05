package authengine

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeAuth struct {
	mu      sync.Mutex
	gate    chan struct{}
	results map[string]Decision
}

func (f *fakeAuth) Authenticate(_ context.Context, raw string) (Decision, error) {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.results[raw], nil
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestShadowJudge(t *testing.T) {
	tests := []struct {
		name     string
		legacy   LegacyOutcome
		lib      Decision
		wantKind string
		skipped  bool
	}{
		{"both accept same", LegacyOutcome{Accepted: true, OwnerID: "u1", Abilities: []string{"read", "write"}}, Decision{Accepted: true, OwnerID: "u1", Abilities: []string{"write", "read"}}, "", false},
		{"both reject", LegacyOutcome{}, Decision{}, "", false},
		{"decision differs", LegacyOutcome{Accepted: true, OwnerID: "u1", Abilities: []string{"read"}}, Decision{}, MismatchDecision, false},
		{"owner differs", LegacyOutcome{Accepted: true, OwnerID: "u1", Abilities: []string{"read"}}, Decision{Accepted: true, OwnerID: "u2", Abilities: []string{"read"}}, MismatchOwner, false},
		{"abilities differ", LegacyOutcome{Accepted: true, OwnerID: "u1", Abilities: []string{"read", "write"}}, Decision{Accepted: true, OwnerID: "u1", Abilities: []string{"read"}}, MismatchAbilities, false},
		{"root normalizes", LegacyOutcome{Accepted: true, OwnerID: "u1", Abilities: []string{"root", "read"}}, Decision{Accepted: true, OwnerID: "u1", Abilities: []string{"root"}}, "", false},
		{"system token skipped", LegacyOutcome{Accepted: true, Abilities: []string{"read"}}, Decision{}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewShadow(&fakeAuth{}, nil, ShadowConfig{QueueSize: 1, Workers: 1, MismatchLog: 3}, discard())
			defer s.Close()
			s.judge(tc.legacy, tc.lib)
			snap := s.Snapshot()
			if tc.skipped {
				if snap.Skipped != 1 || snap.Compared != 0 {
					t.Fatalf("skipped = %d compared = %d", snap.Skipped, snap.Compared)
				}
				return
			}
			if snap.Compared != 1 {
				t.Fatalf("compared = %d, want 1", snap.Compared)
			}
			if tc.wantKind == "" {
				if snap.Matched != 1 || snap.Mismatched != 0 {
					t.Fatalf("snapshot = %+v", snap)
				}
				return
			}
			if snap.Mismatched != 1 || len(snap.Mismatches) != 1 || snap.Mismatches[0].Kind != tc.wantKind {
				t.Fatalf("snapshot = %+v, want one %s mismatch", snap, tc.wantKind)
			}
		})
	}
}

func TestShadowObserveNeverBlocksWhenQueueFull(t *testing.T) {
	gate := make(chan struct{})
	fa := &fakeAuth{gate: gate, results: map[string]Decision{}}
	legacy := func(context.Context, string) (LegacyOutcome, error) { return LegacyOutcome{}, nil }
	s := NewShadow(fa, legacy, ShadowConfig{QueueSize: 2, Workers: 1, MismatchLog: 3}, discard())

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			s.Observe("tok")
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Observe blocked on a full queue")
	}
	if got := s.Snapshot().Dropped; got < 90 {
		t.Fatalf("dropped = %d, want at least 90 of 100", got)
	}
	close(gate)
	s.Close()
	if s.Observe("late") {
		t.Fatal("Observe after Close must drop")
	}
}

func TestShadowCountsAndSurfacesMismatches(t *testing.T) {
	fa := &fakeAuth{results: map[string]Decision{"a": {Accepted: true, OwnerID: "u1", Abilities: []string{"read"}}}}
	legacy := func(_ context.Context, raw string) (LegacyOutcome, error) {
		return LegacyOutcome{Accepted: true, TokenID: "tok_" + raw, OwnerID: "u1", Abilities: []string{"read", "write"}}, nil
	}
	s := NewShadow(fa, legacy, ShadowConfig{QueueSize: 8, Workers: 2, MismatchLog: 2}, discard())
	defer s.Close()
	for range 3 {
		s.Observe("a")
	}
	waitFor(t, func() bool { return s.Snapshot().Compared == 3 })
	snap := s.Snapshot()
	if snap.Mismatched != 3 || snap.Matched != 0 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if len(snap.Mismatches) != 2 {
		t.Fatalf("ring kept %d, want 2", len(snap.Mismatches))
	}
	if m := snap.Mismatches[0]; m.TokenID != "tok_a" || m.Kind != MismatchAbilities {
		t.Fatalf("mismatch = %+v", m)
	}
}
