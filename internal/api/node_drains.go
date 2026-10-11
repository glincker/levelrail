package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

type nodeDrainResource struct {
	ID           string                   `json:"id"`
	NodeID       string                   `json:"node_id"`
	TargetNodeID string                   `json:"target_node_id,omitempty"`
	Status       string                   `json:"status"`
	Error        string                   `json:"error,omitempty"`
	Blocked      []store.NodeDrainBlocked `json:"blocked"`
	Moves        []nodeMoveResource       `json:"moves"`
	StartedAt    string                   `json:"started_at"`
	UpdatedAt    string                   `json:"updated_at"`
	FinishedAt   string                   `json:"finished_at,omitempty"`
}

func (rt *Router) drainResource(r *http.Request, d store.NodeDrain) (nodeDrainResource, error) {
	moves, err := rt.multiNode.store.ListNodeMoves(r.Context(), store.NodeMoveFilter{DrainID: d.ID})
	if err != nil {
		return nodeDrainResource{}, err
	}
	out := nodeDrainResource{
		ID: d.ID, NodeID: d.NodeID, TargetNodeID: d.TargetNodeID, Status: d.Status, Error: d.Error,
		Blocked: d.Blocked, Moves: make([]nodeMoveResource, 0, len(moves)),
		StartedAt: d.StartedAt, UpdatedAt: d.UpdatedAt, FinishedAt: d.FinishedAt,
	}
	if out.Blocked == nil {
		out.Blocked = []store.NodeDrainBlocked{}
	}
	for i := len(moves) - 1; i >= 0; i-- {
		out.Moves = append(out.Moves, toNodeMoveResource(moves[i]))
	}
	return out, nil
}

// handleStartDrain is POST /api/v1/nodes/{id}/drain with multi-node
// enabled: cordon, then move databases and then apps off one at a time,
// each through a verified move. Returns 202 with the drain to poll.
func (rt *Router) handleStartDrain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target := r.URL.Query().Get("target_node_id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: drain node: look up node failed", err, slog.String("node_id", id))
		return
	}
	if target != "" {
		if target == id {
			writeError(w, http.StatusBadRequest, "target_node_id must be a different node")
			return
		}
		cands, err := rt.placementCandidates(r.Context())
		if err != nil {
			rt.internalError(w, "api: drain node: list candidates failed", err, slog.String("node_id", id))
			return
		}
		found := false
		for _, c := range cands {
			if c.ID == target {
				found = true
				if err := placement.Check(c, placement.Request{}); err != nil {
					writeError(w, http.StatusBadRequest, "target_node_id: "+err.Error())
					return
				}
			}
		}
		if !found {
			writeError(w, http.StatusBadRequest, "unknown target_node_id")
			return
		}
	}
	d, err := rt.multiNode.runner.StartDrain(r.Context(), id, target)
	if errors.Is(err, store.ErrNodeMoveInProgress) {
		writeError(w, http.StatusConflict, "node is already draining")
		return
	} else if err != nil {
		rt.internalError(w, "api: start drain failed", err, slog.String("node_id", id))
		return
	}
	rt.logger.Info("api: node drain started", slog.String("drain_id", d.ID), slog.String("node_id", id), slog.String("target_node_id", target))
	res, err := rt.drainResource(r, d)
	if err != nil {
		rt.internalError(w, "api: start drain: load moves failed", err, slog.String("drain_id", d.ID))
		return
	}
	writeJSON(w, http.StatusAccepted, res)
}

// handleListNodeDrains handles GET /api/v1/nodes/{id}/drains, newest first.
func (rt *Router) handleListNodeDrains(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	drains, err := rt.multiNode.store.ListNodeDrains(r.Context(), r.PathValue("id"), 20)
	if err != nil {
		rt.internalError(w, "api: list node drains failed", err)
		return
	}
	out := make([]nodeDrainResource, 0, len(drains))
	for _, d := range drains {
		res, err := rt.drainResource(r, d)
		if err != nil {
			rt.internalError(w, "api: list node drains: load moves failed", err, slog.String("drain_id", d.ID))
			return
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetNodeDrain handles GET /api/v1/node-drains/{id}.
func (rt *Router) handleGetNodeDrain(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMultiNode(w) {
		return
	}
	d, err := rt.multiNode.store.GetNodeDrain(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNodeMoveNotFound) {
		writeError(w, http.StatusNotFound, "drain not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get node drain failed", err)
		return
	}
	res, err := rt.drainResource(r, d)
	if err != nil {
		rt.internalError(w, "api: get node drain: load moves failed", err, slog.String("drain_id", d.ID))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
