package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	buildCacheRecordsShown  = 100
	defaultCachePruneAgeHrs = 168
)

// BuildCacheResolver returns the build cache admin of one node, "" being the control plane's own.
type BuildCacheResolver func(nodeID string) (docker.BuildCacheAdmin, error)

// WithBuildCache enables the /api/v1/builds/cache routes.
func WithBuildCache(r BuildCacheResolver) Option {
	return func(rt *Router) { rt.buildCacheNodes = r }
}

type buildCacheRecordResource struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	SizeBytes   int64      `json:"size_bytes"`
	InUse       bool       `json:"in_use"`
	UsageCount  int        `json:"usage_count"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type buildCacheNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type buildCacheResponse struct {
	Node             string                     `json:"node"`
	Supported        bool                       `json:"supported"`
	Reason           string                     `json:"reason,omitempty"`
	TotalBytes       int64                      `json:"total_bytes"`
	InUseBytes       int64                      `json:"in_use_bytes"`
	ReclaimableBytes int64                      `json:"reclaimable_bytes"`
	RecordCount      int                        `json:"record_count"`
	Records          []buildCacheRecordResource `json:"records"`
	Nodes            []buildCacheNode           `json:"nodes"`
}

type buildCachePruneRequest struct {
	Node           string `json:"node"`
	OlderThanHours *int   `json:"older_than_hours,omitempty"`
	Apply          bool   `json:"apply"`
}

type buildCachePruneResponse struct {
	Node           string                     `json:"node"`
	Applied        bool                       `json:"applied"`
	OlderThanHours int                        `json:"older_than_hours"`
	Candidates     []buildCacheRecordResource `json:"candidates"`
	CandidateCount int                        `json:"candidate_count"`
	WouldFreeBytes int64                      `json:"would_free_bytes"`
	FreedBytes     int64                      `json:"freed_bytes"`
	RemovedCount   int                        `json:"removed_count"`
}

func toCacheRecordResource(r docker.BuildCacheRecord) buildCacheRecordResource {
	out := buildCacheRecordResource{ID: r.ID, Type: r.Type, Description: r.Description, SizeBytes: r.SizeBytes, InUse: r.InUse, UsageCount: r.UsageCount}
	if !r.LastUsedAt.IsZero() {
		t := r.LastUsedAt
		out.LastUsedAt = &t
	}
	return out
}

func (rt *Router) buildCacheFor(w http.ResponseWriter, nodeID string) (docker.BuildCacheAdmin, bool) {
	if rt.buildCacheNodes == nil {
		writeError(w, http.StatusNotImplemented, "build cache management is not configured on this control plane")
		return nil, false
	}
	admin, err := rt.buildCacheNodes(nodeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "this node cannot be reached: "+err.Error())
		return nil, false
	}
	return admin, true
}

func (rt *Router) buildCacheNodeList(r *http.Request) []buildCacheNode {
	out := []buildCacheNode{{ID: "", Name: "local"}}
	if rt.nodes == nil {
		return out
	}
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		return out
	}
	for _, n := range nodes {
		out = append(out, buildCacheNode{ID: n.ID, Name: n.Name})
	}
	return out
}

// handleBuildCache handles GET /api/v1/builds/cache?node=: BuildKit cache usage of one node.
func (rt *Router) handleBuildCache(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node")
	resp := buildCacheResponse{Node: nodeID, Records: []buildCacheRecordResource{}, Nodes: rt.buildCacheNodeList(r)}
	admin, ok := rt.buildCacheFor(w, nodeID)
	if !ok {
		return
	}
	records, err := admin.BuildCacheRecords(r.Context())
	if err != nil {
		resp.Reason = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.Supported = true
	resp.RecordCount = len(records)
	for i, rec := range records {
		resp.TotalBytes += rec.SizeBytes
		if rec.InUse {
			resp.InUseBytes += rec.SizeBytes
		} else {
			resp.ReclaimableBytes += rec.SizeBytes
		}
		if i < buildCacheRecordsShown {
			resp.Records = append(resp.Records, toCacheRecordResource(rec))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleBuildCachePrune handles POST /api/v1/builds/cache/prune. Without apply it only previews.
func (rt *Router) handleBuildCachePrune(w http.ResponseWriter, r *http.Request) {
	var req buildCachePruneRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hours := defaultCachePruneAgeHrs
	if req.OlderThanHours != nil {
		hours = *req.OlderThanHours
	}
	if hours < 0 || hours > 24*365 {
		writeError(w, http.StatusBadRequest, "older_than_hours must be between 0 and 8760")
		return
	}
	admin, ok := rt.buildCacheFor(w, req.Node)
	if !ok {
		return
	}
	olderThan := time.Duration(hours) * time.Hour
	records, err := admin.BuildCacheRecords(r.Context())
	if err != nil {
		rt.internalError(w, "api: build cache prune: list records failed", err)
		return
	}
	plan := docker.PlanBuildCachePrune(records, olderThan, time.Now())
	resp := buildCachePruneResponse{Node: req.Node, OlderThanHours: hours, Candidates: []buildCacheRecordResource{}, CandidateCount: len(plan)}
	for i, rec := range plan {
		resp.WouldFreeBytes += rec.SizeBytes
		if i < buildCacheRecordsShown {
			resp.Candidates = append(resp.Candidates, toCacheRecordResource(rec))
		}
	}
	if req.Apply {
		res, err := admin.PruneBuildCacheOlderThan(r.Context(), olderThan)
		if err != nil {
			rt.internalError(w, "api: build cache prune failed", err)
			return
		}
		resp.Applied = true
		resp.FreedBytes = int64(res.ReclaimedBytes) //nolint:gosec // reclaimed bytes fit int64
		resp.RemovedCount = len(res.Removed)
	}
	writeJSON(w, http.StatusOK, resp)
}

// AuditActionAppBuildCacheReclaim is the audit event for a quota reclaim of one app's cache.
const AuditActionAppBuildCacheReclaim = "app.build_cache.reclaim"

const (
	cacheReclaimTriggerAuto   = "auto"
	cacheReclaimTriggerManual = "manual"
	cacheReclaimSkipRunning   = "build_running"
	cacheReclaimSkipBusy      = "reclaim_running"
	// cacheReclaimHistory caps the in memory reclaim report.
	cacheReclaimHistory = 200
	cacheBytesPerGiB    = int64(1) << 30
)

// cacheScanGate lets one builder disk usage scan run at a time, so many builds
// finishing together do not fire one scan each.
var cacheScanGate = make(chan struct{}, 1)

// AppCacheQuotaReclaimer is the optional surface for per app quota reclaims.
// *build.Client satisfies it.
type AppCacheQuotaReclaimer interface {
	ReclaimAppCacheQuota(ctx context.Context, cacheID string, siblings []string, quotaBytes int64, maxRecords int) (build.AppCacheReclaim, error)
}

type cacheReclaimEntry struct {
	App          string    `json:"app"`
	Trigger      string    `json:"trigger"`
	At           time.Time `json:"at"`
	UsageBytes   int64     `json:"usage_bytes"`
	QuotaBytes   int64     `json:"quota_bytes"`
	FreedBytes   int64     `json:"freed_bytes"`
	RemovedCount int       `json:"removed_count"`
	SkippedInUse int       `json:"skipped_in_use"`
	Bounded      bool      `json:"bounded"`
	Skipped      string    `json:"skipped,omitempty"`
	Partial      bool      `json:"partial,omitempty"`
}

type buildCacheQuotaResponse struct {
	AppQuotaGB        int                 `json:"app_quota_gb"`
	ReclaimMaxRecords int                 `json:"reclaim_max_records"`
	Supported         bool                `json:"supported"`
	Reclaims          []cacheReclaimEntry `json:"reclaims"`
}

type cacheReclaimLog struct {
	mu      sync.Mutex
	entries []cacheReclaimEntry
	running map[string]bool
}

var appCacheReclaims = &cacheReclaimLog{running: map[string]bool{}}

func (l *cacheReclaimLog) add(e cacheReclaimEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	if len(l.entries) > cacheReclaimHistory {
		l.entries = l.entries[len(l.entries)-cacheReclaimHistory:]
	}
}

// claim keeps two reclaims of one app from racing each other.
func (l *cacheReclaimLog) claim(app string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running[app] {
		return false
	}
	l.running[app] = true
	return true
}

func (l *cacheReclaimLog) release(app string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.running, app)
}

// newestFirst returns the entries visible to keep, newest first.
func (l *cacheReclaimLog) newestFirst(keep func(string) bool) []cacheReclaimEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []cacheReclaimEntry{}
	for i := len(l.entries) - 1; i >= 0; i-- {
		if keep(l.entries[i].App) {
			out = append(out, l.entries[i])
		}
	}
	return out
}

func (rt *Router) cacheQuotaReclaimer() (AppCacheQuotaReclaimer, bool) {
	if rt.appBuildCache == nil {
		return nil, false
	}
	q, ok := rt.appBuildCache.(AppCacheQuotaReclaimer)
	return q, ok
}

func (rt *Router) appCacheQuotaBytes(ctx context.Context) int64 {
	return int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildCacheAppQuotaGB)) * cacheBytesPerGiB
}

// appBuildActive reports a running or queued build of the app, so a reclaim never races the next build.
func (rt *Router) appBuildActive(ctx context.Context, name string) (bool, error) {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return false, err
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusRunning || a.Status == store.DeployAttemptStatusQueued {
			return true, nil
		}
	}
	return false, nil
}

// reclaimAppCache trims one app's cache to its quota. It never runs while a
// build of that app is running or queued; in use records are skipped by the planner.
func (rt *Router) reclaimAppCache(ctx context.Context, app, trigger string) (cacheReclaimEntry, error) {
	entry := cacheReclaimEntry{App: app, Trigger: trigger, At: time.Now().UTC(), QuotaBytes: rt.appCacheQuotaBytes(ctx)}
	q, ok := rt.cacheQuotaReclaimer()
	id := build.SanitizeCacheID(app, "")
	if !ok || entry.QuotaBytes <= 0 || id == "" {
		return entry, nil
	}
	if !appCacheReclaims.claim(app) {
		entry.Skipped = cacheReclaimSkipBusy
		return entry, nil
	}
	defer appCacheReclaims.release(app)
	active, err := rt.appBuildActive(ctx, app)
	if err != nil {
		return entry, err
	}
	if active {
		entry.Skipped = cacheReclaimSkipRunning
		return entry, nil
	}
	select {
	case cacheScanGate <- struct{}{}:
	case <-ctx.Done():
		return entry, ctx.Err()
	}
	defer func() { <-cacheScanGate }()
	maxRecords := rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildCacheReclaimMaxRecords)
	res, err := q.ReclaimAppCacheQuota(ctx, id, rt.siblingCacheIDs(ctx, id), entry.QuotaBytes, maxRecords)
	entry.UsageBytes, entry.FreedBytes, entry.RemovedCount = res.UsageBytes, res.FreedBytes, res.Removed
	entry.SkippedInUse, entry.Bounded, entry.Partial = res.SkippedInUse, res.Bounded, err != nil
	if res.Removed > 0 {
		appCacheUsageMemo.drop(id)
	}
	return entry, err
}

// runCacheReclaim is the one path for automatic and manual reclaims: bounded by
// the live timeout, and anything removed is reported and audited even on error.
func (rt *Router) runCacheReclaim(base context.Context, r *http.Request, app, trigger string) (cacheReclaimEntry, error) {
	timeout := runtimesettings.ClampBuildCacheReclaimTimeout(rt.runtimeSettings.Duration(base, runtimesettings.KeyBuildCacheReclaimTimeout))
	ctx, cancel := context.WithTimeout(base, timeout)
	defer cancel()
	entry, err := rt.reclaimAppCache(ctx, app, trigger)
	if entry.RemovedCount > 0 || entry.SkippedInUse > 0 || entry.Skipped == cacheReclaimSkipRunning {
		appCacheReclaims.add(entry)
	}
	if entry.RemovedCount > 0 {
		rt.auditCacheReclaim(base, r, entry)
	}
	return entry, err
}

// AfterBuildFinish is the post build hook shared by the API and push webhook
// build paths; it trims the app's cache to its quota and only logs failures.
func (rt *Router) AfterBuildFinish(app string) {
	ctx := context.Background()
	if rt.appCacheQuotaBytes(ctx) <= 0 {
		return
	}
	if _, err := rt.runCacheReclaim(ctx, nil, app, cacheReclaimTriggerAuto); err != nil {
		rt.logger.Warn("api: build cache quota reclaim failed", slog.String("app", app), slog.String("error", err.Error()))
	}
}

func (rt *Router) auditCacheReclaim(ctx context.Context, r *http.Request, e cacheReclaimEntry) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: build cache reclaim audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "build-cache-quota", ActorName: "build-cache-quota",
		Ability: AbilityDeploy, Method: auditMethodEvent, Path: "/api/v1/apps/" + e.App + "/build-cache/reclaim", StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), Action: AuditActionAppBuildCacheReclaim,
		Detail: fmt.Sprintf(`{"app":%q,"freed_bytes":%d}`, e.App, e.FreedBytes),
	}
	if r != nil {
		a := rt.accessActor(r)
		entry.ActorType, entry.ActorID, entry.ActorName = a.kind, a.id, a.name
		entry.Method, entry.RemoteAddr = http.MethodPost, clientIP(r)
		entry.ClientKind = clientKindFromUserAgent(r.Header.Get("User-Agent"))
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: build cache reclaim audit save failed", slog.String("error", err.Error()), slog.String("app", e.App))
	}
}

// handleBuildCacheQuota handles GET /api/v1/builds/cache/quota: the live quota and the reclaim report.
func (rt *Router) handleBuildCacheQuota(w http.ResponseWriter, r *http.Request) {
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: build cache quota: resolve caller failed", err)
		return
	}
	_, supported := rt.cacheQuotaReclaimer()
	ctx := r.Context()
	writeJSON(w, http.StatusOK, buildCacheQuotaResponse{
		AppQuotaGB:        rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildCacheAppQuotaGB),
		ReclaimMaxRecords: rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildCacheReclaimMaxRecords),
		Supported:         supported,
		Reclaims:          appCacheReclaims.newestFirst(canRead),
	})
}

// handleReclaimAppBuildCache handles POST /api/v1/apps/{name}/build-cache/reclaim.
func (rt *Router) handleReclaimAppBuildCache(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := rt.appCacheScope(w, r, false); !ok {
		return
	}
	if _, ok := rt.cacheQuotaReclaimer(); !ok {
		writeError(w, http.StatusNotImplemented, "no BuildKit builder is available on this control plane")
		return
	}
	if rt.appCacheQuotaBytes(r.Context()) <= 0 {
		writeError(w, http.StatusConflict, "the per app cache quota is off; set build.cache_app_quota_gb first")
		return
	}
	name := r.PathValue("name")
	// Detached from the request so a client disconnect cannot cut a prune short.
	entry, err := rt.runCacheReclaim(context.WithoutCancel(r.Context()), r, name, cacheReclaimTriggerManual)
	switch {
	case errors.Is(err, build.ErrAppCacheNoBuilder):
		writeError(w, http.StatusNotImplemented, "no BuildKit builder is available on this control plane")
	case err != nil:
		rt.internalError(w, "api: app build cache reclaim", err, slog.String("name", name))
	case entry.Skipped != "":
		writeError(w, http.StatusConflict, "a build or reclaim for this app is running or queued; the cache is reclaimed after it finishes")
	default:
		writeJSON(w, http.StatusOK, entry)
	}
}
