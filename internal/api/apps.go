package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// appResource is the wire shape for an app: store.DesiredService plus
// its name, marshaled and unmarshaled directly rather than through a
// parallel domain type, since TASKS.md 1.9 doesn't ask this endpoint to
// represent anything store.DesiredService can't already hold. Replicas
// and strategy still have no home in store.DesiredService itself (that's
// a store-schema change, not an API one), so this resource still can't
// represent everything app.yaml (internal/spec) can express; domains
// closed once TASKS.md 1.6 added the column.
type appResource struct {
	Name      string                  `json:"name"`
	Image     string                  `json:"image"`
	Port      int                     `json:"port"`
	Domains   []string                `json:"domains,omitempty"`
	Env       map[string]string       `json:"env,omitempty"`
	Resources *store.ServiceResources `json:"resources,omitempty"`
	Health    *store.ServiceHealth    `json:"health,omitempty"`
}

func toAppResource(svc store.DesiredService) appResource {
	return appResource{
		Name:      svc.Name,
		Image:     svc.Image,
		Port:      svc.Port,
		Domains:   svc.Domains,
		Env:       svc.Env,
		Resources: svc.Resources,
		Health:    svc.Health,
	}
}

func (a appResource) toDesiredService() store.DesiredService {
	return store.DesiredService{
		Name:      a.Name,
		Image:     a.Image,
		Port:      a.Port,
		Domains:   a.Domains,
		Env:       a.Env,
		Resources: a.Resources,
		Health:    a.Health,
	}
}

func validateAppResource(a appResource) error {
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.Image == "" {
		return errors.New("image is required")
	}
	if a.Port <= 0 {
		return errors.New("port must be a positive integer")
	}
	return nil
}

// handleListApps handles GET /api/v1/apps.
func (rt *Router) handleListApps(w http.ResponseWriter, r *http.Request) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.logger.Error("api: list apps failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]appResource, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, toAppResource(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateApp handles POST /api/v1/apps. Rejects a name that already
// exists rather than silently overwriting it: that's what PUT
// (handleUpdateApp) is for.
func (rt *Router) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req appResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateAppResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err := rt.apps.GetDesiredService(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "an app with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: create app: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.SaveDesiredService(r.Context(), req.toDesiredService()); err != nil {
		rt.logger.Error("api: create app failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

// handleGetApp handles GET /api/v1/apps/{name}.
func (rt *Router) handleGetApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAppResource(*svc))
}

// handleUpdateApp handles PUT /api/v1/apps/{name}. Full replace, same as
// SaveDesiredService itself: there is no partial-update semantics here,
// matching how store.DesiredService is already documented to work.
func (rt *Router) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req appResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = name // the path is authoritative, not whatever the body claims

	if err := validateAppResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update app: check existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.SaveDesiredService(r.Context(), req.toDesiredService()); err != nil {
		rt.logger.Error("api: update app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// handleDeleteApp handles DELETE /api/v1/apps/{name}. See
// store.DeleteDesiredService's doc comment for the known gap: this
// removes desired state, it does not itself stop the running container.
func (rt *Router) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	err := rt.apps.DeleteDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
