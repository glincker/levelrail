package dnsstatus

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// WatchState is where a watched domain is on its way to serving traffic.
type WatchState string

// Watch states, in the order a healthy domain passes through them.
const (
	WatchWaitingDNS  WatchState = "waiting_dns"
	WatchPropagating WatchState = "propagating"
	WatchWaitingCert WatchState = "waiting_certificate"
	WatchLive        WatchState = "live"
	WatchStalled     WatchState = "stalled"
)

// Event kinds a subscriber sees.
const (
	WatchEventSnapshot = "snapshot"
	WatchEventUpdate   = "update"
	WatchEventRemoved  = "removed"
)

const (
	// watchScanEvery is how often Run looks for domains that are due.
	watchScanEvery = time.Second
	// watchCheckTimeout bounds one domain check so a hung resolver cannot stall the pass.
	watchCheckTimeout = 20 * time.Second
	// watchConcurrency bounds checks in flight at once.
	watchConcurrency = 4
	// watchBackoffEvery and watchBackoffMaxShift double the delay every N checks, up to 8x.
	watchBackoffEvery    = 10
	watchBackoffMaxShift = 3
	// watchStalledEvery and watchStalledWindow are the slow hourly polling after the time limit.
	watchStalledEvery  = time.Hour
	watchStalledWindow = 24 * time.Hour
	// watchLiveRetention is how long a live row stays so a refresh still shows it.
	watchLiveRetention = 24 * time.Hour
	// subscriberBuffer is each subscriber's queue; a slower consumer misses events.
	subscriberBuffer = 64
)

// WatchEntry is one watched domain, also the JSON a client sees.
type WatchEntry struct {
	Domain      string     `json:"domain"`
	App         string     `json:"app"`
	State       WatchState `json:"state"`
	Since       time.Time  `json:"since"`
	LastChecked time.Time  `json:"last_checked,omitzero"`
	NextCheck   time.Time  `json:"next_check,omitzero"`
	Attempts    int        `json:"attempts"`
	StalledAt   time.Time  `json:"stalled_at,omitzero"`
	Stopped     bool       `json:"stopped,omitempty"`
	NotifiedAt  time.Time  `json:"notified_at,omitzero"`
}

// WatchEvent is what the stream carries: the entry and why it was sent.
type WatchEvent struct {
	Kind  string     `json:"kind"`
	Entry WatchEntry `json:"entry"`
}

// WatchStore persists the watched set so a restart resumes it.
type WatchStore interface {
	ListWatch(ctx context.Context) ([]WatchEntry, error)
	SaveWatch(ctx context.Context, e WatchEntry) error
	DeleteWatch(ctx context.Context, domain string) error
}

// WatchCheck reports how far a domain has come. An error keeps the previous state.
type WatchCheck func(ctx context.Context, domain, app string) (WatchState, error)

// WatchNotify tells the user a domain became live. Called once per becoming-live.
type WatchNotify func(ctx context.Context, e WatchEntry) error

// WatchSettings returns the live poll interval and the time limit before a domain is stalled.
type WatchSettings func(ctx context.Context) (interval, limit time.Duration)

// WatchConfig wires a Watcher. Now defaults to time.Now; tests pass a fake clock.
type WatchConfig struct {
	Store    WatchStore
	Check    WatchCheck
	Notify   WatchNotify
	Settings WatchSettings
	Now      func() time.Time
	Logger   *slog.Logger
}

// Watcher follows domains that are not answering yet and publishes each change.
type Watcher struct {
	cfg     WatchConfig
	mu      sync.Mutex
	entries map[string]WatchEntry
	busy    map[string]bool
	subs    map[int]chan WatchEvent
	nextSub int
}

// NewWatcher builds a Watcher; call Load (or Run) to resume persisted domains.
func NewWatcher(cfg WatchConfig) *Watcher {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Watcher{cfg: cfg, entries: map[string]WatchEntry{}, busy: map[string]bool{}, subs: map[int]chan WatchEvent{}}
}

func normalizeWatchDomain(d string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))
}

// Load reads the persisted set into memory.
func (w *Watcher) Load(ctx context.Context) error {
	rows, err := w.cfg.Store.ListWatch(ctx)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range rows {
		w.entries[e.Domain] = e
	}
	return nil
}

// Run resumes the persisted set and polls until ctx ends.
func (w *Watcher) Run(ctx context.Context) error {
	if err := w.Load(ctx); err != nil {
		w.cfg.Logger.Error("dnsstatus: load watched domains failed", slog.String("error", err.Error()))
	}
	// Registered so the idle footprint report lists the one-second scan.
	return footprint.Every(ctx, "dns watch scan", watchScanEvery, w.cfg.Logger, func(ctx context.Context) error {
		w.Tick(ctx)
		return nil
	})
}

// Register starts watching domain for app. A live domain only has its app updated;
// a stalled or stopped one starts over.
func (w *Watcher) Register(ctx context.Context, app, domain string) WatchEntry {
	domain = normalizeWatchDomain(domain)
	now := w.cfg.Now().UTC()
	w.mu.Lock()
	e, ok := w.entries[domain]
	switch {
	case ok && e.State == WatchLive:
		e.App = app
	case ok && !e.Stopped && e.State != WatchStalled:
		e.App = app
	default:
		e = WatchEntry{Domain: domain, App: app, State: WatchWaitingDNS, Since: now, NextCheck: now}
	}
	w.entries[domain] = e
	w.mu.Unlock()
	w.persist(ctx, e)
	w.publish(WatchEvent{Kind: WatchEventUpdate, Entry: e})
	return e
}

// Unregister stops watching domain.
func (w *Watcher) Unregister(ctx context.Context, domain string) {
	domain = normalizeWatchDomain(domain)
	w.mu.Lock()
	e, ok := w.entries[domain]
	delete(w.entries, domain)
	w.mu.Unlock()
	if !ok {
		return
	}
	if err := w.cfg.Store.DeleteWatch(ctx, domain); err != nil {
		w.cfg.Logger.Error("dnsstatus: delete watched domain failed", slog.String("domain", domain), slog.String("error", err.Error()))
	}
	w.publish(WatchEvent{Kind: WatchEventRemoved, Entry: e})
}

// Get returns the entry for domain.
func (w *Watcher) Get(domain string) (WatchEntry, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.entries[normalizeWatchDomain(domain)]
	return e, ok
}

// List returns every entry ordered by domain.
func (w *Watcher) List() []WatchEntry {
	w.mu.Lock()
	out := make([]WatchEntry, 0, len(w.entries))
	for _, e := range w.entries {
		out = append(out, e)
	}
	w.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

// Subscribe returns a stream of changes and the function that ends it.
func (w *Watcher) Subscribe() (<-chan WatchEvent, func()) {
	ch := make(chan WatchEvent, subscriberBuffer)
	w.mu.Lock()
	id := w.nextSub
	w.nextSub++
	w.subs[id] = ch
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		delete(w.subs, id)
		w.mu.Unlock()
	}
}

// SubscribeWithSnapshot subscribes and lists in one step so no change falls between them.
func (w *Watcher) SubscribeWithSnapshot() ([]WatchEntry, <-chan WatchEvent, func()) {
	ch := make(chan WatchEvent, subscriberBuffer)
	w.mu.Lock()
	id := w.nextSub
	w.nextSub++
	w.subs[id] = ch
	snap := make([]WatchEntry, 0, len(w.entries))
	for _, e := range w.entries {
		snap = append(snap, e)
	}
	w.mu.Unlock()
	sort.Slice(snap, func(i, j int) bool { return snap[i].Domain < snap[j].Domain })
	return snap, ch, func() {
		w.mu.Lock()
		delete(w.subs, id)
		w.mu.Unlock()
	}
}

func (w *Watcher) publish(ev WatchEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ch := range w.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (w *Watcher) persist(ctx context.Context, e WatchEntry) {
	if err := w.cfg.Store.SaveWatch(ctx, e); err != nil {
		w.cfg.Logger.Error("dnsstatus: save watched domain failed", slog.String("domain", e.Domain), slog.String("error", err.Error()))
	}
}

// Tick checks every domain that is due, at most watchConcurrency at once, and waits for them.
func (w *Watcher) Tick(ctx context.Context) {
	now := w.cfg.Now().UTC()
	var due []WatchEntry
	var expired []string
	w.mu.Lock()
	for d, e := range w.entries {
		switch {
		case e.State == WatchLive && !e.LastChecked.IsZero() && now.Sub(e.LastChecked) >= watchLiveRetention:
			expired = append(expired, d)
		case e.State == WatchLive || e.Stopped || w.busy[d] || e.NextCheck.After(now):
		default:
			w.busy[d] = true
			due = append(due, e)
		}
	}
	w.mu.Unlock()
	for _, d := range expired {
		w.Unregister(ctx, d)
	}
	sem := make(chan struct{}, watchConcurrency)
	var wg sync.WaitGroup
	for _, e := range due {
		sem <- struct{}{}
		wg.Add(1)
		go func(e WatchEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			w.checkOne(ctx, e)
		}(e)
	}
	wg.Wait()
}

func (w *Watcher) checkOne(ctx context.Context, e WatchEntry) {
	cctx, cancel := context.WithTimeout(ctx, watchCheckTimeout)
	state, err := w.cfg.Check(cctx, e.Domain, e.App)
	cancel()
	now := w.cfg.Now().UTC()
	interval, limit := w.cfg.Settings(ctx)

	w.mu.Lock()
	delete(w.busy, e.Domain)
	cur, ok := w.entries[e.Domain]
	if !ok {
		w.mu.Unlock()
		return
	}
	cur.Attempts++
	cur.LastChecked = now
	notify := false
	switch {
	case err == nil && state == WatchLive:
		cur.State, cur.NextCheck, cur.StalledAt, cur.Stopped = WatchLive, time.Time{}, time.Time{}, false
		if cur.NotifiedAt.IsZero() {
			cur.NotifiedAt, notify = now, true
		}
	default:
		if err == nil && cur.State != WatchStalled {
			cur.State = state
		}
		if cur.StalledAt.IsZero() && now.Sub(cur.Since) >= limit {
			cur.State, cur.StalledAt = WatchStalled, now
		}
		switch {
		case cur.StalledAt.IsZero():
			cur.NextCheck = now.Add(backoffDelay(interval, cur.Attempts))
		case now.Sub(cur.StalledAt) >= watchStalledWindow:
			cur.Stopped, cur.NextCheck = true, time.Time{}
		default:
			cur.NextCheck = now.Add(watchStalledEvery)
		}
	}
	w.entries[e.Domain] = cur
	w.mu.Unlock()

	// Saved before the notice so a crash in between loses a notice instead of repeating it.
	w.persist(ctx, cur)
	w.publish(WatchEvent{Kind: WatchEventUpdate, Entry: cur})
	if notify && w.cfg.Notify != nil {
		if nerr := w.cfg.Notify(ctx, cur); nerr != nil {
			w.cfg.Logger.Error("dnsstatus: live notice failed", slog.String("domain", cur.Domain), slog.String("error", nerr.Error()))
		}
	}
}

func backoffDelay(interval time.Duration, attempts int) time.Duration {
	shift := min(attempts/watchBackoffEvery, watchBackoffMaxShift)
	return interval << shift
}
