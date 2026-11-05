package dnsstatus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memWatchStore struct {
	mu   sync.Mutex
	rows map[string]WatchEntry
}

func newMemWatchStore() *memWatchStore { return &memWatchStore{rows: map[string]WatchEntry{}} }

func (s *memWatchStore) ListWatch(context.Context) ([]WatchEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]WatchEntry, 0, len(s.rows))
	for _, e := range s.rows {
		out = append(out, e)
	}
	return out, nil
}

func (s *memWatchStore) SaveWatch(_ context.Context, e WatchEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[e.Domain] = e
	return nil
}

func (s *memWatchStore) DeleteWatch(_ context.Context, d string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, d)
	return nil
}

type watchRig struct {
	now      time.Time
	state    WatchState
	checkErr error
	checks   int
	notices  []WatchEntry
	store    *memWatchStore
	mu       sync.Mutex
}

const (
	testInterval = 10 * time.Second
	testLimit    = time.Minute
)

func (r *watchRig) watcher() *Watcher {
	return NewWatcher(WatchConfig{
		Store: r.store,
		Now:   func() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.now },
		Check: func(context.Context, string, string) (WatchState, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.checks++
			return r.state, r.checkErr
		},
		Notify: func(_ context.Context, e WatchEntry) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.notices = append(r.notices, e)
			return nil
		},
		Settings: func(context.Context) (time.Duration, time.Duration) { return testInterval, testLimit },
	})
}

func (r *watchRig) advance(d time.Duration) { r.mu.Lock(); r.now = r.now.Add(d); r.mu.Unlock() }

func newRig() *watchRig {
	return &watchRig{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), state: WatchWaitingDNS, store: newMemWatchStore()}
}

func TestWatcher_TransitionsToLiveAndNotifiesOnce(t *testing.T) {
	r := newRig()
	w := r.watcher()
	ctx := context.Background()
	w.Register(ctx, "web", "Example.com.")
	for _, st := range []WatchState{WatchWaitingDNS, WatchPropagating, WatchWaitingCert, WatchLive} {
		r.state = st
		w.Tick(ctx)
		if got, _ := w.Get("example.com"); got.State != st {
			t.Fatalf("state = %s, want %s", got.State, st)
		}
		r.advance(testInterval)
	}
	checks := r.checks
	w.Tick(ctx)
	r.advance(time.Hour)
	w.Tick(ctx)
	if r.checks != checks {
		t.Fatalf("live domain polled again: %d checks, want %d", r.checks, checks)
	}
	if len(r.notices) != 1 || r.notices[0].Domain != "example.com" {
		t.Fatalf("notices = %+v, want one for example.com", r.notices)
	}
}

func TestWatcher_BackoffGrowsAndStaysBounded(t *testing.T) {
	if got := backoffDelay(testInterval, 0); got != testInterval {
		t.Fatalf("first delay = %s", got)
	}
	if got := backoffDelay(testInterval, watchBackoffEvery); got != 2*testInterval {
		t.Fatalf("delay after %d checks = %s", watchBackoffEvery, got)
	}
	if got := backoffDelay(testInterval, 10_000); got != testInterval<<watchBackoffMaxShift {
		t.Fatalf("capped delay = %s", got)
	}
}

func TestWatcher_NotDueIsNotChecked(t *testing.T) {
	r := newRig()
	w := r.watcher()
	ctx := context.Background()
	w.Register(ctx, "web", "a.test")
	w.Tick(ctx)
	w.Tick(ctx)
	if r.checks != 1 {
		t.Fatalf("checks = %d, want 1 before the interval passes", r.checks)
	}
	r.advance(testInterval)
	w.Tick(ctx)
	if r.checks != 2 {
		t.Fatalf("checks = %d, want 2 after the interval", r.checks)
	}
}

func TestWatcher_StallsThenSlowsThenStops(t *testing.T) {
	r := newRig()
	w := r.watcher()
	ctx := context.Background()
	w.Register(ctx, "web", "a.test")
	r.advance(testLimit)
	w.Tick(ctx)
	e, _ := w.Get("a.test")
	if e.State != WatchStalled {
		t.Fatalf("state = %s, want stalled", e.State)
	}
	if want := r.now.Add(watchStalledEvery); !e.NextCheck.Equal(want) {
		t.Fatalf("next check = %s, want hourly %s", e.NextCheck, want)
	}
	r.advance(watchStalledWindow)
	w.Tick(ctx)
	e, _ = w.Get("a.test")
	if !e.Stopped {
		t.Fatal("entry still polling after the stalled window")
	}
	n := r.checks
	r.advance(time.Hour)
	w.Tick(ctx)
	if r.checks != n {
		t.Fatal("stopped entry was checked")
	}
	// Going live later still works once it is registered again.
	r.state = WatchLive
	w.Register(ctx, "web", "a.test")
	w.Tick(ctx)
	if e, _ = w.Get("a.test"); e.State != WatchLive {
		t.Fatalf("state after re-register = %s", e.State)
	}
}

func TestWatcher_CheckErrorKeepsState(t *testing.T) {
	r := newRig()
	w := r.watcher()
	ctx := context.Background()
	w.Register(ctx, "web", "a.test")
	r.state = WatchPropagating
	w.Tick(ctx)
	r.advance(testInterval)
	r.checkErr = errors.New("resolver down")
	w.Tick(ctx)
	if e, _ := w.Get("a.test"); e.State != WatchPropagating || e.Attempts != 2 {
		t.Fatalf("entry = %+v, want propagating after 2 attempts", e)
	}
}

func TestWatcher_RestartResumesAndNoticeIsIdempotent(t *testing.T) {
	r := newRig()
	ctx := context.Background()
	first := r.watcher()
	first.Register(ctx, "web", "a.test")
	first.Register(ctx, "web", "b.test")
	r.state = WatchLive
	first.Tick(ctx)
	if len(r.notices) != 2 {
		t.Fatalf("notices = %d, want 2", len(r.notices))
	}

	second := r.watcher()
	if err := second.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if got := second.List(); len(got) != 2 || got[0].State != WatchLive {
		t.Fatalf("resumed = %+v", got)
	}
	r.advance(time.Hour)
	second.Tick(ctx)
	if len(r.notices) != 2 {
		t.Fatalf("notices after restart = %d, want still 2", len(r.notices))
	}

	// A crash between save and notice must not repeat it: NotifiedAt is already stored.
	r.store.rows["c.test"] = WatchEntry{Domain: "c.test", State: WatchWaitingDNS, Since: r.now, NextCheck: r.now, NotifiedAt: r.now}
	third := r.watcher()
	_ = third.Load(ctx)
	third.Tick(ctx)
	if len(r.notices) != 2 {
		t.Fatalf("notices = %d, want no repeat for an already notified row", len(r.notices))
	}
}

func TestWatcher_UnregisterStopsPolling(t *testing.T) {
	r := newRig()
	w := r.watcher()
	ctx := context.Background()
	w.Register(ctx, "web", "a.test")
	w.Unregister(ctx, "a.test")
	r.advance(time.Hour)
	w.Tick(ctx)
	if r.checks != 0 {
		t.Fatalf("removed domain was checked %d times", r.checks)
	}
	if _, ok := r.store.rows["a.test"]; ok {
		t.Fatal("row still persisted")
	}
}

func TestWatcher_EventsArriveInOrder(t *testing.T) {
	r := newRig()
	w := r.watcher()
	ctx := context.Background()
	w.Register(ctx, "web", "a.test")
	snap, events, cancel := w.SubscribeWithSnapshot()
	defer cancel()
	if len(snap) != 1 || snap[0].Domain != "a.test" {
		t.Fatalf("snapshot = %+v", snap)
	}
	for _, st := range []WatchState{WatchPropagating, WatchLive} {
		r.state = st
		w.Tick(ctx)
		r.advance(testInterval)
	}
	w.Unregister(ctx, "a.test")
	want := []struct {
		kind  string
		state WatchState
	}{{WatchEventUpdate, WatchPropagating}, {WatchEventUpdate, WatchLive}, {WatchEventRemoved, WatchLive}}
	for i, wnt := range want {
		select {
		case ev := <-events:
			if ev.Kind != wnt.kind || ev.Entry.State != wnt.state {
				t.Fatalf("event %d = %s/%s, want %s/%s", i, ev.Kind, ev.Entry.State, wnt.kind, wnt.state)
			}
		case <-time.After(50 * time.Millisecond):
			t.Fatalf("event %d missing", i)
		}
	}
}
