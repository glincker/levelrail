package fixit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func housekeepingEngine(t *testing.T, f Fix) *Engine {
	t.Helper()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(&memStore{}, WithClock(func() time.Time { return clock }), WithMaxPerHour(5))
	if err := e.Register(f); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestOrphansFix(t *testing.T) {
	tests := []struct {
		name        string
		failing     bool
		pruneErr    error
		pruneCleans bool
		wantOutcome OutcomeStatus
		wantChanges int
	}{
		{"healthy is skipped", false, nil, true, OutcomeSkipped, 0},
		{"prune fixes it", true, nil, true, OutcomeFixed, 3},
		{"prune succeeds but data remains", true, nil, false, OutcomeStillFailing, 3},
		{"half succeeded: freed some then errored", true, errors.New("volume busy"), false, OutcomeStillFailing, 3},
		{"cannot prune here", true, ErrCannotFixHere, false, OutcomeCannotFixHere, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			failing := tc.failing
			pruned := 0
			e := housekeepingEngine(t, OrphansFix(OrphansDeps{
				Check: func(context.Context) (CheckState, error) {
					return CheckState{Failing: failing, Detail: "3 GiB reclaimable"}, nil
				},
				Reclaimable: func(context.Context) (DiskReclaim, error) {
					return DiskReclaim{ImagesBytes: 5, BuildCacheBytes: 7, OrphanNetworks: []string{"old"}}, nil
				},
				Prune: func(context.Context) (int64, error) {
					pruned++
					if tc.pruneCleans && tc.pruneErr == nil {
						failing = false
					}
					return 12, tc.pruneErr
				},
			}))
			v, err := e.Preview(ctx, "docker.orphans.prune")
			if err != nil || pruned != 0 || len(v.Changes) != tc.wantChanges {
				t.Fatalf("preview must not act: %+v err=%v pruned=%d", v, err, pruned)
			}
			r, err := e.Apply(ctx, "docker.orphans.prune", "root", nil)
			if err != nil || r.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q err=%v, want %q", r.Outcome, err, tc.wantOutcome)
			}
		})
	}
}

func TestLegacyTokensFix(t *testing.T) {
	tests := []struct {
		name        string
		pending     int
		migrateErr  error
		stillLeft   int
		wantOutcome OutcomeStatus
	}{
		{"nothing pending", 0, nil, 0, OutcomeSkipped},
		{"all migrate", 3, nil, 0, OutcomeFixed},
		{"half migrated then failed", 3, errors.New("db locked"), 1, OutcomeStillFailing},
		{"migrate runs but some remain", 3, nil, 2, OutcomeStillFailing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pending := tc.pending
			e := housekeepingEngine(t, LegacyTokensFix(LegacyTokensDeps{
				Pending: func(context.Context) (int, error) { return pending, nil },
				Migrate: func(context.Context) error {
					pending = tc.stillLeft
					return tc.migrateErr
				},
			}))
			r, err := e.Apply(ctx, "tokens.legacy.migrate", "root", nil)
			if err != nil || r.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q err=%v, want %q", r.Outcome, err, tc.wantOutcome)
			}
		})
	}
}

func TestLegacyTokensFixWithoutLibrary(t *testing.T) {
	e := housekeepingEngine(t, LegacyTokensFix(LegacyTokensDeps{
		Pending: func(context.Context) (int, error) { return 2, nil },
	}))
	r, err := e.Apply(context.Background(), "tokens.legacy.migrate", "root", nil)
	if err != nil || r.Outcome != OutcomeCannotFixHere {
		t.Fatalf("got %q err=%v", r.Outcome, err)
	}
}
