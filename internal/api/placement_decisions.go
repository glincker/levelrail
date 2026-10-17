package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Triggers recorded with a placement decision.
const (
	placementTriggerCreate = "create"
	placementTriggerMove   = "move"
	placementTriggerDrain  = "drain"
)

// PlacementDecisionStore persists the latest placement decision per resource.
type PlacementDecisionStore interface {
	SavePlacementDecision(ctx context.Context, d store.PlacementDecision) error
	GetPlacementDecision(ctx context.Context, kind, resource string) (store.PlacementDecision, bool, error)
}

// WithPlacementPolicy sets the overcommit and headroom policy placement applies.
func WithPlacementPolicy(p placement.Policy) Option {
	return func(rt *Router) { rt.placementPolicy = p }
}

// recordPlacement stores d as kind/name's latest decision. A failure is
// logged and dropped: the explanation must never block the placement.
func (rt *Router) recordPlacement(ctx context.Context, kind, name, trigger string, d placement.Decision) {
	if rt.decisions == nil || name == "" {
		return
	}
	body, err := json.Marshal(d)
	if err != nil {
		rt.logger.Warn("api: record placement decision: encode failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	rec := store.PlacementDecision{Kind: kind, Resource: name, Trigger: trigger, NodeID: d.NodeID, DecidedAt: time.Now(), Body: body}
	if err := rt.decisions.SavePlacementDecision(ctx, rec); err != nil {
		rt.logger.Warn("api: record placement decision failed", slog.String("error", err.Error()), slog.String("kind", kind), slog.String("name", name))
	}
}

// recordPinned records an explicit operator choice with the fit it had.
func (rt *Router) recordPinned(ctx context.Context, kind, name, nodeID, trigger string) {
	if rt.decisions == nil || nodeID == "" {
		return
	}
	needs, err := rt.resourceNeeds(ctx, kind, name)
	if err != nil {
		return
	}
	inv, err := rt.loadPlacementInventory(ctx)
	if err != nil {
		return
	}
	cands, err := rt.candidatesFrom(ctx, inv)
	if err != nil {
		return
	}
	rt.recordPlacement(ctx, kind, name, trigger, placement.Pinned(cands, placement.Request{Needs: needs, Policy: rt.placementPolicy}, nodeID))
}

// resourceNeeds is what an existing app or database asks for.
func (rt *Router) resourceNeeds(ctx context.Context, kind, name string) (placement.Resources, error) {
	if kind == store.NodeMoveKindDatabase {
		_, err := rt.databases.GetDesiredDatabase(ctx, name)
		return placement.Resources{}, err
	}
	s, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return placement.Resources{}, err
	}
	return serviceRequests(s.Resources, s.Replicas), nil
}

type recordedDecision struct {
	Trigger   string             `json:"trigger"`
	DecidedAt time.Time          `json:"decided_at"`
	Decision  placement.Decision `json:"decision"`
}

type placementDecisionResource struct {
	Kind     string            `json:"kind"`
	Name     string            `json:"name"`
	NodeID   string            `json:"node_id"`
	Recorded *recordedDecision `json:"recorded"`
	// Preview re-evaluates every node now, as if the workload were being placed fresh.
	Preview placement.Decision `json:"preview"`
}

func (rt *Router) placementDecisionView(ctx context.Context, kind, name string) (placementDecisionResource, error) {
	nodeID, err := rt.currentNode(ctx, kind, name)
	if err != nil {
		return placementDecisionResource{}, err
	}
	needs, err := rt.resourceNeeds(ctx, kind, name)
	if err != nil {
		return placementDecisionResource{}, err
	}
	sel, err := rt.selectorFor(ctx, kind, name)
	if err != nil {
		return placementDecisionResource{}, err
	}
	inv, err := rt.loadPlacementInventory(ctx)
	if err != nil {
		return placementDecisionResource{}, err
	}
	cands, err := rt.candidatesFrom(ctx, inv)
	if err != nil {
		return placementDecisionResource{}, err
	}
	home := nodeID
	if home == "" {
		home = rt.localNodeID
	}
	for i := range cands {
		if cands[i].ID == home {
			cands[i].Requested.CPUMilli -= needs.CPUMilli
			cands[i].Requested.MemoryBytes -= needs.MemoryBytes
			cands[i].Load = max(cands[i].Load-1, 0)
		}
	}
	out := placementDecisionResource{Kind: kind, Name: name, NodeID: nodeID}
	out.Preview, _ = placement.Spread(cands, placement.Request{Selector: sel, Needs: needs, Policy: rt.placementPolicy})
	if rt.decisions == nil {
		return out, nil
	}
	rec, ok, err := rt.decisions.GetPlacementDecision(ctx, kind, name)
	if err != nil || !ok {
		return out, err
	}
	var d placement.Decision
	if err := json.Unmarshal(rec.Body, &d); err != nil {
		rt.logger.Warn("api: placement decision unreadable", slog.String("error", err.Error()), slog.String("name", name))
		return out, nil
	}
	out.Recorded = &recordedDecision{Trigger: rec.Trigger, DecidedAt: rec.DecidedAt, Decision: d}
	return out, nil
}

// handleGetPlacementDecision handles GET /api/v1/{apps,databases}/{name}/placement/decision.
func (rt *Router) handleGetPlacementDecision(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rt.requireMultiNode(w) {
			return
		}
		name := r.PathValue("name")
		view, err := rt.placementDecisionView(r.Context(), kind, name)
		if isResourceNotFound(err) {
			writeError(w, http.StatusNotFound, kind+" not found")
			return
		} else if err != nil {
			rt.internalError(w, "api: get placement decision failed", err, slog.String("kind", kind), slog.String("name", name))
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

type nodeAllocationResource struct {
	NodeID      string                `json:"node_id"`
	Name        string                `json:"name"`
	IsLocal     bool                  `json:"is_local"`
	Schedulable bool                  `json:"schedulable"`
	Online      bool                  `json:"online"`
	Workloads   int                   `json:"workloads"`
	Dimensions  []placement.Dimension `json:"dimensions"`
}

type nodeAllocationsResponse struct {
	Policy placement.Policy         `json:"policy"`
	Nodes  []nodeAllocationResource `json:"nodes"`
}

// handleNodeAllocations handles GET /api/v1/nodes/allocations: every node's
// allocatable capacity against what placed workloads request.
func (rt *Router) handleNodeAllocations(w http.ResponseWriter, r *http.Request) {
	inv, err := rt.loadPlacementInventory(r.Context())
	if err != nil {
		rt.internalError(w, "api: node allocations: load failed", err)
		return
	}
	cands, err := rt.candidatesFrom(r.Context(), inv)
	if err != nil {
		rt.internalError(w, "api: node allocations: candidates failed", err)
		return
	}
	out := nodeAllocationsResponse{Policy: rt.placementPolicy, Nodes: make([]nodeAllocationResource, 0, len(cands))}
	for _, c := range cands {
		e := placement.Evaluate(c, placement.Request{Policy: rt.placementPolicy})
		out.Nodes = append(out.Nodes, nodeAllocationResource{
			NodeID: c.ID, Name: c.Name, IsLocal: c.ID == rt.localNodeID, Schedulable: c.Schedulable,
			Online: c.Online, Workloads: c.Load, Dimensions: e.Dimensions,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
