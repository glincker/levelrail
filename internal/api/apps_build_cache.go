package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionAppBuildCacheClear is the audit event for clearing one app's build cache.
const AuditActionAppBuildCacheClear = "app.build_cache.clear"

// appCacheUsageTTL keeps the size read cheap when the dashboard polls.
const appCacheUsageTTL = 10 * time.Second

// AppBuildCacher is the BuildKit surface the per app cache routes need.
// *build.Client satisfies it.
type AppBuildCacher interface {
	AppCacheUsage(ctx context.Context, cacheID string, siblings []string) (build.AppCacheUsage, error)
	PruneAppCache(ctx context.Context, cacheID string, siblings []string) (build.AppCachePruneResult, error)
}

// WithAppBuildCache enables GET and DELETE /api/v1/apps/{name}/build-cache.
func WithAppBuildCache(b AppBuildCacher) Option {
	return func(rt *Router) { rt.appBuildCache = b }
}

type appBuildCacheResource struct {
	CacheID     string     `json:"cache_id"`
	SizeBytes   int64      `json:"size_bytes"`
	RecordCount int        `json:"record_count"`
	InUse       bool       `json:"in_use"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type appBuildCacheClearResponse struct {
	CacheID      string `json:"cache_id"`
	FreedBytes   int64  `json:"freed_bytes"`
	RemovedCount int    `json:"removed_count"`
}

type appCacheMemo struct {
	mu      sync.Mutex
	entries map[string]appCacheMemoEntry
}

type appCacheMemoEntry struct {
	at  time.Time
	res appBuildCacheResource
}

var appCacheUsageMemo = &appCacheMemo{entries: map[string]appCacheMemoEntry{}}

func (m *appCacheMemo) get(id string) (appBuildCacheResource, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok || time.Since(e.at) > appCacheUsageTTL {
		return appBuildCacheResource{}, false
	}
	return e.res, true
}

func (m *appCacheMemo) put(id string, res appBuildCacheResource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[id] = appCacheMemoEntry{at: time.Now(), res: res}
}

func (m *appCacheMemo) drop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, id)
}

// appCacheScope validates the app and returns its cache id and the other apps' ids.
func (rt *Router) appCacheScope(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	if rt.appBuildCache == nil {
		writeError(w, http.StatusNotImplemented, "build cache management is not configured on this control plane")
		return "", nil, false
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", nil, false
	} else if err != nil {
		rt.internalError(w, "api: app build cache: load app", err, slog.String("name", name))
		return "", nil, false
	}
	id := build.SanitizeCacheID(name, "")
	if id == "" {
		writeError(w, http.StatusConflict, "this app has no usable cache id")
		return "", nil, false
	}
	var siblings []string
	if all, err := rt.apps.ListDesiredServices(r.Context()); err == nil {
		for _, s := range all {
			if sid := build.SanitizeCacheID(s.Name, ""); sid != "" && sid != id {
				siblings = append(siblings, sid)
			}
		}
	}
	return id, siblings, true
}

func (rt *Router) appBuildRunning(ctx context.Context, name string) (bool, error) {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return false, err
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusRunning {
			return true, nil
		}
	}
	return false, nil
}

// handleGetAppBuildCache handles GET /api/v1/apps/{name}/build-cache.
func (rt *Router) handleGetAppBuildCache(w http.ResponseWriter, r *http.Request) {
	id, siblings, ok := rt.appCacheScope(w, r)
	if !ok {
		return
	}
	if res, hit := appCacheUsageMemo.get(id); hit {
		writeJSON(w, http.StatusOK, res)
		return
	}
	u, err := rt.appBuildCache.AppCacheUsage(r.Context(), id, siblings)
	if errors.Is(err, build.ErrAppCacheNoBuilder) {
		writeError(w, http.StatusNotImplemented, "no BuildKit builder is available on this control plane")
		return
	}
	if err != nil {
		rt.internalError(w, "api: app build cache usage", err, slog.String("name", r.PathValue("name")))
		return
	}
	res := appBuildCacheResource{CacheID: id, SizeBytes: u.Bytes, RecordCount: u.Records, InUse: u.InUse}
	if !u.LastUsedAt.IsZero() {
		t := u.LastUsedAt
		res.LastUsedAt = &t
	}
	appCacheUsageMemo.put(id, res)
	writeJSON(w, http.StatusOK, res)
}

// handleClearAppBuildCache handles DELETE /api/v1/apps/{name}/build-cache.
func (rt *Router) handleClearAppBuildCache(w http.ResponseWriter, r *http.Request) {
	id, siblings, ok := rt.appCacheScope(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	running, err := rt.appBuildRunning(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: app build cache: list attempts", err, slog.String("name", name))
		return
	}
	if running {
		writeError(w, http.StatusConflict, "a build for this app is running; wait for it to finish, then clear the cache")
		return
	}
	res, err := rt.appBuildCache.PruneAppCache(r.Context(), id, siblings)
	switch {
	case errors.Is(err, build.ErrAppCacheNoBuilder):
		writeError(w, http.StatusNotImplemented, "no BuildKit builder is available on this control plane")
		return
	case errors.Is(err, build.ErrAppCacheInUse):
		writeError(w, http.StatusConflict, "the cache is in use by a running build; try again when it finishes")
		return
	case err != nil:
		rt.internalError(w, "api: app build cache clear", err, slog.String("name", name))
		return
	}
	appCacheUsageMemo.drop(id)
	rt.auditAppBuildCacheClear(r, name, id, res)
	writeJSON(w, http.StatusOK, appBuildCacheClearResponse{CacheID: id, FreedBytes: res.FreedBytes, RemovedCount: res.Removed})
}

func (rt *Router) auditAppBuildCacheClear(r *http.Request, app, cacheID string, res build.AppCachePruneResult) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: build cache audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: http.MethodDelete, Path: "/api/v1/apps/" + app + "/build-cache", StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionAppBuildCacheClear,
		Detail:     fmt.Sprintf(`{"cache_id":%q,"freed_bytes":%d,"removed":%d}`, cacheID, res.FreedBytes, res.Removed),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: build cache audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
