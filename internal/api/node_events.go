package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultNodeEventsLimit = 50
	maxNodeEventsLimit     = 200
)

// nodeStatusEventLister is the optional NodeStore extension that serves
// a node's status transition history; the SQLite store implements it.
type nodeStatusEventLister interface {
	ListNodeStatusEvents(ctx context.Context, nodeID string, limit int) ([]store.NodeStatusEvent, error)
}

type nodeStatusEventResource struct {
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	CreatedAt  time.Time `json:"created_at"`
}

// handleListNodeEvents handles GET /api/v1/nodes/{id}/events: the node's
// recent status transitions (online, offline, cordoned), newest first.
func (rt *Router) handleListNodeEvents(w http.ResponseWriter, r *http.Request) {
	lister, ok := rt.nodes.(nodeStatusEventLister)
	if !ok {
		writeError(w, http.StatusNotImplemented, "node status history is not available on this control plane")
		return
	}

	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list node events: look up node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	limit := defaultNodeEventsLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxNodeEventsLimit {
			writeError(w, http.StatusBadRequest, "limit must be an integer between 1 and "+strconv.Itoa(maxNodeEventsLimit))
			return
		}
		limit = n
	}

	events, err := lister.ListNodeStatusEvents(r.Context(), id, limit)
	if err != nil {
		rt.logger.Error("api: list node events failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}
	out := make([]nodeStatusEventResource, 0, len(events))
	for _, e := range events {
		out = append(out, nodeStatusEventResource{FromStatus: string(e.FromStatus), ToStatus: string(e.ToStatus), CreatedAt: e.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}
