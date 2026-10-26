package store

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestStack(t *testing.T, db *DB, name string) StackRecord {
	t.Helper()
	s, err := db.CreateStack(t.Context(), StackRecord{Name: name, ProjectID: "shop", CreatedBy: "ci"}, time.Now())
	if err != nil {
		t.Fatalf("CreateStack(%q): %v", name, err)
	}
	return s
}

func TestStacks_CRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestStack(t, db, "shop-prod")
	newTestStack(t, db, "alpha")
	if a.DriftPolicy != "warn" {
		t.Fatalf("default drift = %q", a.DriftPolicy)
	}
	if _, err := db.CreateStack(ctx, StackRecord{Name: "shop-prod", ProjectID: "x"}, time.Now()); !errors.Is(err, ErrStackExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	got, err := db.GetStackByName(ctx, "shop-prod")
	if err != nil || got.ID != a.ID || got.CreatedBy != "ci" {
		t.Fatalf("GetStackByName = %+v, %v", got, err)
	}
	if _, err := db.GetStack(ctx, "nope"); !errors.Is(err, ErrStackNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	list, _ := db.ListStacks(ctx)
	if len(list) != 2 || list[0].Name != "alpha" {
		t.Fatalf("list = %+v", list)
	}
	if err := db.UpdateStackSettings(ctx, a.ID, true, "lock", true); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetStack(ctx, a.ID)
	if !got.AllowPrune || got.DriftPolicy != "lock" || !got.RequireApproval {
		t.Fatalf("settings = %+v", got)
	}

	if err := db.UpsertStackResource(ctx, StackLedgerEntry{StackID: a.ID, Kind: "App", Key: "App/shop/web"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteStack(ctx, a.ID, "bogus"); err == nil {
		t.Fatal("bad mode accepted")
	}
	held, err := db.DeleteStack(ctx, a.ID, "forget")
	if err != nil || len(held) != 1 {
		t.Fatalf("DeleteStack = %+v, %v", held, err)
	}
	if _, ok, _ := db.GetStackResourceOwner(ctx, "App", "App/shop/web"); ok {
		t.Fatal("ledger survived stack delete")
	}
	if _, err := db.DeleteStack(ctx, a.ID, ""); !errors.Is(err, ErrStackNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestStackRevisions_NumberingUnderConcurrency(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	s := newTestStack(t, db, "busy")
	other := newTestStack(t, db, "other")
	const n = 20
	var wg sync.WaitGroup
	nums := make(chan int, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: s.ID, Actor: fmt.Sprint(i)}, time.Now())
			if err != nil {
				t.Errorf("append: %v", err)
				return
			}
			nums <- r.Number
		}()
	}
	wg.Wait()
	close(nums)
	seen := map[int]bool{}
	for v := range nums {
		seen[v] = true
	}
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Fatalf("revision %d missing, got %v", i, seen)
		}
	}
	r, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: other.ID}, time.Now())
	if err != nil || r.Number != 1 {
		t.Fatalf("other stack first revision = %d, %v", r.Number, err)
	}
	if _, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: "ghost"}, time.Now()); !errors.Is(err, ErrStackNotFound) {
		t.Fatalf("ghost err = %v", err)
	}
}

func TestStackRevisions_DocsRoundTripAndIdempotency(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	s := newTestStack(t, db, "docs")
	docs := bytes.Repeat([]byte("kind: App\nname: web\n"), 200)
	first, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: s.ID, RenderedDocs: docs, PlanJSON: []byte(`{"a":1}`), IdempotencyKey: "k1", GitSHA: "abc"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var stored int
	if err := db.QueryRowContext(ctx, `SELECT length(rendered_docs_gz) FROM stack_revisions WHERE stack_id = ?`, s.ID).Scan(&stored); err != nil || stored >= len(docs) {
		t.Fatalf("docs not compressed: %d of %d, %v", stored, len(docs), err)
	}
	got, err := db.GetStackRevision(ctx, s.ID, first.Number)
	if err != nil || !bytes.Equal(got.RenderedDocs, docs) || got.GitSHA != "abc" || string(got.PlanJSON) != `{"a":1}` {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	again, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: s.ID, IdempotencyKey: "k1"}, time.Now())
	if err != nil || !again.Replayed || again.Number != first.Number {
		t.Fatalf("replay = %+v, %v", again, err)
	}
	list, _ := db.ListStackRevisions(ctx, s.ID, 0)
	if len(list) != 1 || list[0].RenderedDocs != nil {
		t.Fatalf("list = %+v", list)
	}
	if _, err := db.GetStackRevision(ctx, s.ID, 9); !errors.Is(err, ErrStackRevisionNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestStackRevisions_RefuseSecretLeak(t *testing.T) {
	db := openTestDB(t)
	s := newTestStack(t, db, "leaky")
	secrets := map[string]string{"DB_PASSWORD": "hunter2-value", "EMPTY": ""}
	_, err := db.AppendStackRevision(t.Context(), NewStackRevision{
		StackID: s.ID, RenderedDocs: []byte("env: {X: hunter2-value}"), Secrets: secrets,
	}, time.Now())
	if !errors.Is(err, ErrStackSecretLeak) {
		t.Fatalf("err = %v", err)
	}
	if list, _ := db.ListStackRevisions(t.Context(), s.ID, 0); len(list) != 0 {
		t.Fatal("leaking revision was stored")
	}
	if _, err := db.AppendStackRevision(t.Context(), NewStackRevision{StackID: s.ID, RenderedDocs: []byte("clean"), Secrets: secrets}, time.Now()); err != nil {
		t.Fatalf("clean docs refused: %v", err)
	}
}

func TestStackRevisions_TransitionsAndSupersede(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	s := newTestStack(t, db, "flow")
	now := time.Now()
	mk := func(status string) int {
		r, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: s.ID, Status: status}, now)
		if err != nil {
			t.Fatal(err)
		}
		return r.Number
	}
	status := func(n int) string {
		r, _ := db.GetStackRevision(ctx, s.ID, n)
		return r.Status
	}
	r1 := mk(StackRevApplying)
	if err := db.SetStackRevisionStatus(ctx, s.ID, r1, StackRevSuperseded, nil, now); !errors.Is(err, ErrStackBadTransition) {
		t.Fatalf("applying to superseded err = %v", err)
	}
	if err := db.SetStackRevisionStatus(ctx, s.ID, r1, StackRevApplied, []byte(`{"ok":true}`), now); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetStackRevision(ctx, s.ID, r1)
	if got.FinishedAt == nil || string(got.ResultJSON) != `{"ok":true}` {
		t.Fatalf("terminal fields = %+v", got)
	}
	if err := db.SetStackRevisionStatus(ctx, s.ID, r1, StackRevApplying, nil, now); !errors.Is(err, ErrStackBadTransition) {
		t.Fatalf("backwards err = %v", err)
	}

	r2 := mk(StackRevPendingApproval)
	if err := db.SetStackRevisionStatus(ctx, s.ID, r2, StackRevApplied, nil, now); !errors.Is(err, ErrStackBadTransition) {
		t.Fatalf("pending to applied err = %v", err)
	}
	for _, next := range []string{StackRevApplying, StackRevPartial} {
		if err := db.SetStackRevisionStatus(ctx, s.ID, r2, next, nil, now); err != nil {
			t.Fatal(err)
		}
	}
	if status(r1) != StackRevSuperseded || status(r2) != StackRevPartial {
		t.Fatalf("after r2: r1=%s r2=%s", status(r1), status(r2))
	}
	r3 := mk(StackRevApplying)
	if err := db.SetStackRevisionStatus(ctx, s.ID, r3, StackRevFailed, nil, now); err != nil {
		t.Fatal(err)
	}
	if status(r2) != StackRevPartial {
		t.Fatalf("failed revision superseded r2: %s", status(r2))
	}
	if err := db.SetStackRevisionStatus(ctx, s.ID, 99, StackRevApplied, nil, now); !errors.Is(err, ErrStackRevisionNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if StackRevisionCanTransition(StackRevFailed, StackRevApplied) || !StackRevisionCanTransition(StackRevApplied, StackRevSuperseded) {
		t.Fatal("transition table drifted")
	}
}

func TestStackLedger_ConflictAndRelease(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a, b := newTestStack(t, db, "a"), newTestStack(t, db, "b")
	e := StackLedgerEntry{StackID: a.ID, Kind: "App", Key: "App/shop/web", LiveID: "1", SinceRevision: 1}
	if err := db.UpsertStackResource(ctx, e); err != nil {
		t.Fatal(err)
	}
	e.LiveID, e.SinceRevision = "2", 5
	if err := db.UpsertStackResource(ctx, e); err != nil {
		t.Fatalf("re-claim own resource: %v", err)
	}
	e.StackID = b.ID
	if err := db.UpsertStackResource(ctx, e); !errors.Is(err, ErrStackLedgerConflict) {
		t.Fatalf("conflict err = %v", err)
	}
	// Same key under another kind is a different resource.
	if err := db.UpsertStackResource(ctx, StackLedgerEntry{StackID: b.ID, Kind: "Database", Key: "App/shop/web"}); err != nil {
		t.Fatal(err)
	}
	owner, ok, _ := db.GetStackResourceOwner(ctx, "App", "App/shop/web")
	if !ok || owner.StackID != a.ID || owner.LiveID != "2" || owner.SinceRevision != 1 {
		t.Fatalf("owner = %+v", owner)
	}
	if err := db.DeleteStackResource(ctx, a.ID, "App", "App/shop/web"); err != nil {
		t.Fatal(err)
	}
	e.StackID = b.ID
	if err := db.UpsertStackResource(ctx, e); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	if err := db.DeleteStackLedger(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := db.ListStackLedger(ctx, b.ID); len(l) != 0 {
		t.Fatalf("ledger left = %+v", l)
	}
}

func TestStackPin(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	s := newTestStack(t, db, "pinned")
	if err := db.SetStackPin(ctx, s.ID, 1); !errors.Is(err, ErrStackRevisionNotFound) {
		t.Fatalf("pin missing revision err = %v", err)
	}
	if err := db.SetStackPin(ctx, "ghost", 1); !errors.Is(err, ErrStackNotFound) {
		t.Fatalf("pin ghost err = %v", err)
	}
	if _, err := db.AppendStackRevision(ctx, NewStackRevision{StackID: s.ID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStackPin(ctx, s.ID, 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetStack(ctx, s.ID); got.PinnedRevision != 1 {
		t.Fatalf("pin = %d", got.PinnedRevision)
	}
	if err := db.ClearStackPin(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetStack(ctx, s.ID); got.PinnedRevision != 0 {
		t.Fatalf("pin after clear = %d", got.PinnedRevision)
	}
}

func TestStackLock_Lease(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	s := newTestStack(t, db, "locked")
	now := time.Now()
	ttl := time.Minute

	var wins, locked atomic.Int32
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.AcquireStackLock(ctx, s.ID, fmt.Sprintf("runner-%d", i), 1, ttl, now)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrStackLocked):
				locked.Add(1)
			default:
				t.Errorf("acquire: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || locked.Load() != 1 {
		t.Fatalf("wins=%d locked=%d, want 1 and 1", wins.Load(), locked.Load())
	}
	cur, ok, _ := db.GetStackLock(ctx, s.ID)
	if !ok {
		t.Fatal("no lock row")
	}
	loser := "runner-0"
	if cur.Holder == loser {
		loser = "runner-1"
	}

	if _, err := db.AcquireStackLock(ctx, s.ID, loser, 2, ttl, now.Add(ttl-time.Second)); !errors.Is(err, ErrStackLocked) {
		t.Fatalf("live lease takeover err = %v", err)
	}
	if _, err := db.RenewStackLock(ctx, s.ID, loser, ttl, now); !errors.Is(err, ErrStackLockLost) {
		t.Fatalf("renew by non-holder err = %v", err)
	}
	renewed, err := db.RenewStackLock(ctx, s.ID, cur.Holder, ttl, now.Add(30*time.Second))
	if err != nil || !renewed.ExpiresAt.After(cur.ExpiresAt) {
		t.Fatalf("renew = %+v, %v", renewed, err)
	}
	// Past the renewed expiry the other runner takes over.
	took, err := db.AcquireStackLock(ctx, s.ID, loser, 2, ttl, renewed.ExpiresAt)
	if err != nil || took.Holder != loser || took.Revision != 2 {
		t.Fatalf("expired takeover = %+v, %v", took, err)
	}
	// The old holder releasing late must not free the new lease.
	if err := db.ReleaseStackLock(ctx, s.ID, cur.Holder); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetStackLock(ctx, s.ID); !ok {
		t.Fatal("stale release freed the new holder's lease")
	}
	if err := db.ReleaseStackLock(ctx, s.ID, loser); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetStackLock(ctx, s.ID); ok {
		t.Fatal("lease not released")
	}
	if _, err := db.AcquireStackLock(ctx, s.ID, "x", 1, ttl, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ForceReleaseStackLock(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetStackLock(ctx, s.ID); ok {
		t.Fatal("force release left a lease")
	}
	if _, err := db.AcquireStackLock(ctx, "ghost", "x", 1, ttl, now); !errors.Is(err, ErrStackNotFound) {
		t.Fatalf("ghost lock err = %v", err)
	}
}
