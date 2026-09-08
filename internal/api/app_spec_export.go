package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"gopkg.in/yaml.v3"
)

// handleExportAppSpec handles GET /api/v1/apps/{name}/spec: reconstructs
// name's already-stored desired state (store.DesiredService) as an
// app.yaml document, via the same specServiceFromDesired reverse-mapping
// POST /api/v1/apps/{name}/builds already uses (builds.go). AbilityRead:
// this discloses nothing a caller couldn't already see field-by-field
// through GET /api/v1/apps/{name}, only reshapes it into the spec.Spec
// YAML document form.
//
// The build: block is always the zero value: store.DesiredService has no
// column for build type/path/repo, so there is nothing to reconstruct it
// from (same gap specServiceFromDesired's own doc comment names for a
// manual build trigger). One consequence worth calling out: an empty
// build.type fails the app spec's own JSON Schema (build.type is a
// required enum), so this document will not round-trip through
// spec.Parse until a caller fills in a real build: block; it is meant to
// be read or hand-edited before redeploying, not reparsed as-is. Two
// further fidelity losses carry over from specServiceFromDesired
// unchanged: a secret env var's original { required: true } flag is
// lost (reconstructed as not required), and any { from: ... }
// cross-resource reference is unrecoverable, since store.DesiredService
// only ever persists the already-resolved literal value.
func (rt *Router) handleExportAppSpec(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: export app spec: get service failed", err, slog.String("name", name))
		return
	}

	svcSpec := specServiceFromDesired(*svc, spec.Build{})
	out := spec.Spec{Version: 1, Services: map[string]spec.Service{name: svcSpec}}

	data, err := yaml.Marshal(out)
	if err != nil {
		rt.internalError(w, "api: export app spec: marshal yaml failed", err, slog.String("name", name))
		return
	}

	w.Header().Set("Content-Type", "text/yaml")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		rt.logger.Warn("api: export app spec: write response failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}
