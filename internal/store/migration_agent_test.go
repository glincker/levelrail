package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func newAgentSession(t *testing.T, db *DB, id string) time.Time {
	t.Helper()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	err := db.SaveMigrationAgentSession(context.Background(), MigrationAgentSession{ID: id, Label: "src", TokenHash: "h-" + id,
		TokenExpiresAt: now.Add(time.Hour), CreatedBy: "tester", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return now
}

func TestMigrationAgentTokenIsSingleUse(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := newAgentSession(t, db, "ms_a")
	if err := db.RedeemMigrationAgentToken(ctx, "ms_a", "fp", "1", now.Add(72*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if err := db.RedeemMigrationAgentToken(ctx, "ms_a", "fp2", "2", now.Add(72*time.Hour), now); !errors.Is(err, ErrMigrationAgentTokenUsed) {
		t.Fatalf("second redeem got %v, want ErrMigrationAgentTokenUsed", err)
	}
	got, err := db.GetMigrationAgentSession(ctx, "ms_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != MigrationAgentConnected || got.CertFingerprint != "fp" || got.TokenUsedAt == nil {
		t.Fatalf("unexpected row after redeem: %+v", got)
	}
}

func TestMigrationAgentRedeemConcurrentHasOneWinner(t *testing.T) {
	db := openTestDB(t)
	now := newAgentSession(t, db, "ms_race")
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.RedeemMigrationAgentToken(context.Background(), "ms_race", "fp", "1", now.Add(time.Hour), now); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d concurrent redeems succeeded, want exactly 1", wins)
	}
}

func TestMigrationAgentRedeemRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, db *DB, now time.Time)
		at     func(now time.Time) time.Time
	}{
		{"expired token", func(*testing.T, *DB, time.Time) {}, func(n time.Time) time.Time { return n.Add(2 * time.Hour) }},
		{"revoked session", func(t *testing.T, db *DB, n time.Time) {
			if err := db.SetMigrationAgentState(context.Background(), "ms_x", MigrationAgentRevoked, n); err != nil {
				t.Fatal(err)
			}
		}, func(n time.Time) time.Time { return n }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			now := newAgentSession(t, db, "ms_x")
			tc.mutate(t, db, now)
			err := db.RedeemMigrationAgentToken(context.Background(), "ms_x", "fp", "1", now.Add(time.Hour), tc.at(now))
			if !errors.Is(err, ErrMigrationAgentTokenUsed) {
				t.Fatalf("got %v, want a refusal", err)
			}
		})
	}
}

func TestMigrationAgentRevokedIsSticky(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := newAgentSession(t, db, "ms_r")
	if err := db.SetMigrationAgentState(ctx, "ms_r", MigrationAgentRevoked, now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMigrationAgentState(ctx, "ms_r", MigrationAgentLost, now); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetMigrationAgentSession(ctx, "ms_r")
	if got.State != MigrationAgentRevoked || got.RevokedAt == nil {
		t.Fatalf("revoked session changed state: %+v", got)
	}
	if err := db.SetMigrationAgentState(ctx, "ms_missing", MigrationAgentLost, now); !errors.Is(err, ErrMigrationAgentNotFound) {
		t.Fatalf("got %v, want not found", err)
	}
}

func TestMigrationAgentTouchInventoryScanAndList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := newAgentSession(t, db, "ms_t")
	if err := db.TouchMigrationAgentSession(ctx, "ms_t", "v1", "linux", "amd64", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveMigrationAgentInventory(ctx, "ms_t", `{"a":1}`, now); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveMigrationAgentScan(ctx, "ms_t", `{"b":2}`, now); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListMigrationAgentSessions(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v %d", err, len(rows))
	}
	r := rows[0]
	if r.AgentVersion != "v1" || r.AgentOS != "linux" || r.InventoryJSON != `{"a":1}` || r.ScanJSON != `{"b":2}` || r.LastSeenAt == nil {
		t.Fatalf("unexpected row: %+v", r)
	}
}
