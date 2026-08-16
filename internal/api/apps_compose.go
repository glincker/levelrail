package api

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// composeDeployResponse is POST /api/v1/apps/{name}/compose's response
// shape: the services a compose.yaml fanned out into, reusing
// appResource so it round-trips through the same shape every other app
// read/write endpoint already uses.
type composeDeployResponse struct {
	AppID    string        `json:"app_id"`
	Services []appResource `json:"services"`
}

// handleDeployCompose handles POST /api/v1/apps/{name}/compose: the
// request body is a compose.yaml document, name is the store.App it
// becomes (App.ID == App.Name, matching how migrations/0039_apps.sql's
// own backfill treats a single-service app's ID). Each resulting
// service is saved directly via SaveDesiredService, the same path
// POST /api/v1/apps (a pre-built image, no build step) already uses,
// since every compose service here already carries a resolved image.
func (rt *Router) handleDeployCompose(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	file, err := compose.Parse(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	services, err := compose.ToDesiredServices(name, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := rt.appCompose.SaveApp(r.Context(), store.App{ID: name, Name: name, CreatedAt: now, UpdatedAt: now}); err != nil {
		rt.internalError(w, "api: deploy compose: save app failed", err, slog.String("name", name))
		return
	}

	out := make([]appResource, 0, len(services))
	for _, svc := range services {
		if err := rt.appCompose.SaveDesiredService(r.Context(), svc); err != nil {
			rt.internalError(w, "api: deploy compose: save service failed", err, slog.String("service", svc.Name))
			return
		}
		out = append(out, toAppResource(svc))
	}

	writeJSON(w, http.StatusOK, composeDeployResponse{AppID: name, Services: out})
}
