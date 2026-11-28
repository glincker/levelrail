package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/dbviewer"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
)

const (
	envAppsSummaryCacheSeconds = "APP_APPS_SUMMARY_CACHE_SECONDS"
	envAppsSummaryCacheMax     = "APP_APPS_SUMMARY_CACHE_MAX_ENTRIES"
	envAppsSummaryTimeout      = "APP_APPS_SUMMARY_TIMEOUT"

	defaultAppsSummaryCacheSeconds = 30
	defaultAppsSummaryCacheMax     = 500
	defaultAppsSummaryTimeout      = 800 * time.Millisecond
	// summaryFetchWorkers bounds node round trips one list request starts.
	summaryFetchWorkers = 8
)

// liveSignals are the optional values read from a node for one app. A nil field means unknown.
type liveSignals struct {
	restarts   *int
	increasing bool
	dbSize     *int64
	at         time.Time
}

// summaryCache is a bounded name to signals map; an expired entry stays until evicted so a rising restart count can be seen.
type summaryCache struct {
	mu sync.Mutex
	m  map[string]liveSignals
}

func (c *summaryCache) fresh(name string, now time.Time, ttl time.Duration) (liveSignals, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.m[name]
	return s, ok && now.Sub(s.at) < ttl
}

func (c *summaryCache) put(name string, s liveSignals, now time.Time, maxEntries int) liveSignals {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]liveSignals)
	}
	prev, had := c.m[name]
	if s.restarts != nil && had && prev.restarts != nil {
		switch {
		case *s.restarts > *prev.restarts:
			s.increasing = true
		case *s.restarts == *prev.restarts:
			s.increasing = prev.increasing
		}
	}
	s.at = now
	if !had {
		for len(c.m) >= maxEntries && len(c.m) > 0 {
			oldest, oldestAt := "", now
			for k, v := range c.m {
				if oldest == "" || v.at.Before(oldestAt) {
					oldest, oldestAt = k, v.at
				}
			}
			delete(c.m, oldest)
		}
	}
	c.m[name] = s
	return s
}

// summaryCacheKey keeps entries holding a database size apart from those without one.
func summaryCacheKey(name string, withDBSize bool) string {
	if withDBSize {
		return name + "\x00db"
	}
	return name
}

// summaryCacheTTL is the live setting; 0 or less disables live signals.
func (rt *Router) summaryCacheTTL(ctx context.Context) time.Duration {
	n := defaultAppsSummaryCacheSeconds
	if rt.runtimeSettings != nil {
		n = rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppsSummaryCacheSeconds)
	} else {
		n = envIntDefault(envAppsSummaryCacheSeconds, n)
	}
	return time.Duration(n) * time.Second
}

// liveSummarySignals reads restarts and database size for the given page of apps, within one short deadline.
// Apps that fail or time out are absent from the result.
// withDBSize is true only for callers allowed to see database sizes; the exec never runs otherwise.
func (rt *Router) liveSummarySignals(ctx context.Context, svcs []store.DesiredService, kinds map[string]string, now time.Time, withDBSize bool) map[string]liveSignals {
	ttl := rt.summaryCacheTTL(ctx)
	if ttl <= 0 {
		return nil
	}
	maxEntries := rt.tunableInt(ctx, runtimesettings.KeyAppsSummaryCacheMaxEntries, envAppsSummaryCacheMax, defaultAppsSummaryCacheMax)
	timeout := rt.tunableDuration(ctx, runtimesettings.KeyAppsSummaryTimeout, envAppsSummaryTimeout, defaultAppsSummaryTimeout)

	out := make(map[string]liveSignals, len(svcs))
	var stale []store.DesiredService
	for _, s := range svcs {
		if kinds[s.Name] == appKindCron {
			continue
		}
		if sig, ok := rt.appSummaryCache.fresh(summaryCacheKey(s.Name, withDBSize), now, ttl); ok {
			out[s.Name] = sig
			continue
		}
		stale = append(stale, s)
	}
	if len(stale) == 0 {
		return out
	}

	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		sem  = make(chan struct{}, summaryFetchWorkers)
		done = make(chan struct{})
	)
	for _, s := range stale {
		wg.Add(1)
		go func(s store.DesiredService) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-fctx.Done():
				return
			}
			sig, ok := rt.fetchLiveSignals(fctx, s, kinds[s.Name], withDBSize)
			if !ok {
				return
			}
			sig = rt.appSummaryCache.put(summaryCacheKey(s.Name, withDBSize), sig, time.Now(), maxEntries)
			mu.Lock()
			out[s.Name] = sig
			mu.Unlock()
		}(s)
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-fctx.Done():
	}
	mu.Lock()
	defer mu.Unlock()
	snap := make(map[string]liveSignals, len(out))
	for k, v := range out {
		snap[k] = v
	}
	return snap
}

func (rt *Router) fetchLiveSignals(ctx context.Context, svc store.DesiredService, kind string, withDBSize bool) (liveSignals, bool) {
	var sig liveSignals
	if n, ok := rt.liveRestarts(ctx, svc); ok {
		sig.restarts = &n
	}
	if withDBSize && kind == appKindDatabase {
		if size, ok := rt.liveDBSize(ctx, svc.Name); ok {
			sig.dbSize = &size
		}
	}
	return sig, sig.restarts != nil || sig.dbSize != nil
}

// liveRestarts sums Docker's RestartCount over the app's replicas through the node's inspector.
func (rt *Router) liveRestarts(ctx context.Context, svc store.DesiredService) (int, bool) {
	lister, err := rt.exposureLister(svc.NodeID)
	if err != nil || lister == nil {
		return 0, false
	}
	inspector, ok := lister.(docker.ContainerDetailInspector)
	if !ok {
		return 0, false
	}
	src, ok := rt.appGroups.(interface {
		GetOrCreateInstanceID(ctx context.Context) (string, error)
	})
	if !ok {
		return 0, false
	}
	instanceID, err := src.GetOrCreateInstanceID(ctx)
	if err != nil || instanceID == "" {
		return 0, false
	}
	list, err := inspector.InspectContainerDetails(ctx, docker.DetailQuery{
		NamePrefix: svc.Name + "-", LabelKey: spec.InstanceLabelKey, LabelValue: instanceID, Limit: max(svc.Replicas, 1) * 2,
	})
	if err != nil {
		return 0, false
	}
	total, seen := 0, false
	for _, d := range list {
		if d.InstanceID == instanceID && application.OwnsContainer(svc.Name, d.Name) {
			total += d.RestartCount
			seen = true
		}
	}
	return total, seen
}

// liveDBSize reuses the database viewer's catalog read, so only SQL databases Levelrail runs report a size.
func (rt *Router) liveDBSize(ctx context.Context, name string) (int64, bool) {
	if rt.databases == nil || rt.execRuntime == nil {
		return 0, false
	}
	desired, err := rt.databases.GetDesiredDatabase(ctx, name)
	if err != nil || desired == nil {
		if err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
			rt.logger.Debug("api: apps summary: load database failed", "name", name)
		}
		return 0, false
	}
	var dialect dbviewer.Dialect
	switch desired.Engine {
	case store.EnginePostgres:
		dialect = dbviewer.DialectPostgres
	case store.EngineMySQL:
		dialect = dbviewer.DialectMySQL
	case store.EngineMariaDB:
		dialect = dbviewer.DialectMariaDB
	default:
		return 0, false
	}
	nodeRuntime, err := rt.execRuntime(desired.NodeID)
	if err != nil {
		return 0, false
	}
	state, err := nodeRuntime.InspectByName(ctx, databaseContainerName(name))
	if err != nil || state == nil || !state.Running {
		return 0, false
	}
	ev, err := dbviewer.Target{
		Exec: nodeRuntime, ContainerID: state.ID, Dialect: dialect, Limits: rt.databaseViewerLimits(ctx),
	}.Evidence(ctx)
	if err != nil {
		return 0, false
	}
	return ev.DatabaseSizeBytes, true
}

// applyLiveSignals copies the known live values onto the summary.
func applyLiveSignals(s *appListSummary, sig liveSignals) {
	if s == nil {
		return
	}
	s.Restarts = sig.restarts
	s.RestartsIncreasing = sig.increasing && sig.restarts != nil
	s.DBSizeBytes = sig.dbSize
}
