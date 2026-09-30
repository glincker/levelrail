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

// isManagedContainer reports whether c is a Levelrail-managed container:
// it carries this platform's own instance label (spec.InstanceLabelKey,
// stamped on every container any control-plane instance creates) and its
// name still matches a real app or database record in desired (the
// desiredContainerNameSet an app's row produces). A container with no
// such label was never created by this platform; one that has the label
// but whose name has fallen out of desired is a Levelrail container
// whose owning app or database row was deleted without the container
// itself being cleaned up. Either way, false means orphaned: no
// reconciler is converging it, so the stop/remove/claim routes below are
// safe to act on it directly.
func isManagedContainer(c docker.ContainerState, desired map[string]bool) bool {
	if c.Labels[spec.InstanceLabelKey] == "" {
		return false
	}
	return desired[c.Name]
}

// lookupSystemContainer finds one container by its exact name among
// containers (docker.ContainerState.Name, an exact match, not
// ListByPrefix's own prefix semantics).
func lookupSystemContainer(name string, containers []docker.ContainerState) (docker.ContainerState, bool) {
	for _, c := range containers {
		if c.Name == name {
			return c, true
		}
	}
	return docker.ContainerState{}, false
}

// requireOrphanedContainer resolves the container named by the
// {name} path value and confirms it is genuinely orphaned (not
// isManagedContainer), writing the appropriate error response and
// returning ok=false otherwise: 404 if no such container exists, 409 if
// it's Levelrail-managed. Every mutating route below calls this rather
// than trusting a client-supplied "is this orphaned" flag, so a stale or
// forged request can never stop, remove, or claim a container the
// reconciler still owns.
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

// orphanedContainerStopTimeout mirrors every reconciler controller's own
// defaultStopTimeout (e.g. internal/reconcile/database's), so a manual
// stop through this route waits the same grace period a reconciler-
// driven one already would.
const orphanedContainerStopTimeout = 10 * time.Second

// handleStopOrphanedContainer handles POST
// /api/v1/system/containers/{name}/stop: stops (does not remove) one
// orphaned container. 501 if no OrphanedContainerManager is configured.
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

// handleRemoveOrphanedContainer handles POST
// /api/v1/system/containers/{name}/remove: stops and removes one
// orphaned container (Remove's force=true stops it first if still
// running, the same Engine API behavior Runtime.Remove's own doc
// comment documents). 501 if no OrphanedContainerManager is configured.
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

// appNameInvalidChars matches everything a derived app name may not
// contain, mirroring web/src/components/CreateAppFields.tsx's own
// imageSlugFrom: lowercase, [a-z0-9-] only.
var appNameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

// deriveAppNameFromContainer turns an orphaned container's own name into
// a candidate app name: lowercased, non [a-z0-9-] runs collapsed to a
// single dash, leading/trailing dashes trimmed. Returns "" if nothing
// usable is left, which callers treat as "ask the operator for a name."
func deriveAppNameFromContainer(containerName string) string {
	slug := appNameInvalidChars.ReplaceAllString(strings.ToLower(containerName), "-")
	return strings.Trim(slug, "-")
}

// defaultClaimPort is used when an orphaned container reports no port
// binding at all, so validateAppResource's port>0 requirement is always
// satisfiable; the operator can still change it on the app afterward.
const defaultClaimPort = 80

// containerClaimPort picks the port a claimed app deploys with: the
// orphaned container's own first published container port, so a
// container that actually serves traffic on, say, 3000 doesn't silently
// become an app listening on 80. Falls back to defaultClaimPort when the
// container published nothing (e.g. it isn't running right now).
func containerClaimPort(c docker.ContainerState) int {
	if len(c.Ports) > 0 && c.Ports[0].ContainerPort > 0 {
		return c.Ports[0].ContainerPort
	}
	return defaultClaimPort
}

// claimOrphanedContainerRequest is POST
// /api/v1/system/containers/{name}/claim's body: an optional app name
// override. Omitted (or blank), the derived name from
// deriveAppNameFromContainer is used instead.
type claimOrphanedContainerRequest struct {
	Name string `json:"name"`
}

// handleClaimOrphanedContainer handles POST
// /api/v1/system/containers/{name}/claim: creates a real Levelrail app
// (store.DesiredService row) with Image set to the orphaned container's
// own image, build.type: image's exact shape (a prebuilt image already
// in a registry, internal/spec.go), not an attempt to adopt the
// container's live process or filesystem state. The reconciler creates
// its own fresh container for the new app on its first reconcile pass;
// this handler never touches the orphaned container itself.
//
// Reuses handleCreateApp directly via a constructed *http.Request rather
// than duplicating its validation, secret-handling, node-placement, and
// desired-state-save logic: this endpoint's only real job is deriving
// the right appResource from an orphaned container, not reimplementing
// app creation.
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
