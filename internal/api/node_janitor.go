package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/footprint"
	"github.com/glincker/levelrail/internal/nodecapacity"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/kit/ids"
)

const (
	nodeJanitorTick       = time.Minute
	nodeCleanupTriggerJan = "janitor"
	nodeCleanupTriggerDry = "janitor-dry"
	bytesPerGB            = int64(1) << 30
)

// nodeJanitorState remembers the last pass of each remote node for the schedule and the doctor row.
type nodeJanitorState struct {
	mu    sync.Mutex
	last  map[string]time.Time
	plans map[string]nodecapacity.BudgetPlan
}

func newNodeJanitorState() *nodeJanitorState {
	return &nodeJanitorState{last: map[string]time.Time{}, plans: map[string]nodecapacity.BudgetPlan{}}
}

// nodeBudgetBytes is the node's unused-image budget: its own override, else the default.
func (rt *Router) nodeBudgetBytes(ctx context.Context, nodeID string) int64 {
	s := rt.imageSettings()
	gb := s.Int(ctx, runtimesettings.KeyJanitorNodeBudgetGB)
	if m, err := runtimesettings.ParseNodeBudgetOverrides(s.String(ctx, runtimesettings.KeyJanitorNodeBudgetByNode)); err == nil {
		if v, ok := m[nodeID]; ok {
			gb = v
		}
	}
	return int64(gb) * bytesPerGB
}

// SetNodeBudgetOverride saves one node's budget in GB; 0 removes the override.
func (rt *Router) SetNodeBudgetOverride(ctx context.Context, nodeID string, gb int, actor string) error {
	s := rt.imageSettings()
	m, err := runtimesettings.ParseNodeBudgetOverrides(s.String(ctx, runtimesettings.KeyJanitorNodeBudgetByNode))
	if err != nil {
		m = map[string]int{}
	}
	if gb == 0 {
		delete(m, nodeID)
	} else {
		m[nodeID] = gb
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode node budgets: %w", err)
	}
	val, err := json.Marshal(string(raw))
	if err != nil {
		return fmt.Errorf("encode node budgets: %w", err)
	}
	if _, err := s.Set(ctx, runtimesettings.KeyJanitorNodeBudgetByNode, val, actor); err != nil {
		return fmt.Errorf("save node budgets: %w", err)
	}
	return nil
}

// previewNodeBudget asks the agent for a dry-run preview only and plans against the budget.
// It never sends a removing request.
func previewNodeBudget(ctx context.Context, c nodeCleaner, keep []string, budgetBytes int64) (nodecapacity.Preview, nodecapacity.BudgetPlan, error) {
	res, err := c.NodeCleanup(ctx, true, keep)
	if err != nil {
		return nodecapacity.Preview{}, nodecapacity.BudgetPlan{}, fmt.Errorf("preview node cleanup: %w", err)
	}
	return res.Preview, nodecapacity.PlanBudget(res.Preview, budgetBytes), nil
}

// remoteJanitorPass previews a node, records what would be freed as a history event, and removes only
// when janitor.remote_execute is on, the node opted into auto-clean and it is over budget.
func (rt *Router) remoteJanitorPass(ctx context.Context, id string, autoClean bool) {
	c := rt.remoteCleaner(id)
	if c == nil || !rt.remoteCleanupCapable(id) {
		return
	}
	keep, err := rt.keepContainerNames(ctx)
	if err != nil {
		rt.logger.Warn("api: node janitor: desired containers unreadable", slog.String("error", err.Error()), slog.String("node_id", id))
		return
	}
	started := time.Now()
	prev, plan, err := previewNodeBudget(ctx, c, keep, rt.nodeBudgetBytes(ctx, id))
	if err != nil {
		rt.logger.Warn("api: node janitor: preview failed", slog.String("error", err.Error()), slog.String("node_id", id))
		return
	}
	rt.nodeGuard.janitor.mu.Lock()
	rt.nodeGuard.janitor.plans[id] = plan
	rt.nodeGuard.janitor.mu.Unlock()
	if !plan.Over() {
		return
	}
	if rt.imageSettings().Bool(ctx, runtimesettings.KeyJanitorRemoteExecute) && autoClean {
		if _, err := rt.runNodeCleanup(ctx, id, nodeCleanupTriggerJan, "janitor"); err != nil {
			rt.logger.Warn("api: node janitor: cleanup failed", slog.String("error", err.Error()), slog.String("node_id", id))
		}
		return
	}
	body, merr := json.Marshal(prev.Classes)
	if merr != nil {
		body = []byte("[]")
	}
	// A dry run frees nothing: ReclaimedBytes stays 0 and the plan sits in the body.
	run := store.NodeCleanupRun{ID: ids.MustHex(8), NodeID: id, StartedAt: started, Trigger: nodeCleanupTriggerDry, Actor: "janitor",
		Body: string(body), Error: fmt.Sprintf("dry run: %d bytes over budget, would free %d bytes", plan.OverBytes, plan.WouldFreeBytes)}
	if ierr := rt.nodeGuard.store.InsertNodeCleanupRun(ctx, run, nodeCleanupHistory); ierr != nil {
		rt.logger.Error("api: node janitor: record dry run failed", slog.String("error", ierr.Error()), slog.String("node_id", id))
	}
}

// RunRemoteNodeJanitor runs the per-node schedule until ctx ends. The interval is re-read each tick.
func (rt *Router) RunRemoteNodeJanitor(ctx context.Context) {
	if rt.nodeGuard == nil {
		return
	}
	_ = footprint.Every(ctx, "node janitor", nodeJanitorTick, rt.logger, func(ctx context.Context) error {
		rt.nodeJanitorTick(ctx, time.Now())
		return nil
	})
}

func (rt *Router) nodeJanitorTick(ctx context.Context, now time.Time) {
	interval := time.Duration(rt.imageSettings().Int(ctx, runtimesettings.KeyJanitorRemoteInterval)) * time.Minute
	auto := map[string]bool{}
	if list, err := rt.nodeGuard.store.ListAutoCleanNodes(ctx); err == nil {
		for _, id := range list {
			auto[id] = true
		}
	}
	st := rt.nodeGuard.janitor
	for _, id := range rt.guardNodeIDs(ctx) {
		if rt.isLocalGuardNode(id) {
			continue
		}
		st.mu.Lock()
		due := now.Sub(st.last[id]) >= interval
		if due {
			st.last[id] = now
		}
		st.mu.Unlock()
		if due {
			rt.remoteJanitorPass(ctx, id, auto[id])
		}
	}
}

// doctorCheckNodeDiskBudget warns when a remote node's last pass found unused images over its budget.
func (rt *Router) doctorCheckNodeDiskBudget(ctx context.Context) doctorCheckResource {
	const code, name = "node_disk_budget", "Node disk over budget"
	if rt.nodeGuard == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Node cleanup is not configured on this control plane."}
	}
	st := rt.nodeGuard.janitor
	st.mu.Lock()
	defer st.mu.Unlock()
	var over []string
	for id, p := range st.plans {
		if p.Over() {
			over = append(over, fmt.Sprintf("%s: %d MiB of unused images over its %d GB budget", rt.nodeDisplayName(ctx, id), p.OverBytes>>20, p.BudgetBytes/bytesPerGB))
		}
	}
	if len(over) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "No remote node holds unused images over its disk budget."}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprint(over),
		Fix: "Run levelrail-cli janitor run --node <id> to preview, then --yes to clean, or raise janitor.node_images_budget_gb."}
}
