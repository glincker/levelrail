package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/nodecapacity"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/kit/ids"
)

const (
	errCodeNodeCapacity = "node_capacity_critical"
	// NodeGuardrailLocalID names the control plane's own node in guardrail routes and storage.
	NodeGuardrailLocalID  = "local"
	nodeGuardInterval     = time.Minute
	nodeCleanupHistory    = 50
	nodeCleanupListLimit  = 20
	nodeCleanupTriggerMan = "manual"
	nodeCleanupTriggerAut = "auto"
)

// NodeGuardrailStore persists per-node threshold overrides and cleanup history. *store.DB satisfies it.
type NodeGuardrailStore interface {
	GetNodeGuardrails(ctx context.Context, nodeID string) (store.NodeGuardrails, bool, error)
	SetNodeGuardrails(ctx context.Context, g store.NodeGuardrails, now time.Time) error
	ListAutoCleanNodes(ctx context.Context) ([]string, error)
	InsertNodeCleanupRun(ctx context.Context, r store.NodeCleanupRun, keep int) error
	ListNodeCleanupRuns(ctx context.Context, nodeID string, limit int) ([]store.NodeCleanupRun, error)
}

type nodeGuardState struct {
	store      NodeGuardrailStore
	defaults   nodecapacity.Thresholds
	notify     func(ctx context.Context, text string)
	mu         sync.Mutex
	lastLevels map[string]nodecapacity.Level
}

// SetNodeGuardrails enables the per-node capacity guardrails. notify may be nil.
func (rt *Router) SetNodeGuardrails(s NodeGuardrailStore, notify func(ctx context.Context, text string)) {
	rt.nodeGuard = &nodeGuardState{
		store:      s,
		defaults:   nodecapacity.ThresholdsFromEnv(os.LookupEnv),
		notify:     notify,
		lastLevels: map[string]nodecapacity.Level{},
	}
}

func (rt *Router) isLocalGuardNode(id string) bool {
	return id == "" || id == NodeGuardrailLocalID || (rt.localNodeID != "" && id == rt.localNodeID)
}

func (rt *Router) guardKey(id string) string {
	if rt.isLocalGuardNode(id) {
		return NodeGuardrailLocalID
	}
	return id
}

// nodeSample reads a node's host numbers: live for the control plane's node, the agent's last report otherwise.
func (rt *Router) nodeSample(id string) (nodecapacity.Sample, bool) {
	var s nodecapacity.Sample
	if rt.isLocalGuardNode(id) {
		if rt.dataDir == "" {
			return s, false
		}
		if total, used, err := telemetry.HostDiskBytes(rt.dataDir); err == nil && total > 0 {
			s.DiskTotal, s.DiskFree = uint64(total), uint64(max(total-used, 0))
		}
		if total, free, err := telemetry.HostInodes(rt.dataDir); err == nil {
			s.InodesTotal, s.InodesFree = total, free
		}
		if total, avail, err := telemetry.HostMemoryBytes(); err == nil && total > 0 {
			s.MemTotal, s.MemAvail = uint64(total), uint64(max(avail, 0))
		}
		if total, free, err := telemetry.HostSwapBytes(); err == nil && total > 0 {
			s.SwapTotal, s.SwapFree = uint64(total), uint64(max(free, 0))
		}
		return s, true
	}
	if rt.nodeHostReports == nil {
		return s, false
	}
	rep, ok := rt.nodeHostReports.Get(id)
	if !ok {
		return s, false
	}
	return nodecapacity.Sample{
		DiskTotal: rep.DiskTotalBytes, DiskFree: rep.DiskFreeBytes,
		InodesTotal: rep.DiskInodesTotal, InodesFree: rep.DiskInodesFree,
		MemTotal: rep.MemoryTotalBytes, MemAvail: rep.MemoryAvailableBytes,
		SwapTotal: rep.SwapTotalBytes, SwapFree: rep.SwapFreeBytes,
	}, true
}

func (rt *Router) guardThresholds(ctx context.Context, id string) (nodecapacity.Thresholds, store.NodeGuardrails) {
	g, ok, err := rt.nodeGuard.store.GetNodeGuardrails(ctx, rt.guardKey(id))
	if err != nil {
		rt.logger.Warn("api: node guardrails: load overrides failed", slog.String("error", err.Error()), slog.String("node_id", id))
		return rt.nodeGuard.defaults, store.NodeGuardrails{}
	}
	if !ok {
		return rt.nodeGuard.defaults, store.NodeGuardrails{}
	}
	return rt.nodeGuard.defaults.Merge(g), g
}

// nodeCapacityStatus evaluates one node; ok is false when no reading exists yet.
func (rt *Router) nodeCapacityStatus(ctx context.Context, id string) (nodecapacity.Status, bool) {
	if rt.nodeGuard == nil {
		return nodecapacity.Status{}, false
	}
	s, ok := rt.nodeSample(id)
	if !ok {
		return nodecapacity.Status{}, false
	}
	th, _ := rt.guardThresholds(ctx, id)
	return nodecapacity.Evaluate(s, th), true
}

// nodeBlockReason is non-empty when new builds and deploys on the node must be refused.
func (rt *Router) nodeBlockReason(ctx context.Context, id string) string {
	st, ok := rt.nodeCapacityStatus(ctx, id)
	if !ok || !st.Blocking() {
		return ""
	}
	return st.Reason
}

// refuseNodeCritical writes a 507 and reports true when the node is at its critical floor.
func (rt *Router) refuseNodeCritical(ctx context.Context, w http.ResponseWriter, id string) bool {
	reason := rt.nodeBlockReason(ctx, id)
	if reason == "" {
		return false
	}
	writeCodedError(w, http.StatusInsufficientStorage, errCodeNodeCapacity,
		"this node is out of room ("+reason+"): the running version keeps serving, free space on the node and try again")
	return true
}

type nodeCapacityResponse struct {
	NodeID           string                  `json:"node_id"`
	Local            bool                    `json:"local"`
	HasReading       bool                    `json:"has_reading"`
	Status           nodecapacity.Status     `json:"status"`
	Settings         nodeGuardrailSettings   `json:"settings"`
	CleanupSupported bool                    `json:"cleanup_supported"`
	CleanupReason    string                  `json:"cleanup_unsupported_reason,omitempty"`
	Defaults         nodecapacity.Thresholds `json:"defaults"`
}

type nodeGuardrailSettings struct {
	DiskWarnFreePct  *float64 `json:"disk_warn_free_pct"`
	DiskCritFreePct  *float64 `json:"disk_crit_free_pct"`
	InodeWarnFreePct *float64 `json:"inode_warn_free_pct"`
	InodeCritFreePct *float64 `json:"inode_crit_free_pct"`
	MemWarnAvailPct  *float64 `json:"mem_warn_avail_pct"`
	MemCritAvailPct  *float64 `json:"mem_crit_avail_pct"`
	SwapWarnUsedPct  *float64 `json:"swap_warn_used_pct"`
	SwapCritUsedPct  *float64 `json:"swap_crit_used_pct"`
	AutoClean        bool     `json:"auto_clean"`
}

func settingsOf(g store.NodeGuardrails) nodeGuardrailSettings {
	return nodeGuardrailSettings{g.DiskWarnFreePct, g.DiskCritFreePct, g.InodeWarnFreePct, g.InodeCritFreePct,
		g.MemWarnAvailPct, g.MemCritAvailPct, g.SwapWarnUsedPct, g.SwapCritUsedPct, g.AutoClean}
}

const cleanupRemoteReason = "cleanup is not supported on this node yet, free space on the node itself"

func (rt *Router) cleanupReady() bool { return rt.janitor != nil && rt.janitorStore != nil }

func (rt *Router) guardNodeExists(w http.ResponseWriter, r *http.Request) (string, bool) {
	if rt.nodeGuard == nil {
		writeError(w, http.StatusNotImplemented, "node capacity guardrails are not configured on this control plane")
		return "", false
	}
	id := r.PathValue("id")
	if rt.isLocalGuardNode(id) {
		return rt.guardKey(id), true
	}
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: node guardrails: load node failed", err, slog.String("node_id", id))
		return "", false
	}
	return id, true
}

// handleGetNodeCapacity handles GET /api/v1/nodes/{id}/capacity.
func (rt *Router) handleGetNodeCapacity(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.guardNodeExists(w, r)
	if !ok {
		return
	}
	th, g := rt.guardThresholds(r.Context(), id)
	resp := nodeCapacityResponse{NodeID: id, Local: rt.isLocalGuardNode(id), Settings: settingsOf(g), Defaults: rt.nodeGuard.defaults}
	if s, have := rt.nodeSample(id); have {
		resp.HasReading, resp.Status = true, nodecapacity.Evaluate(s, th)
	} else {
		resp.Status = nodecapacity.Status{Metrics: []nodecapacity.MetricStatus{}, Thresholds: th, Worst: nodecapacity.LevelUnknown}
	}
	resp.CleanupSupported = resp.Local && rt.cleanupReady()
	if !resp.Local {
		resp.CleanupReason = cleanupRemoteReason
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePutNodeGuardrails handles PUT /api/v1/nodes/{id}/guardrails.
func (rt *Router) handlePutNodeGuardrails(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.guardNodeExists(w, r)
	if !ok {
		return
	}
	var req nodeGuardrailSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	g := store.NodeGuardrails{NodeID: id, DiskWarnFreePct: req.DiskWarnFreePct, DiskCritFreePct: req.DiskCritFreePct,
		InodeWarnFreePct: req.InodeWarnFreePct, InodeCritFreePct: req.InodeCritFreePct, MemWarnAvailPct: req.MemWarnAvailPct,
		MemCritAvailPct: req.MemCritAvailPct, SwapWarnUsedPct: req.SwapWarnUsedPct, SwapCritUsedPct: req.SwapCritUsedPct,
		AutoClean: req.AutoClean, UpdatedBy: rt.actorForRequest(r)}
	merged := rt.nodeGuard.defaults.Merge(g)
	if !merged.Validate() || !validOverrides(g) {
		writeError(w, http.StatusBadRequest, "percentages must be between 0 and 100, and each critical value must be stricter than its warn value")
		return
	}
	if err := rt.nodeGuard.store.SetNodeGuardrails(r.Context(), g, time.Now()); err != nil {
		rt.internalError(w, "api: node guardrails: save failed", err, slog.String("node_id", id))
		return
	}
	rt.handleGetNodeCapacity(w, r)
}

func validOverrides(g store.NodeGuardrails) bool {
	for _, p := range []*float64{g.DiskWarnFreePct, g.DiskCritFreePct, g.InodeWarnFreePct, g.InodeCritFreePct,
		g.MemWarnAvailPct, g.MemCritAvailPct, g.SwapWarnUsedPct, g.SwapCritUsedPct} {
		if p != nil && (*p < 0 || *p > 100) {
			return false
		}
	}
	return true
}

type nodeCleanupRequest struct {
	DryRun  bool `json:"dry_run"`
	Confirm bool `json:"confirm"`
}

type nodeCleanupResponse struct {
	DryRun  bool                 `json:"dry_run"`
	Preview nodecapacity.Preview `json:"preview"`
	Freed   int64                `json:"freed_bytes"`
	Error   string               `json:"error,omitempty"`
}

// handleNodeCleanup handles POST /api/v1/nodes/{id}/cleanup: a dry-run preview, or a confirmed removal of the safe classes.
func (rt *Router) handleNodeCleanup(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.guardNodeExists(w, r)
	if !ok {
		return
	}
	if !rt.isLocalGuardNode(id) {
		writeCodedError(w, http.StatusNotImplemented, "cleanup_unsupported", cleanupRemoteReason)
		return
	}
	if !rt.cleanupReady() {
		writeError(w, http.StatusNotImplemented, "cleanup is not configured on this control plane")
		return
	}
	var req nodeCleanupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DryRun {
		p, err := nodecapacity.BuildPreview(r.Context(), rt.janitor, rt.janitor.Docker)
		if err != nil {
			rt.internalError(w, "api: node cleanup preview failed", err, slog.String("node_id", id))
			return
		}
		writeJSON(w, http.StatusOK, nodeCleanupResponse{DryRun: true, Preview: p})
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirm is required: preview with dry_run first, then repeat with confirm true")
		return
	}
	res, err := rt.runNodeCleanup(r.Context(), id, nodeCleanupTriggerMan, rt.actorForRequest(r))
	if errors.Is(err, janitor.ErrBusy) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: node cleanup failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, nodeCleanupResponse{Preview: res.Preview, Freed: res.FreedBytes, Error: res.Error})
}

// runNodeCleanup removes the safe classes and records the outcome in the node's history.
func (rt *Router) runNodeCleanup(ctx context.Context, nodeID, trigger, actor string) (nodecapacity.Result, error) {
	started := time.Now()
	res, err := nodecapacity.Execute(ctx, rt.janitor, janitor.TriggerNodeGuardrail)
	if err != nil {
		return res, fmt.Errorf("node %s: %w", nodeID, err)
	}
	body, merr := json.Marshal(res.Classes)
	if merr != nil {
		body = []byte("[]")
	}
	run := store.NodeCleanupRun{ID: ids.MustHex(8), NodeID: nodeID, StartedAt: started, Trigger: trigger, Actor: actor,
		ReclaimedBytes: res.FreedBytes, Body: string(body), Error: res.Error}
	if ierr := rt.nodeGuard.store.InsertNodeCleanupRun(ctx, run, nodeCleanupHistory); ierr != nil {
		rt.logger.Error("api: node cleanup: record run failed", slog.String("error", ierr.Error()), slog.String("node_id", nodeID))
	}
	rt.logger.Info("api: node cleanup finished", slog.String("node_id", nodeID), slog.String("trigger", trigger),
		slog.String("actor", actor), slog.Int64("freed_bytes", res.FreedBytes))
	return res, nil
}

type nodeCleanupRunResource struct {
	ID         string          `json:"id"`
	StartedAt  time.Time       `json:"started_at"`
	Trigger    string          `json:"trigger"`
	Actor      string          `json:"actor"`
	FreedBytes int64           `json:"freed_bytes"`
	Classes    json.RawMessage `json:"classes"`
	Error      string          `json:"error,omitempty"`
}

// handleListNodeCleanupRuns handles GET /api/v1/nodes/{id}/cleanup-runs.
func (rt *Router) handleListNodeCleanupRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.guardNodeExists(w, r)
	if !ok {
		return
	}
	runs, err := rt.nodeGuard.store.ListNodeCleanupRuns(r.Context(), id, nodeCleanupListLimit)
	if err != nil {
		rt.internalError(w, "api: list node cleanup runs failed", err, slog.String("node_id", id))
		return
	}
	out := make([]nodeCleanupRunResource, 0, len(runs))
	for _, run := range runs {
		out = append(out, nodeCleanupRunResource{ID: run.ID, StartedAt: run.StartedAt, Trigger: run.Trigger, Actor: run.Actor,
			FreedBytes: run.ReclaimedBytes, Classes: json.RawMessage(run.Body), Error: run.Error})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) guardNodeIDs(ctx context.Context) []string {
	out := []string{NodeGuardrailLocalID}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.logger.Warn("api: node guardrails: list nodes failed", slog.String("error", err.Error()))
		return out
	}
	for _, n := range nodes {
		if n.ID != "" && !rt.isLocalGuardNode(n.ID) {
			out = append(out, n.ID)
		}
	}
	return out
}

func (rt *Router) nodeDisplayName(ctx context.Context, id string) string {
	if rt.isLocalGuardNode(id) {
		return "control plane"
	}
	if n, err := rt.nodes.GetNode(ctx, id); err == nil && n != nil && n.Name != "" {
		return n.Name
	}
	return id
}

// nodeCapacityAttentionItems lists nodes at warn or critical.
func (rt *Router) nodeCapacityAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.nodeGuard == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	var items []attention.Item
	for _, id := range rt.guardNodeIDs(r.Context()) {
		st, ok := rt.nodeCapacityStatus(r.Context(), id)
		if !ok || (st.Worst != nodecapacity.LevelWarn && st.Worst != nodecapacity.LevelCritical) {
			continue
		}
		sev := attention.Warning
		if st.Blocking() {
			sev = attention.Critical
		}
		name := rt.nodeDisplayName(r.Context(), id)
		detail := st.Reason
		if st.Blocking() {
			detail += ": new builds and deploys are blocked, the running version keeps serving"
		}
		items = append(items, feedItem(sev, attention.KindNodeCapacity, name, detail, map[string]string{"node_id": id, "level": string(st.Worst)}))
	}
	return items
}

// RunNodeGuardrailWatcher notifies on level changes and runs opted-in auto-clean until ctx ends.
func (rt *Router) RunNodeGuardrailWatcher(ctx context.Context) {
	if rt.nodeGuard == nil {
		return
	}
	t := time.NewTicker(nodeGuardInterval)
	defer t.Stop()
	for {
		rt.checkNodeGuardrails(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (rt *Router) checkNodeGuardrails(ctx context.Context) {
	auto := map[string]bool{}
	if list, err := rt.nodeGuard.store.ListAutoCleanNodes(ctx); err == nil {
		for _, id := range list {
			auto[id] = true
		}
	}
	for _, id := range rt.guardNodeIDs(ctx) {
		st, ok := rt.nodeCapacityStatus(ctx, id)
		if !ok {
			continue
		}
		rt.nodeGuard.mu.Lock()
		prev := rt.nodeGuard.lastLevels[id]
		rt.nodeGuard.lastLevels[id] = st.Worst
		rt.nodeGuard.mu.Unlock()
		if rt.nodeGuard.notify != nil && st.Worst != prev && (st.Worst == nodecapacity.LevelWarn || st.Worst == nodecapacity.LevelCritical) {
			rt.nodeGuard.notify(ctx, fmt.Sprintf("Node %s: %s (%s)", rt.nodeDisplayName(ctx, id), st.Reason, st.Worst))
		}
		if auto[id] && rt.isLocalGuardNode(id) && rt.cleanupReady() && st.Worst != nodecapacity.LevelOK && st.Worst != nodecapacity.LevelUnknown {
			if _, err := rt.runNodeCleanup(ctx, id, nodeCleanupTriggerAut, "auto-clean"); err != nil && !errors.Is(err, janitor.ErrBusy) {
				rt.logger.Warn("api: node auto-clean failed", slog.String("error", err.Error()), slog.String("node_id", id))
			}
		}
	}
}
