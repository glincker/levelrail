package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/nodescale"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

const nodeScaleInTick = time.Minute

type scaleInSettings struct{ rt *Router }

func (s scaleInSettings) Enabled(ctx context.Context) bool {
	return s.rt.imageSettings().Bool(ctx, runtimesettings.KeyNodesScaleIn)
}

func (s scaleInSettings) IdleMinutes(ctx context.Context) int {
	return s.rt.imageSettings().Int(ctx, runtimesettings.KeyNodesScaleInIdleMinutes)
}

type volumeMoveNodes interface {
	NodesWithRunningVolumeMoves(ctx context.Context) (map[string]bool, error)
}

type scaleInSource struct{ rt *Router }

// Nodes joins nodes with provisions, labels, pools, workload counts and prices.
func (s scaleInSource) Nodes(ctx context.Context) ([]nodescale.Node, error) {
	rt := s.rt
	list, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	provisioned := map[string]bool{}
	if rt.nodeProvisions != nil {
		if rows, err := rt.nodeProvisions.ListNodeProvisions(ctx); err == nil {
			for _, p := range rows {
				if p.NodeID != "" && p.Status != "failed" {
					provisioned[p.NodeID] = true
				}
			}
		}
	}
	var labels map[string]map[string]string
	var pools []store.NodePool
	if rt.multiNode != nil {
		labels, _ = rt.multiNode.store.ListNodeLabels(ctx)
		if ps, ok := rt.multiNode.store.(poolStore); ok {
			pools, _ = ps.ListNodePools(ctx)
		}
	}
	var counts map[string]int
	if cs, ok := rt.nodes.(nodeResourceCountStore); ok {
		counts, _ = cs.NodeResourceCounts(ctx)
	}
	var busy map[string]bool
	if vm, ok := rt.nodes.(volumeMoveNodes); ok {
		busy, _ = vm.NodesWithRunningVolumeMoves(ctx)
	}
	prices := rt.deployTargetPrices(ctx)
	out := make([]nodescale.Node, 0, len(list))
	for _, n := range list {
		node := nodescale.Node{
			ID: n.ID, Name: n.Name, Labels: labels[n.ID], Provisioned: provisioned[n.ID],
			ControlPlane: n.ID == rt.localNodeID, Workloads: counts[n.ID], HoldsVolumes: busy[n.ID],
		}
		for _, p := range pools {
			if scaleInPoolMatches(p.Selector, labels[n.ID]) {
				node.Pools = append(node.Pools, p.Name)
			}
		}
		if pr, ok := prices[n.ID]; ok {
			node.MonthlyPrice = pr.MonthlyPrice
		}
		out = append(out, node)
	}
	return out, nil
}

func scaleInPoolMatches(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func (rt *Router) nodeScaleWorker() *nodescale.Worker {
	rt.scaleInOnce.Do(func() {
		rt.scaleIn = nodescale.New(scaleInSource{rt}, scaleInSettings{rt}, nodeScaleInTick, rt.recordScaleInEvent)
	})
	return rt.scaleIn
}

// RunNodeScaleIn evaluates idle nodes until ctx ends. It only records would_destroy events.
func (rt *Router) RunNodeScaleIn(ctx context.Context) {
	if rt.nodes == nil {
		return
	}
	rt.nodeScaleWorker().Run(ctx)
}

func (rt *Router) recordScaleInEvent(e nodescale.Event) {
	rt.logger.Info("api: node scale-in would destroy", slog.String("node_id", e.NodeID), slog.Int("idle_minutes", e.IdleMinutes))
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	detail, _ := json.Marshal(e)
	entry := store.AuditEntry{
		ID: id, CreatedAt: store.FormatAuditTime(e.At), ActorType: auditActorSystem, ActorID: "node-scale-in", ActorName: "node scale-in",
		Ability: AbilityRoot, Method: http.MethodGet, Path: "/api/v1/nodes/" + e.NodeID, StatusCode: http.StatusOK,
		ClientKind: "system", Action: "node.scale_in." + e.Kind, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.Background(), entry); err != nil {
		rt.logger.Warn("api: save scale-in audit failed", slog.String("error", err.Error()))
	}
}

// handleNodeScaleInPreview handles GET /api/v1/nodes/scale-in/preview. Read only.
func (rt *Router) handleNodeScaleInPreview(w http.ResponseWriter, r *http.Request) {
	p, err := rt.nodeScaleWorker().Preview(r.Context(), time.Now())
	if err != nil {
		rt.internalError(w, "api: node scale-in preview failed", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
