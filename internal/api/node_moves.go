package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

type nodeMoveStepResource struct {
	Phase   string `json:"phase"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
	At      string `json:"at"`
}

type nodeMoveResource struct {
	ID         string                 `json:"id"`
	Kind       string                 `json:"kind"`
	Resource   string                 `json:"resource"`
	FromNodeID string                 `json:"from_node_id"`
	ToNodeID   string                 `json:"to_node_id"`
	Stateful   bool                   `json:"stateful"`
	Volumes    []string               `json:"volumes"`
	Phase      string                 `json:"phase"`
	Status     string                 `json:"status"`
	Error      string                 `json:"error,omitempty"`
	DrainID    string                 `json:"drain_id,omitempty"`
	Steps      []nodeMoveStepResource `json:"steps"`
	StartedAt  string                 `json:"started_at"`
	UpdatedAt  string                 `json:"updated_at"`
	FinishedAt string                 `json:"finished_at,omitempty"`
}

func toNodeMoveResource(m store.NodeMove) nodeMoveResource {
	steps := make([]nodeMoveStepResource, 0, len(m.Steps))
	for _, s := range m.Steps {
		steps = append(steps, nodeMoveStepResource(s))
	}
	vols := m.Volumes
	if vols == nil {
		vols = []string{}
	}
	return nodeMoveResource{
		ID: m.ID, Kind: m.Kind, Resource: m.Resource, FromNodeID: m.FromNodeID, ToNodeID: m.ToNodeID,
		Stateful: m.Stateful, Volumes: vols, Phase: m.Phase, Status: m.Status, Error: m.Error, DrainID: m.DrainID,
		Steps: steps, StartedAt: m.StartedAt, UpdatedAt: m.UpdatedAt, FinishedAt: m.FinishedAt,
	}
}

// moveRequest is the body of POST /apps/{name}/move and /databases/{name}/move.
// An empty node_id lets the spread scheduler pick.
type moveRequest struct {
	NodeID string `json:"node_id"`
}

// handleStartMove starts a verified move: stage on the target, wait for
// readiness, then stop the source (or for stateful resources: stop, copy
// volumes, start on the target), rolling back when the target never
// becomes ready.
func (rt *Router) handleStartMove(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rt.requireMultiNode(w) {
			return
		}
		if rt.execRuntime == nil {
			writeError(w, http.StatusNotImplemented, "node moves need node runtimes, which this control plane does not have")
			return
		}
		name := r.PathValue("name")
		var req moveRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		from, err := rt.currentNode(r.Context(), kind, name)
		if isResourceNotFound(err) {
			writeError(w, http.StatusNotFound, kind+" not found")
			return
		} else if err != nil {
			rt.internalError(w, "api: start move: load failed", err, slog.String("name", name))
			return
		}
		if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown node_id"), nodeRef(req.NodeID)) {
			return
		}
		target := req.NodeID
		if target == "" {
			target, err = rt.spreadTarget(r.Context(), kind, name, from)
		} else {
			err = rt.checkPlacement(r.Context(), kind, name, target)
		}
		if errors.Is(err, placement.ErrNoEligibleNode) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		} else if err != nil {
			rt.internalError(w, "api: start move: choose target failed", err, slog.String("name", name))
			return
		}
		if (rt.moveExec()).sameNode(from, target) {
			writeError(w, http.StatusBadRequest, kind+" is already on that node")
			return
		}
		volumes, err := rt.moveVolumes(r, kind, name)
		if err != nil {
			rt.internalError(w, "api: start move: list volumes failed", err, slog.String("name", name))
			return
		}
		m, err := rt.multiNode.runner.Start(r.Context(), nodemove.Request{Kind: kind, Resource: name, FromNodeID: from, ToNodeID: target, Volumes: volumes})
		switch {
		case errors.Is(err, store.ErrNodeMoveInProgress):
			writeError(w, http.StatusConflict, kind+" is already being moved")
			return
		case nodemove.IsPermanent(err):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		case err != nil:
			rt.internalError(w, "api: start move failed", err, slog.String("name", name))
			return
		}
		rt.logger.Info("api: node move started", slog.String("move_id", m.ID), slog.String("kind", kind), slog.String("name", name),
			slog.String("from_node_id", from), slog.String("to_node_id", target))
		writeJSON(w, http.StatusAccepted, toNodeMoveResource(m))
	}
}

func (rt *Router) moveExec() *moveExecutor { return &moveExecutor{rt: rt} }

func (rt *Router) moveVolumes(r *http.Request, kind, name string) ([]string, error) {
	if kind == store.NodeMoveKindDatabase {
		d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
		if err != nil {
			return nil, err
		}
		return databaseVolumes(*d), nil
	}
	s, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		return nil, err
	}
	return serviceVolumes(*s), nil
}

// handleListNodeMoves handles GET /api/v1/node-moves with optional kind,
// resource, drain_id, status and limit filters.
func (rt *Router) handleListNodeMoves(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	moves, err := rt.multiNode.store.ListNodeMoves(r.Context(), store.NodeMoveFilter{
		Kind: q.Get("kind"), Resource: q.Get("resource"), DrainID: q.Get("drain_id"), Status: q.Get("status"), Limit: limit,
	})
	if err != nil {
		rt.internalError(w, "api: list node moves failed", err)
		return
	}
	out := make([]nodeMoveResource, 0, len(moves))
	for _, m := range moves {
		out = append(out, toNodeMoveResource(m))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetNodeMove handles GET /api/v1/node-moves/{id}.
func (rt *Router) handleGetNodeMove(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	m, err := rt.multiNode.store.GetNodeMove(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNodeMoveNotFound) {
		writeError(w, http.StatusNotFound, "move not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get node move failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toNodeMoveResource(m))
}
