package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodefailure"
	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

const placementTriggerNodeFailure = "node_failure"

// WithNodeFailure enables the node failure policy controller and its routes.
func WithNodeFailure(s nodefailure.Store, cfg nodefailure.Config) Option {
	return func(rt *Router) {
		rt.nodeFailure = nodefailure.New(s, &failureExecutor{rt: rt, move: &moveExecutor{rt: rt}}, cfg, rt.logger)
	}
}

// RunNodeFailure evaluates node failure state until ctx is done.
func (rt *Router) RunNodeFailure(ctx context.Context) {
	if rt.nodeFailure != nil {
		rt.nodeFailure.Run(ctx)
	}
}

type failureExecutor struct {
	rt   *Router
	move *moveExecutor
}

func (e *failureExecutor) IsLocal(nodeID string) bool {
	return nodeID == e.rt.localNodeID
}

func (e *failureExecutor) Resources(ctx context.Context, nodeID string) ([]nodefailure.Resource, error) {
	rs, err := e.move.Remaining(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]nodefailure.Resource, 0, len(rs))
	for _, r := range rs {
		out = append(out, nodefailure.Resource{Kind: r.Kind, Name: r.Name, Volumes: r.Volumes})
	}
	return out, nil
}

func (e *failureExecutor) NodeOf(ctx context.Context, kind, name string) (string, error) {
	if kind == store.NodeMoveKindDatabase {
		d, err := e.rt.databases.GetDesiredDatabase(ctx, name)
		if err != nil {
			return "", err
		}
		return d.NodeID, nil
	}
	svc, err := e.rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return "", err
	}
	return svc.NodeID, nil
}

func (e *failureExecutor) PickTarget(ctx context.Context, res nodefailure.Resource, from string, commit bool) (string, error) {
	if commit {
		return e.move.PickTarget(ctx, nodemove.Resource{Kind: res.Kind, Name: res.Name, Volumes: res.Volumes}, from, "")
	}
	d, err := e.rt.spreadDecision(ctx, res.Kind, res.Name, from)
	return d.NodeID, err
}

func (e *failureExecutor) Place(ctx context.Context, kind, name, nodeID string) error {
	return e.move.Place(ctx, kind, name, nodeID)
}

func (e *failureExecutor) Fence(ctx context.Context, kind, name, nodeID string) error {
	return e.move.RemoveFrom(ctx, kind, name, nodeID)
}

func (e *failureExecutor) Audit(ctx context.Context, action, detail string) {
	e.rt.auditSystemEvent(ctx, "node-failure", action, detail)
}

// auditSystemEvent records an automatic, platform-initiated action.
func (rt *Router) auditSystemEvent(ctx context.Context, actor, action, detail string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	err = rt.auditLog.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: "system", ActorName: actor, Ability: AbilityRoot, Method: http.MethodPost,
		Path: action + ": " + detail, StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: "system", Action: action,
	})
	if err != nil {
		rt.logger.Warn("api: system audit failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func (rt *Router) requireNodeFailure(w http.ResponseWriter) bool {
	if rt.nodeFailure == nil {
		writeError(w, http.StatusNotImplemented, "node failure policy is not configured on this control plane")
		return false
	}
	return true
}

func (rt *Router) failureError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNodeNotFound):
		writeError(w, http.StatusNotFound, "node not found")
	case errors.Is(err, nodefailure.ErrNodeHealthy):
		writeError(w, http.StatusConflict, "node is reachable, nothing to do")
	case errors.Is(err, nodefailure.ErrNoFailure), errors.Is(err, nodefailure.ErrNoSuchAction):
		writeError(w, http.StatusConflict, err.Error())
	default:
		rt.internalError(w, "api: node failure failed", err)
	}
}

// handleGetNodeFailure handles GET /api/v1/nodes/{id}/failure.
func (rt *Router) handleGetNodeFailure(w http.ResponseWriter, r *http.Request) {
	if !rt.requireNodeFailure(w) {
		return
	}
	st, err := rt.nodeFailure.Status(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.failureError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type failurePolicyBody struct {
	Mode         string `json:"mode"`
	GraceSeconds int    `json:"grace_seconds"`
}

// handleSetNodeFailurePolicy handles PUT /api/v1/nodes/{id}/failure-policy.
// An empty mode and zero grace go back to the control plane defaults.
func (rt *Router) handleSetNodeFailurePolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.requireNodeFailure(w) {
		return
	}
	var body failurePolicyBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Mode != "" && !store.IsNodeFailureMode(body.Mode) {
		writeError(w, http.StatusBadRequest, "mode must be off, notify, auto or empty for the default")
		return
	}
	if body.GraceSeconds < 0 || (body.GraceSeconds > 0 && body.GraceSeconds < 30) {
		writeError(w, http.StatusBadRequest, "grace_seconds must be 0 (default) or at least 30")
		return
	}
	id := r.PathValue("id")
	if err := rt.nodeFailure.SetPolicy(r.Context(), store.NodeFailurePolicy{NodeID: id, Mode: body.Mode, GraceSeconds: body.GraceSeconds}); err != nil {
		rt.failureError(w, err)
		return
	}
	st, err := rt.nodeFailure.Status(r.Context(), id)
	if err != nil {
		rt.failureError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type failureLostBody struct {
	DryRun  bool `json:"dry_run"`
	Confirm bool `json:"confirm"`
}

// handleMarkNodeLost handles POST /api/v1/nodes/{id}/failure/lost: dry_run
// returns the plan, anything else needs confirm.
func (rt *Router) handleMarkNodeLost(w http.ResponseWriter, r *http.Request) {
	if !rt.requireNodeFailure(w) {
		return
	}
	var body failureLostBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !body.DryRun && !body.Confirm {
		writeError(w, http.StatusBadRequest, "set confirm to true, or dry_run to see the plan first")
		return
	}
	st, err := rt.nodeFailure.MarkLost(r.Context(), r.PathValue("id"), body.DryRun)
	if err != nil {
		rt.failureError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type failureResolveBody struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Action       string `json:"action"`
	TargetNodeID string `json:"target_node_id"`
}

// handleResolveNodeFailure handles POST /api/v1/nodes/{id}/failure/resolve.
func (rt *Router) handleResolveNodeFailure(w http.ResponseWriter, r *http.Request) {
	if !rt.requireNodeFailure(w) {
		return
	}
	var body failureResolveBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Kind == "" || body.Name == "" || (body.Action != nodefailure.ResolveWait && body.Action != nodefailure.ResolveRestore) {
		writeError(w, http.StatusBadRequest, "kind, name and action (wait or restore) are required")
		return
	}
	ref := appRef(body.Name)
	if body.Kind == store.NodeMoveKindDatabase {
		ref = databaseRef(body.Name)
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "resource not found"), ref, nodeRef(r.PathValue("id"))) {
		return
	}
	if body.TargetNodeID != "" {
		if err := rt.checkPlacement(r.Context(), body.Kind, body.Name, body.TargetNodeID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	st, err := rt.nodeFailure.Resolve(r.Context(), r.PathValue("id"), body.Kind, body.Name, body.Action, body.TargetNodeID)
	if err != nil {
		if errors.Is(err, placement.ErrNoEligibleNode) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		rt.failureError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// nodeFailureSummary is the compact failure view embedded in node resources.
type nodeFailureSummary struct {
	Phase    string `json:"phase"`
	Mode     string `json:"mode,omitempty"`
	Reason   string `json:"reason,omitempty"`
	NeedsYou int    `json:"needs_decision"`
	Moved    int    `json:"moved"`
	Since    string `json:"unreachable_at,omitempty"`
}

// failureSummaries maps node ID to its live failure state for list and detail views.
func (rt *Router) failureSummaries(ctx context.Context) map[string]*nodeFailureSummary {
	out := map[string]*nodeFailureSummary{}
	if rt.nodeFailure == nil {
		return out
	}
	states, err := rt.nodeFailure.States(ctx)
	if err != nil {
		rt.logger.Warn("api: list node failure states failed", slog.String("error", err.Error()))
		return out
	}
	for id, s := range states {
		sum := &nodeFailureSummary{Phase: s.Phase, Reason: s.Reason, Since: store.FormatAuditTime(s.UnreachableAt)}
		for _, a := range s.Actions {
			switch {
			case a.Class == store.NodeFailureClassDecision && a.Status != store.NodeFailureActionMoved && a.Status != store.NodeFailureActionFenced:
				sum.NeedsYou++
			case a.Status == store.NodeFailureActionMoved || a.Status == store.NodeFailureActionFenced:
				sum.Moved++
			}
		}
		out[id] = sum
	}
	return out
}
