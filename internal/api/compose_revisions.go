package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type composeRevisionResource struct {
	Revision   int      `json:"revision"`
	Source     string   `json:"source"`
	TemplateID string   `json:"template_id,omitempty"`
	Profiles   []string `json:"profiles,omitempty"`
	CreatedBy  string   `json:"created_by,omitempty"`
	CreatedAt  string   `json:"created_at"`
	Compose    string   `json:"compose,omitempty"`
}

func toComposeRevisionResource(rev store.ComposeRevision, withBody bool) composeRevisionResource {
	out := composeRevisionResource{
		Revision: rev.Revision, Source: rev.Source, TemplateID: rev.TemplateID, Profiles: rev.Profiles,
		CreatedBy: rev.CreatedBy, CreatedAt: rev.CreatedAt.UTC().Format(time.RFC3339),
	}
	if withBody {
		out.Compose = rev.Compose
	}
	return out
}

// composeRevisionHistoryLimit bounds the history list, newest first.
const composeRevisionHistoryLimit = 50

// handleListComposeRevisions handles GET /api/v1/apps/{name}/compose/revisions.
func (rt *Router) handleListComposeRevisions(w http.ResponseWriter, r *http.Request) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		writeError(w, http.StatusNotImplemented, "compose history is not available on this store")
		return
	}
	appID := r.PathValue("name")
	list, err := revs.ListComposeRevisions(r.Context(), appID, composeRevisionHistoryLimit)
	if err != nil {
		rt.internalError(w, "api: compose: list revisions failed", err, slog.String("app", appID))
		return
	}
	out := make([]composeRevisionResource, 0, len(list))
	for _, rev := range list {
		out = append(out, toComposeRevisionResource(rev, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "revisions": out})
}

// handleGetComposeRevision handles GET /api/v1/apps/{name}/compose/revisions/{revision}.
func (rt *Router) handleGetComposeRevision(w http.ResponseWriter, r *http.Request) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		writeError(w, http.StatusNotImplemented, "compose history is not available on this store")
		return
	}
	n, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil || n <= 0 {
		writeError(w, http.StatusBadRequest, "revision must be a positive integer")
		return
	}
	appID := r.PathValue("name")
	rev, err := revs.GetComposeRevision(r.Context(), appID, n)
	if errors.Is(err, store.ErrComposeRevisionNotFound) {
		writeError(w, http.StatusNotFound, "compose revision not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: compose: get revision failed", err, slog.String("app", appID))
		return
	}
	writeJSON(w, http.StatusOK, toComposeRevisionResource(rev, true))
}

type composeRollbackRequest struct {
	Revision int `json:"revision,omitempty"`
}

// handleRollbackCompose handles POST /api/v1/apps/{name}/compose/rollback:
// re-applies an earlier compose file (default: the one before the latest)
// through the normal deploy path, recorded as a new revision.
func (rt *Router) handleRollbackCompose(w http.ResponseWriter, r *http.Request) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		writeError(w, http.StatusNotImplemented, "compose history is not available on this store")
		return
	}
	var req composeRollbackRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	appID := r.PathValue("name")
	latest, err := revs.GetComposeRevision(r.Context(), appID, 0)
	if errors.Is(err, store.ErrComposeRevisionNotFound) {
		writeError(w, http.StatusNotFound, "this app has no compose history to roll back to")
		return
	}
	if err != nil {
		rt.internalError(w, "api: compose: rollback: load latest failed", err, slog.String("app", appID))
		return
	}
	target := req.Revision
	if target == 0 {
		target = latest.Revision - 1
	}
	if target <= 0 || target == latest.Revision {
		writeError(w, http.StatusConflict, "no earlier compose revision to roll back to")
		return
	}
	rev, err := revs.GetComposeRevision(r.Context(), appID, target)
	if errors.Is(err, store.ErrComposeRevisionNotFound) {
		writeError(w, http.StatusNotFound, "compose revision not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: compose: rollback: load target failed", err, slog.String("app", appID))
		return
	}
	resp, ok := rt.deployComposeBody(w, r, appID, []byte(rev.Compose), composeDeployOpts{
		profiles: rev.Profiles, source: store.ComposeRevisionSourceRollback, templateID: rev.TemplateID,
	})
	if !ok {
		return
	}
	rt.logger.Info("api: compose: rolled back", slog.String("app", appID), slog.Int("from", latest.Revision), slog.Int("to", target), slog.Int("revision", resp.Revision))
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "rolled_back_to": target, "deploy": resp})
}
