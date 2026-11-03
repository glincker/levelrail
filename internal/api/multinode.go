package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PlacementStore is the persistence multi-node placement needs; *store.DB satisfies it.
type PlacementStore interface {
	nodemove.Store
	ListNodeLabels(ctx context.Context) (map[string]map[string]string, error)
	GetNodeLabels(ctx context.Context, nodeID string) (map[string]string, error)
	SetNodeLabels(ctx context.Context, nodeID string, labels map[string]string) error
	ListNodeTaints(ctx context.Context) (map[string][]store.NodeTaint, error)
	GetNodeTaints(ctx context.Context, nodeID string) ([]store.NodeTaint, error)
	SetNodeTaints(ctx context.Context, nodeID string, taints []store.NodeTaint) error
	GetPlacementSelector(ctx context.Context, kind, resource string) (map[string]string, error)
	SetPlacementSelector(ctx context.Context, kind, resource string, selector map[string]string) error
	UpdateServicePlacementRules(ctx context.Context, name string, tolerations, affinity json.RawMessage) error
	EnsureDatabaseMeshPort(ctx context.Context, name string, minPort, maxPort int) (int, error)
}

// NodeMeshStatusSource reads a remote node's own WireGuard view over its agent session.
type NodeMeshStatusSource interface {
	MeshStatus(ctx context.Context, nodeID string) (network.Status, error)
}

type multiNodeState struct {
	store      PlacementStore
	runner     *nodemove.Runner
	peerStatus NodeMeshStatusSource
	portMin    int
	portMax    int
}

// WithMultiNode enables persisted node moves, drains, labels and selectors.
// Without it those routes return 501 and drain keeps no history.
func WithMultiNode(s PlacementStore, cfg nodemove.Config, meshPortMin, meshPortMax int) Option {
	return func(rt *Router) {
		exec := &moveExecutor{rt: rt}
		if ds, ok := s.(PlacementDecisionStore); ok {
			rt.decisions = ds
		}
		rt.multiNode = &multiNodeState{
			store:   s,
			runner:  nodemove.NewRunner(s, exec, cfg, rt.logger, nodemove.WithDrainPlanner(exec)),
			portMin: meshPortMin,
			portMax: meshPortMax,
		}
	}
}

// RunNodeMoves advances node moves and drains until ctx is done, resuming
// any left running by a previous process.
func (rt *Router) RunNodeMoves(ctx context.Context) {
	if rt.multiNode != nil {
		rt.multiNode.runner.Run(ctx)
	}
}

// SetMeshPeerStatus wires node-pair diagnostics to remote agents' mesh views.
func (rt *Router) SetMeshPeerStatus(src NodeMeshStatusSource) {
	if rt.multiNode != nil {
		rt.multiNode.peerStatus = src
	}
}

func (rt *Router) requireMultiNode(w http.ResponseWriter) bool {
	if rt.multiNode == nil {
		writeError(w, http.StatusNotImplemented, "multi-node placement is not configured on this control plane")
		return false
	}
	return true
}

// placementCandidates is every registered node as the scheduler sees it.
func (rt *Router) placementCandidates(ctx context.Context) ([]placement.Candidate, error) {
	inv, err := rt.loadPlacementInventory(ctx)
	if err != nil {
		return nil, err
	}
	return rt.candidatesFrom(ctx, inv)
}

func (rt *Router) selectorFor(ctx context.Context, kind, name string) (placement.Selector, error) {
	if rt.multiNode == nil {
		return nil, nil
	}
	sel, err := rt.multiNode.store.GetPlacementSelector(ctx, kind, name)
	return placement.Selector(sel), err
}

// checkPlacement validates an explicit target for kind/name: it must be a
// known, schedulable, online node whose labels satisfy the selector. ""
// (the local node without mesh) is always allowed.
func (rt *Router) checkPlacement(ctx context.Context, kind, name, nodeID string) error {
	if nodeID == "" {
		return nil
	}
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		return err
	}
	sel, err := rt.selectorFor(ctx, kind, name)
	if err != nil {
		return err
	}
	cands, req, err := rt.ruledRequest(ctx, kind, name, cands, placement.Request{Selector: sel})
	if err != nil {
		return err
	}
	for _, c := range cands {
		if c.ID == nodeID {
			return placement.Check(c, req)
		}
	}
	return fmt.Errorf("%w: node %s is not registered", placement.ErrNoEligibleNode, nodeID)
}

// spreadTarget picks the eligible node with the most headroom for kind/name,
// never exclude, and records why.
func (rt *Router) spreadTarget(ctx context.Context, kind, name, exclude, trigger string) (string, error) {
	d, err := rt.spreadDecision(ctx, kind, name, exclude)
	if err == nil {
		rt.recordPlacement(ctx, kind, name, trigger, d)
	}
	return d.NodeID, err
}

// spreadDecision is spreadTarget without recording, for previews.
func (rt *Router) spreadDecision(ctx context.Context, kind, name, exclude string) (placement.Decision, error) {
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		return placement.Decision{}, err
	}
	sel, err := rt.selectorFor(ctx, kind, name)
	if err != nil {
		return placement.Decision{}, err
	}
	needs, err := rt.resourceNeeds(ctx, kind, name)
	if err != nil {
		return placement.Decision{}, err
	}
	cands, req, err := rt.ruledRequest(ctx, kind, name, cands, placement.Request{Selector: sel, Exclude: exclude, Needs: needs, Policy: rt.placementPolicy})
	if err != nil {
		return placement.Decision{}, err
	}
	return placement.Spread(cands, req)
}

type labelsBody struct {
	Labels map[string]string `json:"labels"`
}

// handleGetNodeLabels handles GET /api/v1/nodes/{id}/labels.
func (rt *Router) handleGetNodeLabels(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get node labels: look up node failed", err, slog.String("node_id", id))
		return
	}
	labels, err := rt.multiNode.store.GetNodeLabels(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get node labels failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, labelsBody{Labels: labels})
}

// handleSetNodeLabels handles PUT /api/v1/nodes/{id}/labels, replacing the set.
func (rt *Router) handleSetNodeLabels(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	id := r.PathValue("id")
	var body labelsBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := placement.ValidateLabels(body.Labels); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.multiNode.store.SetNodeLabels(r.Context(), id, body.Labels); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set node labels failed", err, slog.String("node_id", id))
		return
	}
	if body.Labels == nil {
		body.Labels = map[string]string{}
	}
	writeJSON(w, http.StatusOK, body)
}

type taintsBody struct {
	Taints []placement.Taint `json:"taints"`
}

// handleGetNodeTaints handles GET /api/v1/nodes/{id}/taints.
func (rt *Router) handleGetNodeTaints(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get node taints: look up node failed", err, slog.String("node_id", id))
		return
	}
	taints, err := rt.multiNode.store.GetNodeTaints(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get node taints failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, taintsBody{Taints: toPlacementTaints(taints)})
}

// handleSetNodeTaints handles PUT /api/v1/nodes/{id}/taints, replacing the set.
func (rt *Router) handleSetNodeTaints(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	id := r.PathValue("id")
	var body taintsBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := placement.ValidateTaints(body.Taints); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows := make([]store.NodeTaint, len(body.Taints))
	for i, t := range body.Taints {
		rows[i] = store.NodeTaint{Key: t.Key, Value: t.Value, Effect: string(t.Effect)}
	}
	if err := rt.multiNode.store.SetNodeTaints(r.Context(), id, rows); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set node taints failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, taintsBody{Taints: toPlacementTaints(rows)})
}

func toPlacementTaints(rows []store.NodeTaint) []placement.Taint {
	out := make([]placement.Taint, len(rows))
	for i, t := range rows {
		out[i] = placement.Taint{Key: t.Key, Value: t.Value, Effect: placement.TaintEffect(t.Effect)}
	}
	return out
}

type placementResource struct {
	Kind         string            `json:"kind"`
	Name         string            `json:"name"`
	NodeID       string            `json:"node_id"`
	NodeSelector map[string]string `json:"node_selector"`
	// Tolerations and Affinity are an app's placement rules; databases have none.
	Tolerations []placement.Toleration `json:"tolerations"`
	Affinity    *placement.Affinity    `json:"affinity,omitempty"`
	Eligible    []string               `json:"eligible_node_ids"`
	Rejected    []placement.Rejection  `json:"rejected"`
	// DomainWarnings lists failure domains that hold every replica of the app.
	DomainWarnings []placement.DomainRisk `json:"domain_warnings"`
}

// placementBody sets the selector and, for apps, the rules. Omitted rules stay
// as they are; [] clears tolerations and {} clears affinity.
type placementBody struct {
	NodeSelector map[string]string       `json:"node_selector"`
	Tolerations  *[]placement.Toleration `json:"tolerations"`
	Affinity     *placement.Affinity     `json:"affinity"`
}

func (rt *Router) currentNode(ctx context.Context, kind, name string) (string, error) {
	if kind == store.NodeMoveKindDatabase {
		d, err := rt.databases.GetDesiredDatabase(ctx, name)
		if err != nil {
			return "", err
		}
		return d.NodeID, nil
	}
	s, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return "", err
	}
	return s.NodeID, nil
}

func (rt *Router) placementView(ctx context.Context, kind, name string) (placementResource, error) {
	nodeID, err := rt.currentNode(ctx, kind, name)
	if err != nil {
		return placementResource{}, err
	}
	sel, err := rt.selectorFor(ctx, kind, name)
	if err != nil {
		return placementResource{}, err
	}
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		return placementResource{}, err
	}
	out := placementResource{Kind: kind, Name: name, NodeID: nodeID, NodeSelector: sel, Eligible: []string{}, Rejected: []placement.Rejection{}, DomainWarnings: []placement.DomainRisk{}}
	if out.NodeSelector == nil {
		out.NodeSelector = map[string]string{}
	}
	rules, err := rt.rulesFor(ctx, kind, name)
	if err != nil {
		return placementResource{}, err
	}
	out.Tolerations, out.Affinity = rules.Tolerations, rules.Affinity
	if out.Tolerations == nil {
		out.Tolerations = []placement.Toleration{}
	}
	needs, err := rt.resourceNeeds(ctx, kind, name)
	if err != nil {
		return placementResource{}, err
	}
	cands, req, err := rt.ruledRequest(ctx, kind, name, cands, placement.Request{Selector: sel, Needs: needs, Policy: rt.placementPolicy})
	if err != nil {
		return placementResource{}, err
	}
	for _, c := range cands {
		if e := placement.Evaluate(c, req); !e.Eligible {
			out.Rejected = append(out.Rejected, placement.Rejection{NodeID: c.ID, Reason: e.Reason})
		} else {
			out.Eligible = append(out.Eligible, c.ID)
		}
	}
	if kind == store.NodeMoveKindApp {
		risks, err := rt.appDomainRisks(ctx, name, cands)
		if err != nil {
			return placementResource{}, err
		}
		out.DomainWarnings = append(out.DomainWarnings, risks...)
	}
	return out, nil
}

func isResourceNotFound(err error) bool {
	return errors.Is(err, store.ErrServiceNotFound) || errors.Is(err, store.ErrDatabaseNotFound)
}

func (rt *Router) handleGetPlacement(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rt.requireMultiNode(w) {
			return
		}
		name := r.PathValue("name")
		view, err := rt.placementView(r.Context(), kind, name)
		if isResourceNotFound(err) {
			writeError(w, http.StatusNotFound, kind+" not found")
			return
		} else if err != nil {
			rt.internalError(w, "api: get placement failed", err, slog.String("kind", kind), slog.String("name", name))
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// handleSetPlacement sets a selector. It never moves anything: an existing
// placement that no longer matches is reported in the response, and the
// next move or drain honors the selector.
func (rt *Router) handleSetPlacement(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rt.requireMultiNode(w) {
			return
		}
		name := r.PathValue("name")
		var body placementBody
		if err := decodeJSONBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := placement.ValidateLabels(body.NodeSelector); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rulesSent := body.Tolerations != nil || body.Affinity != nil
		if rulesSent && kind != store.NodeMoveKindApp {
			writeError(w, http.StatusBadRequest, "tolerations and affinity apply to apps only")
			return
		}
		if rulesSent {
			var tols []placement.Toleration
			if body.Tolerations != nil {
				tols = *body.Tolerations
			}
			if err := placement.ValidateRules(tols, body.Affinity); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if _, err := rt.currentNode(r.Context(), kind, name); isResourceNotFound(err) {
			writeError(w, http.StatusNotFound, kind+" not found")
			return
		} else if err != nil {
			rt.internalError(w, "api: set placement: load failed", err, slog.String("name", name))
			return
		}
		// A rules-only body leaves the selector alone.
		if body.NodeSelector != nil || !rulesSent {
			if err := rt.multiNode.store.SetPlacementSelector(r.Context(), kind, name, body.NodeSelector); err != nil {
				rt.internalError(w, "api: set placement failed", err, slog.String("kind", kind), slog.String("name", name))
				return
			}
		}
		if rulesSent {
			if err := rt.saveRules(r.Context(), name, body); err != nil {
				rt.internalError(w, "api: set placement rules failed", err, slog.String("name", name))
				return
			}
		}
		view, err := rt.placementView(r.Context(), kind, name)
		if err != nil {
			rt.internalError(w, "api: set placement: reload failed", err, slog.String("name", name))
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}
