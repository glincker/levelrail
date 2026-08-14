package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeJoinTokenTTL is how long a minted join token stays redeemable
// before an operator has to mint a new one. Not (yet) an env-configurable
// threshold: this is a one-shot enrollment window, not an ongoing
// operational setting like APP_SESSION_TTL, so CLAUDE.md 7's "no
// hardcoded thresholds" concern doesn't apply the same way here. Revisit
// if real usage shows this needs to be adjustable.
const nodeJoinTokenTTL = 15 * time.Minute

// NodeStore is the store surface the node-management handlers need.
// *store.DB satisfies this structurally, the same narrow
// consumer-defined interface convention every other Store sub-interface
// in this package already follows.
type NodeStore interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
	GetNode(ctx context.Context, id string) (*store.Node, error)
	DeleteNode(ctx context.Context, id string) error
	SaveNodeJoinToken(ctx context.Context, t store.NodeJoinToken) error
	UpdateNodeWorkloads(ctx context.Context, id string, acceptsApp, acceptsBuild bool) error
	// SetNodeSchedulable is TASKS.md 3.7's cordon/uncordon mutation.
	SetNodeSchedulable(ctx context.Context, id string, schedulable bool) error
}

// nodeResource is the wire shape for a node.
type nodeResource struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Address         string     `json:"address,omitempty"`
	Status          string     `json:"status"`
	CertFingerprint string     `json:"cert_fingerprint,omitempty"`
	JoinedAt        *time.Time `json:"joined_at,omitempty"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	// Schedulable is TASKS.md 3.7's cordon state: false means the node
	// refuses new placements (handleSetAppNode, handleDrainNode) while
	// whatever's already running there keeps running, see
	// store.Node.Schedulable's own doc comment for why this is a
	// separate field from Status rather than folded into it.
	Schedulable           bool      `json:"schedulable"`
	AcceptsAppWorkloads   bool      `json:"accepts_app_workloads"`
	AcceptsBuildWorkloads bool      `json:"accepts_build_workloads"`
	CreatedAt             time.Time `json:"created_at"`
}

func toNodeResource(n store.Node) nodeResource {
	return nodeResource{
		ID:                    n.ID,
		Name:                  n.Name,
		Address:               n.Address,
		Status:                string(n.Status),
		CertFingerprint:       n.CertFingerprint,
		JoinedAt:              n.JoinedAt,
		LastSeenAt:            n.LastSeenAt,
		Schedulable:           n.Schedulable,
		AcceptsAppWorkloads:   n.AcceptsAppWorkloads,
		AcceptsBuildWorkloads: n.AcceptsBuildWorkloads,
		CreatedAt:             n.CreatedAt,
	}
}

// errNodeCordoned is validatePlacementTarget's sentinel for "this node
// exists but isn't accepting new placements right now." Distinct from
// store.ErrNodeNotFound so callers (handleSetAppNode, handleDrainNode)
// can each pick their own wording, both still a 400: neither is a server
// failure.
var errNodeCordoned = errors.New("api: node is cordoned")

// validatePlacementTarget checks nodeID is safe to place or move a
// resource onto: a real, schedulable node, or the empty string (the
// local-node sentinel, DesiredService.NodeID's own doc comment), which
// has no row in the nodes table to check and is always valid. Shared by
// handleSetAppNode (TASKS.md 3.3) and handleDrainNode (TASKS.md 3.7,
// validating the drain target) so the "is this a legal placement target"
// rule lives in exactly one place. Returns store.ErrNodeNotFound or
// errNodeCordoned, both handled explicitly by callers rather than this
// helper writing the HTTP response itself, since the two call sites want
// different wording for the same failure.
func (rt *Router) validatePlacementTarget(ctx context.Context, nodeID string) error {
	if nodeID == "" {
		return nil
	}
	node, err := rt.nodes.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if !node.Schedulable {
		return errNodeCordoned
	}
	return nil
}

// handleListNodes handles GET /api/v1/nodes (TASKS.md 3.1).
func (rt *Router) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		rt.logger.Error("api: list nodes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]nodeResource, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toNodeResource(n))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetNode handles GET /api/v1/nodes/{id}.
func (rt *Router) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toNodeResource(*n))
}

// handleDeleteNode handles DELETE /api/v1/nodes/{id}. Idempotent at the
// store layer (store.DeleteNode), matching handleDeleteAlertRule's own
// idempotent-delete shape: deleting an already-gone or never-placed-on
// node ID proceeds straight through, both list calls below simply
// return empty.
//
// TASKS.md 3.7 is what makes this a real, safe delete instead of only
// ever failing loudly: it refuses to run (409) while any service or
// database still has this node as its NodeID, pointing the operator at
// POST .../drain, the only way those placements actually clear (short
// of manually moving each one via PUT /api/v1/apps/{name}/node). Once
// drained, the same request that was refused now succeeds with no other
// change needed.
func (rt *Router) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	services, err := rt.apps.ListDesiredServicesByNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: delete node: list placed services failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	databases, err := rt.databases.ListDesiredDatabasesByNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: delete node: list placed databases failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(services) > 0 || len(databases) > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"node %q still has %d service(s) and %d database(s) placed on it; drain it first via POST /api/v1/nodes/%s/drain",
			id, len(services), len(databases), id,
		))
		return
	}

	if err := rt.nodes.DeleteNode(r.Context(), id); err != nil {
		rt.logger.Error("api: delete node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setNodeWorkloadsRequest is the body PUT /api/v1/nodes/{id}/workloads
// expects. Both fields are required, not individually optional/nil-able
// PATCH-style fields: a caller always states the node's complete
// intended capability set in one call, the same "full replace, not a
// partial merge" shape handleSetAppNode's own node_id field already
// uses for placement, so there is never an ambiguous "field omitted,
// leave unchanged" case to reason about.
type setNodeWorkloadsRequest struct {
	AcceptsAppWorkloads   bool `json:"accepts_app_workloads"`
	AcceptsBuildWorkloads bool `json:"accepts_build_workloads"`
}

// handleSetNodeWorkloads handles PUT /api/v1/nodes/{id}/workloads
// (TASKS.md 3.5): the only way a node's workload capability changes.
// This is what internal/build.SelectBuildNode's node list ultimately
// reflects (via cmd/levelrail's wiring), so toggling
// accepts_build_workloads here is what actually opts a node in or out
// of dedicated build-node consideration.
func (rt *Router) handleSetNodeWorkloads(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req setNodeWorkloadsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.nodes.UpdateNodeWorkloads(r.Context(), id, req.AcceptsAppWorkloads, req.AcceptsBuildWorkloads); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set node workloads failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	n, err := rt.nodes.GetNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: set node workloads: reload after update failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toNodeResource(*n))
}

// handleCordonNode handles POST /api/v1/nodes/{id}/cordon (TASKS.md
// 3.7): marks id unschedulable for new placements without evacuating
// anything already running there (store.SetNodeSchedulable's own doc
// comment). Idempotent: cordoning an already-cordoned node is a
// successful no-op, matching UpdateNodeStatus's own tolerance for
// setting a status a node is already in.
func (rt *Router) handleCordonNode(w http.ResponseWriter, r *http.Request) {
	rt.setNodeSchedulable(w, r, false)
}

// handleUncordonNode handles POST /api/v1/nodes/{id}/uncordon: the
// inverse of handleCordonNode.
func (rt *Router) handleUncordonNode(w http.ResponseWriter, r *http.Request) {
	rt.setNodeSchedulable(w, r, true)
}

func (rt *Router) setNodeSchedulable(w http.ResponseWriter, r *http.Request, schedulable bool) {
	id := r.PathValue("id")
	if err := rt.nodes.SetNodeSchedulable(r.Context(), id, schedulable); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set node schedulable failed", slog.String("error", err.Error()), slog.String("node_id", id), slog.Bool("schedulable", schedulable))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	n, err := rt.nodes.GetNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: set node schedulable: reload after update failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toNodeResource(*n))
}

// drainNodeResponse reports what a drain actually moved, and what it
// didn't: TASKS.md's own scope note calls out testing "what happens if
// moving service 2 of 3 off a node fails partway through" as the
// reconciler-adjacent case to cover here, and the answer has to be
// visible to the caller, not just to a log line. A partial failure is
// not fatal to the request (StatusMultiStatus, not 500): everything that
// did move stays moved, and Errors names exactly what didn't so an
// operator can retry just that resource, e.g. via
// PUT /api/v1/apps/{name}/node directly.
type drainNodeResponse struct {
	TargetNodeID   string   `json:"target_node_id"`
	MovedServices  []string `json:"moved_services"`
	MovedDatabases []string `json:"moved_databases"`
	Errors         []string `json:"errors,omitempty"`
}

// handleDrainNode handles POST /api/v1/nodes/{id}/drain?target_node_id=
// (TASKS.md 3.7): moves every service and database currently placed on
// id to target_node_id (default "", the local-node sentinel), reusing
// TASKS.md 3.3's own UpdateServiceNode/UpdateDatabaseNode one resource
// at a time rather than inventing a second placement mechanism. This is
// the only supported way to clear a node's placements before deleting it
// (handleDeleteNode's own doc comment).
//
// One failure does not stop the rest: the same "one broken resource
// must not block others" principle cmd/levelrail's dynamicSource already
// applies to reconcile passes, applied here to a bulk placement change.
// Each resource is attempted independently and its own outcome recorded
// in the response; a real live-container consequence of this (the
// previous node's own container is not itself stopped by this call, only
// desired placement changes, the reconcile engine's next pass is what
// actually converges each moved resource on its new node) is the same
// known gap TASKS.md 3.3 already left open, not something drain
// introduces.
func (rt *Router) handleDrainNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	targetNodeID := r.URL.Query().Get("target_node_id")

	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: drain node: look up node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.validatePlacementTarget(r.Context(), targetNodeID); err != nil {
		switch {
		case errors.Is(err, store.ErrNodeNotFound):
			writeError(w, http.StatusBadRequest, "unknown target_node_id")
		case errors.Is(err, errNodeCordoned):
			writeError(w, http.StatusBadRequest, "target node is cordoned and not accepting new placements")
		default:
			rt.logger.Error("api: drain node: validate target failed", slog.String("error", err.Error()), slog.String("node_id", id), slog.String("target_node_id", targetNodeID))
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	services, err := rt.apps.ListDesiredServicesByNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: drain node: list services failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	databases, err := rt.databases.ListDesiredDatabasesByNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: drain node: list databases failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := drainNodeResponse{
		TargetNodeID:   targetNodeID,
		MovedServices:  make([]string, 0, len(services)),
		MovedDatabases: make([]string, 0, len(databases)),
	}

	for _, svc := range services {
		if err := rt.apps.UpdateServiceNode(r.Context(), svc.Name, targetNodeID); err != nil {
			rt.logger.Error("api: drain node: move service failed", slog.String("error", err.Error()), slog.String("node_id", id), slog.String("service", svc.Name))
			resp.Errors = append(resp.Errors, fmt.Sprintf("service %s: %s", svc.Name, err.Error()))
			continue
		}
		resp.MovedServices = append(resp.MovedServices, svc.Name)
	}
	for _, d := range databases {
		if err := rt.databases.UpdateDatabaseNode(r.Context(), d.Name, targetNodeID); err != nil {
			rt.logger.Error("api: drain node: move database failed", slog.String("error", err.Error()), slog.String("node_id", id), slog.String("database", d.Name))
			resp.Errors = append(resp.Errors, fmt.Sprintf("database %s: %s", d.Name, err.Error()))
			continue
		}
		resp.MovedDatabases = append(resp.MovedDatabases, d.Name)
	}

	status := http.StatusOK
	if len(resp.Errors) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, resp)
}

// nodeHealthControllerName mirrors applicationControllerName's own
// convention (deploys.go), naming internal/reconcile/nodehealth's own
// "node-health/" + nodeID Name() format as a plain string here rather
// than importing that package, the same "don't pull a runtime-heavy
// package in just for a string format" reasoning applicationControllerName's
// own doc comment already gives.
func nodeHealthControllerName(nodeID string) string {
	return "node-health/" + nodeID
}

// handleGetNodeHealth handles GET /api/v1/nodes/{id}/health (TASKS.md
// 3.7): surfaces internal/reconcile/nodehealth's stored Heartbeat
// condition for this node, the identical "read reconcile_status, this
// handler computes nothing itself" shape handleDeployHistory
// (deploys.go) already established for applications. A node whose
// health controller hasn't reconciled yet (freshly enrolled, before the
// engine's first pass) gets an empty list, not an error, the same "nil
// until first reconcile" behavior GetConditions already documents.
func (rt *Router) handleGetNodeHealth(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: get node health: look up node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	conditions, err := rt.deploys.GetConditions(r.Context(), nodeHealthControllerName(id))
	if err != nil {
		rt.logger.Error("api: get node health failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, conditions)
}

// createNodeJoinTokenResponse is a join token's one and only appearance
// in plaintext, the same "shown once, never recoverable again" shape
// createTokenResponse (tokens.go) already established for API tokens.
type createNodeJoinTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleCreateNodeJoinToken handles POST /api/v1/nodes/join-tokens
// (TASKS.md 3.1): mints a one-time token an operator pastes into a new
// node's enrollment command (TASKS.md 3.2, `cmd/levelrail-agent`, not
// built yet, is what will eventually exchange this token for a client
// certificate). Nothing in this codebase redeems a token yet; this
// handler only mints and persists the hash.
func (rt *Router) handleCreateNodeJoinToken(w http.ResponseWriter, r *http.Request) {
	plaintext, err := randomToken()
	if err != nil {
		rt.logger.Error("api: create node join token: generate token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := randomNodeJoinTokenID()
	if err != nil {
		rt.logger.Error("api: create node join token: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now()
	rec := store.NodeJoinToken{
		ID:        id,
		TokenHash: hashToken(plaintext),
		CreatedAt: now,
		ExpiresAt: now.Add(nodeJoinTokenTTL),
	}
	if err := rt.nodes.SaveNodeJoinToken(r.Context(), rec); err != nil {
		rt.logger.Error("api: create node join token: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, createNodeJoinTokenResponse{Token: plaintext, ExpiresAt: rec.ExpiresAt})
}

// randomNodeJoinTokenID generates a short, URL-safe, non-secret
// identifier for a join-token row, the exact shape randomTokenID
// (tokens.go) already establishes for API tokens, duplicated rather than
// shared: the two ID spaces are for genuinely different resources and
// nothing depends on them being interchangeable.
func randomNodeJoinTokenID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate node join token id: %w", err)
	}
	return "njt_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
