package api

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	vitalsPruneEvery   = 10 * time.Minute
	vitalsPruneBatch   = 5000
	vitalsPrunePause   = 100 * time.Millisecond
	vitalsBufferMaxKey = 50000
	vitalsOriginTTL    = 30 * time.Second
)

type vitalsKey struct {
	app, route, metric, device string
	hour                       time.Time
	bucket                     int
}

// vitalsBuffer aggregates increments in memory; new keys are dropped once it is full so a flood cannot grow it.
type vitalsBuffer struct {
	mu  sync.Mutex
	m   map[vitalsKey]int64
	max int
}

func newVitalsBuffer(limit int) *vitalsBuffer {
	return &vitalsBuffer{m: map[vitalsKey]int64{}, max: limit}
}

func (b *vitalsBuffer) add(k vitalsKey, n int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.m[k]; !ok && len(b.m) >= b.max {
		return
	}
	b.m[k] += n
}

func (b *vitalsBuffer) drain() map[vitalsKey]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.m
	b.m = map[vitalsKey]int64{}
	return out
}

type vitalsOriginEntry struct {
	expires time.Time
	hosts   map[string]struct{}
}

// vitalsRuntime is the per router in memory state of the collector.
type vitalsRuntime struct {
	perApp, perIP *apiRateLimiter
	buf           *vitalsBuffer
	gate          *vitalsRouteGate
	originMu      sync.Mutex
	origins       map[string]vitalsOriginEntry
}

var vitalsRuntimes sync.Map

func (rt *Router) vitalsRuntime() *vitalsRuntime {
	if v, ok := vitalsRuntimes.Load(rt); ok {
		return v.(*vitalsRuntime)
	}
	setting := func(key string) func() int {
		return func() int {
			if rt.runtimeSettings == nil {
				return 0
			}
			return rt.runtimeSettings.Int(context.Background(), key)
		}
	}
	v := &vitalsRuntime{
		perApp:  newAPIRateLimiter(0),
		perIP:   newAPIRateLimiter(0),
		buf:     newVitalsBuffer(vitalsBufferMaxKey),
		gate:    newVitalsRouteGate(),
		origins: map[string]vitalsOriginEntry{},
	}
	v.perApp.source, v.perIP.source = setting(KeyVitalsRatePerMin), setting(KeyVitalsRatePerIP)
	actual, _ := vitalsRuntimes.LoadOrStore(rt, v)
	return actual.(*vitalsRuntime)
}

// flush writes the aggregated counters; on failure they go back into the buffer for the next tick.
func (v *vitalsRuntime) flush(ctx context.Context, db vitalsStore) error {
	pending := v.buf.drain()
	if len(pending) == 0 {
		return nil
	}
	rows := make([]store.VitalsCount, 0, len(pending))
	for k, n := range pending {
		rows = append(rows, store.VitalsCount{App: k.app, Route: k.route, Metric: k.metric, Device: k.device, Hour: k.hour, Bucket: k.bucket, Count: n})
	}
	if err := db.AddVitalsCounts(ctx, rows); err != nil {
		for k, n := range pending {
			v.buf.add(k, n)
		}
		return err
	}
	return nil
}

// pruneVitals deletes expired rows in short batches, pausing between them so beacon flushes are not starved.
func pruneVitals(ctx context.Context, db vitalsStore, before time.Time, batch int, pause time.Duration) (int64, error) {
	var total int64
	for {
		n, err := db.PruneVitalsBatch(ctx, before, batch)
		total += n
		if err != nil || n < int64(batch) {
			return total, err
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		case <-time.After(pause):
		}
	}
}

// RunVitalsWorker flushes buffered counters on the live interval and prunes old rows, so retention holds with no beacons.
// A crash loses at most one flush interval of counters; a clean shutdown flushes once more.
func (rt *Router) RunVitalsWorker(ctx context.Context) error {
	db, ok := rt.vitalsDB()
	if !ok || rt.runtimeSettings == nil {
		return nil
	}
	v := rt.vitalsRuntime()
	var lastPrune time.Time
	for {
		wait := time.Duration(max(rt.runtimeSettings.Int(ctx, KeyVitalsFlushSeconds), 1)) * time.Second
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			_ = v.flush(fctx, db)
			cancel()
			return ctx.Err()
		case <-time.After(wait):
		}
		if err := v.flush(ctx, db); err != nil {
			rt.logger.Warn("api: vitals flush failed", slog.String("error", err.Error()))
		}
		if now := time.Now(); now.Sub(lastPrune) >= vitalsPruneEvery {
			lastPrune = now
			days := rt.runtimeSettings.Int(ctx, KeyVitalsRetentionDay)
			if _, err := pruneVitals(ctx, db, now.Add(-time.Duration(days)*24*time.Hour), vitalsPruneBatch, vitalsPrunePause); err != nil {
				rt.logger.Warn("api: prune vitals failed", slog.String("error", err.Error()))
			}
		}
	}
}

// vitalsOriginAllowed accepts the app's own domains and localhost; an app with no domains accepts any origin.
// Origin is advisory: a non browser client can send any value. A store error denies.
func (rt *Router) vitalsOriginAllowed(ctx context.Context, app, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || rt.domains == nil {
		return true
	}
	v := rt.vitalsRuntime()
	now := time.Now()
	v.originMu.Lock()
	e, ok := v.origins[app]
	v.originMu.Unlock()
	if !ok || now.After(e.expires) {
		doms, err := rt.domains.ListServiceDomains(ctx)
		if err != nil {
			return false
		}
		e = vitalsOriginEntry{expires: now.Add(vitalsOriginTTL), hosts: map[string]struct{}{}}
		for _, d := range doms {
			if d.ServiceName == app {
				e.hosts[strings.ToLower(d.Domain)] = struct{}{}
			}
		}
		v.originMu.Lock()
		v.origins[app] = e
		v.originMu.Unlock()
	}
	if len(e.hosts) == 0 {
		return true
	}
	_, ok = e.hosts[host]
	return ok
}
