package api

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

const (
	deploySummaryCacheMax = 64
)

type deploySummaryEntry struct {
	at     time.Time
	briefs []store.DeploymentBrief
}

var deploySummaryCache = struct {
	mu sync.Mutex
	m  map[string]deploySummaryEntry
}{m: map[string]deploySummaryEntry{}}

// deploySummaryTTL reads the live setting per call; 0 disables the cache.
func (rt *Router) deploySummaryTTL(ctx context.Context) time.Duration {
	return time.Duration(max(0, rt.runtimeSettings.Int(ctx, runtimesettings.KeyDeploySummaryCacheSeconds))) * time.Second
}

// cachedDeploymentBriefs memoizes the 30 day brief scan: the dashboard polls it and every call
// reads every recent attempt. The key carries the caller's visible set so authz is unchanged.
func cachedDeploymentBriefs(ctx context.Context, ds deploymentStore, since time.Time, visible []string, env, project string, ttl time.Duration) ([]store.DeploymentBrief, error) {
	if ttl <= 0 {
		return ds.ListDeploymentBriefsScoped(ctx, since, visible, env, project)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join(visible, "\x00")))
	key := strings.Join([]string{since.Truncate(time.Minute).Format(time.RFC3339), env, project, fmt.Sprint(visible == nil, len(visible)), fmt.Sprint(h.Sum64())}, "|") + "|" + fmt.Sprintf("%p", ds)
	now := time.Now()
	deploySummaryCache.mu.Lock()
	e, ok := deploySummaryCache.m[key]
	deploySummaryCache.mu.Unlock()
	if ok && now.Sub(e.at) < ttl {
		return e.briefs, nil
	}
	briefs, err := ds.ListDeploymentBriefsScoped(ctx, since, visible, env, project)
	if err != nil {
		return nil, err
	}
	deploySummaryCache.mu.Lock()
	if len(deploySummaryCache.m) >= deploySummaryCacheMax {
		deploySummaryCache.m = map[string]deploySummaryEntry{}
	}
	deploySummaryCache.m[key] = deploySummaryEntry{at: now, briefs: briefs}
	deploySummaryCache.mu.Unlock()
	return briefs, nil
}
