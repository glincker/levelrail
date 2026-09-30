package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// A container is managed if it carries this platform's instance label
// and its name still has a live app/database record; otherwise orphaned.
func isManagedContainer(c docker.ContainerState, desired map[string]bool) bool {
	if c.Labels[spec.InstanceLabelKey] == "" {
		return false
	}
	return desired[c.Name]
}

func lookupSystemContainer(name string, containers []docker.ContainerState) (docker.ContainerState, bool) {
	for _, c := range containers {
		if c.Name == name {
			return c, true
		}
	}
	return docker.ContainerState{}, false
}

// Re-derives orphaned status server-side rather than trusting the
// client: 404 if no such container, 409 if it's Levelrail-managed.
func (rt *Router) requireOrphanedContainer(w http.ResponseWriter, r *http.Request, name string) (docker.ContainerState, bool) {
	containers, err := rt.containers.ListByPrefix(r.Context(), "")
	if err != nil {
		rt.internalError(w, "api: look up container failed", err)
		return docker.ContainerState{}, false
	}
	c, found := lookupSystemContainer(name, containers)
	if !found {
		writeError(w, http.StatusNotFound, "container not found")
		return docker.ContainerState{}, false
	}
	desired, err := rt.desiredContainerNameSet(r.Context())
	if err != nil {
		rt.internalError(w, "api: compute desired container names failed", err)
		return docker.ContainerState{}, false
	}
	if isManagedContainer(c, desired) {
		writeError(w, http.StatusConflict, "container is managed by Levelrail; manage it from its own app page instead")
		return docker.ContainerState{}, false
	}
	return c, true
}

// Matches every reconciler controller's own defaultStopTimeout.
const orphanedContainerStopTimeout = 10 * time.Second

func (rt *Router) handleStopOrphanedContainer(w http.ResponseWriter, r *http.Request) {
	if rt.containers == nil || rt.orphanedContainers == nil {
		writeError(w, http.StatusNotImplemented, "container management is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	c, ok := rt.requireOrphanedContainer(w, r, name)
	if !ok {
		return
	}
	if err := rt.orphanedContainers.Stop(r.Context(), c.ID, orphanedContainerStopTimeout); err != nil {
		rt.logger.Error("api: stop orphaned container failed", slog.String("error", err.Error()), slog.String("container", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.logger.Info("api: stopped orphaned container", slog.String("container", name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (rt *Router) handleRemoveOrphanedContainer(w http.ResponseWriter, r *http.Request) {
	if rt.containers == nil || rt.orphanedContainers == nil {
		writeError(w, http.StatusNotImplemented, "container management is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	c, ok := rt.requireOrphanedContainer(w, r, name)
	if !ok {
		return
	}
	if err := rt.orphanedContainers.Remove(r.Context(), c.ID, true); err != nil {
		rt.logger.Error("api: remove orphaned container failed", slog.String("error", err.Error()), slog.String("container", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.logger.Info("api: removed orphaned container", slog.String("container", name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// Mirrors web/src/components/CreateAppFields.tsx's own imageSlugFrom.
var appNameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

// Returns "" if nothing usable is left, callers then ask the operator for a name.
func deriveAppNameFromContainer(containerName string) string {
	slug := appNameInvalidChars.ReplaceAllString(strings.ToLower(containerName), "-")
	return strings.Trim(slug, "-")
}

const defaultClaimPort = 80

// The container's own first published port, so it doesn't silently become an app on 80.
func containerClaimPort(c docker.ContainerState) int {
	if len(c.Ports) > 0 && c.Ports[0].ContainerPort > 0 {
		return c.Ports[0].ContainerPort
	}
	return defaultClaimPort
}

type claimOrphanedContainerRequest struct {
	Name string `json:"name"`
}

// Creates a real app (build.type: image) from the container's own image,
// it does not adopt the container's live state. Reuses handleCreateApp
// directly instead of duplicating its validation/secrets/placement logic.
func (rt *Router) handleClaimOrphanedContainer(w http.ResponseWriter, r *http.Request) {
	if rt.containers == nil {
		writeError(w, http.StatusNotImplemented, "container listing is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	c, ok := rt.requireOrphanedContainer(w, r, name)
	if !ok {
		return
	}

	var req claimOrphanedContainerRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // no body, or a malformed one, just falls through to the derived name below
	}
	appName := strings.TrimSpace(req.Name)
	if appName == "" {
		appName = deriveAppNameFromContainer(c.Name)
	}
	if appName == "" {
		writeError(w, http.StatusBadRequest, "could not derive an app name from this container's name; provide one explicitly")
		return
	}

	createBody, err := json.Marshal(appResource{Name: appName, Image: c.Image, Port: containerClaimPort(c)})
	if err != nil {
		rt.internalError(w, "api: claim container: build create app request failed", err, slog.String("container", name))
		return
	}
	createReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/api/v1/apps", bytes.NewReader(createBody))
	if err != nil {
		rt.internalError(w, "api: claim container: build create app request failed", err, slog.String("container", name))
		return
	}
	createReq.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	rt.handleCreateApp(rec, createReq)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rec.Code)
	if _, err := w.Write(rec.Body.Bytes()); err != nil {
		rt.logger.Error("api: claim container: write response failed", slog.String("error", err.Error()), slog.String("container", name))
		return
	}
	if rec.Code != http.StatusCreated {
		return
	}
	rt.logger.Info("api: claimed orphaned container into a new app",
		slog.String("container", name), slog.String("app", appName))
}
