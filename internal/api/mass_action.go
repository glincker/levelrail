package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

const defaultBulkConfirmAbove = 10

// bulkNeedsCountConfirm reports whether a disruptive bulk action over n apps must carry confirm_count.
// Delete has its own stricter confirm_names rule, so it is not repeated here.
func bulkNeedsCountConfirm(action string, n int) bool {
	switch action {
	case bulkActionRedeploy, bulkActionRestart, bulkActionStop:
		return n > envIntDefault("APP_BULK_CONFIRM_ABOVE", defaultBulkConfirmAbove)
	default:
		return false
	}
}

func bulkCountConfirmMessage(action string, n int) string {
	return fmt.Sprintf("%s of %d apps needs confirm_count: %d (run it with dry_run first to see the targets)", action, n, n)
}

type drainPreviewResponse struct {
	DryRun    bool     `json:"dry_run"`
	NodeID    string   `json:"node_id"`
	Services  []string `json:"services"`
	Databases []string `json:"databases"`
}

// previewNodeDrain lists what a drain would move without cordoning or moving anything.
func (rt *Router) previewNodeDrain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: drain preview: look up node failed", err, slog.String("node_id", id))
		return
	}
	services, err := rt.apps.ListDesiredServicesByNode(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: drain preview: list services failed", err, slog.String("node_id", id))
		return
	}
	databases, err := rt.databases.ListDesiredDatabasesByNode(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: drain preview: list databases failed", err, slog.String("node_id", id))
		return
	}
	out := drainPreviewResponse{DryRun: true, NodeID: id, Services: make([]string, 0, len(services)), Databases: make([]string, 0, len(databases))}
	for _, s := range services {
		out.Services = append(out.Services, s.Name)
	}
	for _, d := range databases {
		out.Databases = append(out.Databases, d.Name)
	}
	writeJSON(w, http.StatusOK, out)
}
