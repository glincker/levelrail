package stacksync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func waitIdle(t *testing.T, s *Syncer, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.Busy(id) {
		if time.Now().After(deadline) {
			t.Fatal("triggered sync never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStackSyncTriggerCoalesces(t *testing.T) {
	rec := store.StackSyncRecord{StackID: "s1", StackName: "infra", Config: store.StackSyncConfig{RepoURL: "https://git.example.invalid/o/r.git", Path: "stacks", Enabled: true, Branch: "main"}}
	release := make(chan struct{})
	var runs, active, overlap atomic.Int32
	s := &Syncer{Store: &memStore{recs: []store.StackSyncRecord{rec}}, Fetch: func(context.Context, store.StackSyncRecord) (string, string, func(), error) {
		if active.Add(1) > 1 {
			overlap.Add(1)
		}
		defer active.Add(-1)
		if runs.Add(1) == 1 {
			<-release
		}
		return "", "", nil, errors.New("offline")
	}}
	if got := s.Trigger(rec, true); got != TriggerQueued {
		t.Fatalf("first trigger = %s, want queued", got)
	}
	for range 9 {
		if got := s.Trigger(rec, false); got != TriggerCoalesced {
			t.Fatalf("burst trigger = %s, want coalesced", got)
		}
	}
	close(release)
	waitIdle(t, s, rec.StackID)
	if n := runs.Load(); n != 2 {
		t.Fatalf("runs = %d, want 2 (one running, one pending)", n)
	}
	if overlap.Load() != 0 {
		t.Fatal("two syncs of one stack ran at once")
	}
}

func TestStackSyncTriggerRefreshPlansOnly(t *testing.T) {
	rec := store.StackSyncRecord{StackID: "s2", StackName: "infra", Config: store.StackSyncConfig{RepoURL: "https://git.example.invalid/o/r.git", Path: "stacks", Branch: "main", Enabled: true, Apply: true}}
	var sawApply atomic.Bool
	s := &Syncer{Store: &memStore{recs: []store.StackSyncRecord{rec}}, Fetch: func(_ context.Context, r store.StackSyncRecord) (string, string, func(), error) {
		if r.Config.Apply {
			sawApply.Store(true)
		}
		return "", "", nil, errors.New("offline")
	}}
	s.Trigger(rec, false)
	waitIdle(t, s, rec.StackID)
	if sawApply.Load() {
		t.Fatal("refresh trigger handed apply to the sync")
	}
}

func TestStackSyncWebhookOnlyNotPolled(t *testing.T) {
	rec := store.StackSyncRecord{StackID: "s3", StackName: "infra", Config: store.StackSyncConfig{RepoURL: "https://git.example.invalid/o/r.git", Path: "stacks", Branch: "main", Enabled: true, WebhookOnly: true}}
	var fetched atomic.Int32
	s := &Syncer{Store: &memStore{recs: []store.StackSyncRecord{rec}}, Fetch: func(context.Context, store.StackSyncRecord) (string, string, func(), error) {
		fetched.Add(1)
		return "", "", nil, errors.New("offline")
	}}
	for range 3 {
		s.pass(t.Context())
	}
	if fetched.Load() != 0 {
		t.Fatalf("webhook only stack polled %d times", fetched.Load())
	}
	s.Trigger(rec, false)
	waitIdle(t, s, rec.StackID)
	if fetched.Load() != 1 {
		t.Fatalf("webhook trigger fetched %d times, want 1", fetched.Load())
	}
}
