package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BuildRecordReader is what the build record route needs from the store.
type BuildRecordReader interface {
	GetBuildRecordByAttempt(ctx context.Context, attemptID string) (store.BuildRecord, error)
	EnvironmentOfApp(ctx context.Context, appName string) (*store.EnvironmentRef, error)
}

type buildRecordResource struct {
	ID        string          `json:"id"`
	BuildID   string          `json:"build_id"`
	App       string          `json:"app"`
	Claims    json.RawMessage `json:"claims"`
	Signature string          `json:"signature"`
	KeyID     string          `json:"key_id"`
	CreatedAt time.Time       `json:"created_at"`
}

// handleGetBuildRecord handles GET /api/v1/builds/{id}/record, the signed record of a remote build.
func (rt *Router) handleGetBuildRecord(w http.ResponseWriter, r *http.Request) {
	rd, ok := rt.environments.(BuildRecordReader)
	if !ok {
		writeError(w, http.StatusNotImplemented, "build records are not available on this control plane")
		return
	}
	id := r.PathValue("id")
	rec, err := rd.GetBuildRecordByAttempt(r.Context(), id)
	if errors.Is(err, store.ErrBuildRecordNotFound) {
		writeError(w, http.StatusNotFound, "build record not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get build record failed", err)
		return
	}
	env, err := rd.EnvironmentOfApp(r.Context(), rec.App)
	if err != nil {
		rt.internalError(w, "api: build record environment failed", err)
		return
	}
	// A caller without the environment sees the same 404 as a missing record.
	if env != nil && !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "build record not found"), environmentRef(env.ID)) {
		return
	}
	writeJSON(w, http.StatusOK, buildRecordResource{
		ID: rec.ID, BuildID: id, App: rec.App, Claims: json.RawMessage(rec.Claims),
		Signature: rec.Signature, KeyID: rec.KeyID, CreatedAt: rec.CreatedAt,
	})
}
