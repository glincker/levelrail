package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// previewExcludeStore is the optional store surface for per-variable preview scope.
type previewExcludeStore interface {
	SetPreviewEnvExclude(ctx context.Context, name, key string, exclude bool) error
	ListPreviewEnvExcludes(ctx context.Context, name string) ([]string, error)
}

// handleSetAppPreviewEnvExclude handles PUT and DELETE on preview-env-exclude/{key}:
// keeps one env var out of new preview environments, or puts it back.
func (rt *Router) handleSetAppPreviewEnvExclude(exclude bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, key := r.PathValue("name"), r.PathValue("key")
		st, ok := rt.apps.(previewExcludeStore)
		if key == "" || !ok {
			writeError(w, http.StatusBadRequest, "env var key is required")
			return
		}
		err := st.SetPreviewEnvExclude(r.Context(), name, key, exclude)
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		if err != nil {
			rt.internalError(w, "api: set preview env exclude failed", err, slog.String("name", name))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "excluded": exclude})
	}
}

// previewEnvExcludes lists the keys name keeps out of previews; empty when unsupported.
func (rt *Router) previewEnvExcludes(ctx context.Context, name string) []string {
	st, ok := rt.apps.(previewExcludeStore)
	if !ok {
		return nil
	}
	keys, err := st.ListPreviewEnvExcludes(ctx, name)
	if err != nil {
		rt.logger.Warn("api: load preview env excludes failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	return keys
}
