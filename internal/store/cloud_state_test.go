package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCloudStateLockExclusiveUnderConcurrency(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, locked := 0, 0
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.AcquireCloudStateLock(ctx, "acct", "net", fmt.Sprintf("runner-%d", i), time.Minute, now)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrCloudStateLocked):
				locked++
			default:
				t.Errorf("acquire: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins != 1 || locked != 15 {
		t.Fatalf("wins = %d, locked = %d; want exactly one holder", wins, locked)
	}
}

func TestCloudStateLockLifecycleAndFencing(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()

	a, err := db.AcquireCloudStateLock(ctx, "acct", "net", "runner-a", time.Minute, now)
	if err != nil || a.Fence != 1 {
		t.Fatalf("acquire = %+v, %v", a, err)
	}
	if _, err := db.StealCloudStateLock(ctx, "acct", "net", "runner-b", time.Minute, now.Add(30*time.Second)); !errors.Is(err, ErrCloudStateLockLive) {
		t.Fatalf("steal of a live lock err = %v", err)
	}
	if a, err = db.RenewCloudStateLock(ctx, a, time.Minute, now.Add(30*time.Second)); err != nil {
		t.Fatalf("renew: %v", err)
	}
	v, err := db.PutCloudState(ctx, a, 0, []byte("sealed-1"), now.Add(40*time.Second))
	if err != nil || v != 1 {
		t.Fatalf("first put = %d, %v", v, err)
	}
	if _, err := db.PutCloudState(ctx, a, 0, []byte("sealed-x"), now.Add(41*time.Second)); !errors.Is(err, ErrCloudStateConflict) {
		t.Fatalf("stale version err = %v", err)
	}

	later := now.Add(5 * time.Minute)
	if _, err := db.RenewCloudStateLock(ctx, a, time.Minute, later); !errors.Is(err, ErrCloudStateLockLost) {
		t.Fatalf("renew after expiry err = %v", err)
	}
	b, err := db.StealCloudStateLock(ctx, "acct", "net", "runner-b", time.Minute, later)
	if err != nil || b.Fence != a.Fence+1 {
		t.Fatalf("steal = %+v, %v", b, err)
	}
	if n, _ := db.CountCloudStateLockSteals(ctx, "acct", "net"); n != 1 {
		t.Fatalf("steal log rows = %d", n)
	}
	// The old holder wakes up with its old fence and must not write.
	stale := a
	stale.ExpiresAt = later.Add(time.Hour)
	if _, err := db.PutCloudState(ctx, stale, 1, []byte("sealed-stale"), later); !errors.Is(err, ErrCloudStateLockLost) {
		t.Fatalf("stale writer err = %v", err)
	}
	if err := db.ReleaseCloudStateLock(ctx, a); !errors.Is(err, ErrCloudStateLockLost) {
		t.Fatalf("stale release err = %v", err)
	}
	if v, err = db.PutCloudState(ctx, b, 1, []byte("sealed-2"), later); err != nil || v != 2 {
		t.Fatalf("new holder put = %d, %v", v, err)
	}
	if err := db.ReleaseCloudStateLock(ctx, b); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := db.PutCloudState(ctx, b, 2, []byte("sealed-3"), later); !errors.Is(err, ErrCloudStateLockLost) {
		t.Fatalf("put after release err = %v", err)
	}
	c, err := db.AcquireCloudStateLock(ctx, "acct", "net", "runner-c", time.Minute, later)
	if err != nil || c.Fence != b.Fence+1 {
		t.Fatalf("fence must keep growing after release: %+v, %v", c, err)
	}
	got, err := db.GetCloudState(ctx, "acct", "net")
	if err != nil || got.Version != 2 || got.Fence != b.Fence || string(got.Sealed) != "sealed-2" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := db.GetCloudState(ctx, "acct", "other"); !errors.Is(err, ErrCloudStateNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestCloudStateQuarantineInsertListResolve(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := db.InsertCloudStateQuarantine(ctx, CloudStateQuarantine{Account: "acct", Stack: "net"}); !errors.Is(err, ErrCloudStateBadInput) {
		t.Fatalf("empty insert: %v", err)
	}
	q := CloudStateQuarantine{Account: "acct", Stack: "net", Sealed: []byte("ct"), Fence: 3, Reason: "fence_moved", CreatedAt: now}
	id, err := db.InsertCloudStateQuarantine(ctx, q)
	if err != nil || id == 0 {
		t.Fatalf("insert: %d %v", id, err)
	}
	if other, _ := db.ListCloudStateQuarantine(ctx, "acct", "other"); len(other) != 0 {
		t.Fatalf("other stack sees %d rows", len(other))
	}
	rows, err := db.ListCloudStateQuarantine(ctx, "acct", "net")
	if err != nil || len(rows) != 1 || rows[0].ID != id || rows[0].Fence != 3 || string(rows[0].Sealed) != "ct" {
		t.Fatalf("list = %+v, %v", rows, err)
	}
	if err := db.ResolveCloudStateQuarantine(ctx, "acct", "other", id, now); !errors.Is(err, ErrCloudStateQuarantineNotFound) {
		t.Fatalf("cross stack resolve: %v", err)
	}
	if err := db.ResolveCloudStateQuarantine(ctx, "acct", "net", id, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ResolveCloudStateQuarantine(ctx, "acct", "net", id, now); !errors.Is(err, ErrCloudStateQuarantineNotFound) {
		t.Fatalf("double resolve: %v", err)
	}
	if rows, _ := db.ListCloudStateQuarantine(ctx, "acct", "net"); len(rows) != 0 {
		t.Fatalf("resolved row still listed: %+v", rows)
	}
}
