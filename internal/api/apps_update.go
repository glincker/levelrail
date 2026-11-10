package api

import (
	"errors"
	"log/slog"
	"maps"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appUpdateResponse adds the build-time env hint to the saved app; the
// rebuild itself stays an explicit user action.
type appUpdateResponse struct {
	appResource
	RebuildRequired bool     `json:"rebuild_required,omitempty"`
	RebuildKeys     []string `json:"rebuild_keys,omitempty"`
}

// handleUpdateApp handles PUT /api/v1/apps/{name}. A field the body omits
// keeps its stored value; an explicit empty value or null clears it.
func (rt *Router) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.refuseIfProtected(w, r, name) {
		return
	}

	var req appResource
	sent, ok := decodeAppUpdate(w, r, &req)
	if !ok {
		return
	}
	req.Name = name // the path is authoritative, not whatever the body claims
	if len(req.Secrets) > 0 {
		writeError(w, http.StatusBadRequest, "secret values cannot be set through this endpoint: use PUT /api/v1/apps/{name}/secrets/{key}")
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: update app: check existing failed", err, slog.String("name", name))
		return
	}
	mergeOmittedAppFields(&req, *existing, sent)

	if err := validateAppUpdate(req, *existing); err != nil {
		writeAppInvalid(w, err)
		return
	}
	if err := rt.checkReplicaCap(r.Context(), req.Replicas); err != nil {
		writeAppInvalid(w, err)
		return
	}
	if !rt.requireWildcardDNS01(w, r, req.Domains) {
		return
	}

	if eerr := rt.hostPortConflict(r.Context(), name, existing.NodeID, req.HostPort, existing); eerr != nil {
		writeError(w, eerr.status, eerr.msg)
		return
	}

	if !rt.keepAutoscaledReplicas(w, r, name, existing, &req) {
		return
	}

	// imageChanged also makes this PUT a real redeploy (see
	// recordPlainDeployAttempt below), so it clears EnvDirty the same way
	// RestartService/handleTriggerDeploy do; otherwise EnvDirty latches
	// true the moment Env changes and stays true across an unrelated
	// save (e.g. a health check edit sent via the same PUT).
	if !rt.mergeEnvPlaceholders(w, &req, existing) {
		return
	}
	imageChanged := req.Image != existing.Image
	desired := req.toDesiredService()
	appID := existing.AppID
	if appID == "" {
		appID = name
	}
	portsRelease, portsOK := rt.admitPublishedPorts(w, r, appID, []store.DesiredService{*existing}, []store.DesiredService{desired})
	if !portsOK {
		return
	}
	defer portsRelease()
	quotaRelease, quotaOK := rt.admitAppResizeQuota(w, r, *existing, desired)
	if !quotaOK {
		return
	}
	defer quotaRelease()
	// Egress isn't in appResource.toDesiredService() (that field's own
	// doc comment), so without this it would silently reset to nil on
	// every save: SaveDesiredService, unlike NodeID/StorageTargetID,
	// always writes this column.
	desired.Egress = existing.Egress
	preserveUnsentAppFields(&desired, *existing, req, imageChanged)
	deploy.EnsureNextCacheLink(&desired)
	deploy.EnsureNextCacheCALink(r.Context(), &desired, rt.databases)
	if imageChanged {
		desired.EnvDirty = false
	} else {
		desired.EnvDirty = existing.EnvDirty || !maps.Equal(existing.Env, req.Env)
	}
	if err := rt.apps.SaveDesiredService(r.Context(), desired); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.internalError(w, "api: update app failed", err, slog.String("name", name))
		return
	}
	// An image change here is functionally the same redeploy action as
	// handleTriggerDeploy, which already records one: an ordinary PUT
	// that happens to change Image must not be a blind spot in deploy
	// history just because it went through this endpoint instead.
	if imageChanged {
		rt.recordPlainDeployAttempt(r.Context(), desired, req.Image)
	}
	rt.recordEnvChanges(r, name, existing.Env, desired.Env, envChangeSourceAPI)
	for _, ev := range updateEvents(name, *existing, desired) {
		rt.recordAppEvent(r, ev)
	}
	// SaveDesiredService never touches node_id or project_id (their own
	// doc comments explain why), so the response reflects existing's
	// placement and project, not req's: req.NodeID/req.ProjectID are
	// always their zero value here anyway (the client never has a way to
	// set either on this endpoint, unlike handleCreateApp's own
	// deliberate ProjectID exception), but spelling out "carry the real,
	// unchanged value forward" is clearer than relying on that zero
	// value happening to look right.
	req.NodeID = existing.NodeID
	req.ProjectID = existing.ProjectID
	req.SecretEnv = store.SecretEnvNames(desired.SecretEnv)
	req.VaultEnv = toAppResource(desired).VaultEnv
	// Re-fetch rather than reusing req.toDesiredService(): NodeID and
	// RestartNonce (both needed to compute the real running container's
	// name, desiredServiceContainerNames) are response-only fields this
	// endpoint's request body never carries, the same boundary NodeID's
	// own doc comment establishes.
	if saved, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		req.ResourcesAppliedLive = rt.applyResourcesLiveToReplicas(r.Context(), *saved)
		req.EnvDirty = saved.EnvDirty
	}
	rt.redactAppResponse(r.Context(), &req, desired)
	keys := buildTimeEnvChanges(existing.Env, desired.Env)
	rt.changes.Publish(changeApps, name)
	writeJSON(w, http.StatusOK, appUpdateResponse{appResource: req, RebuildRequired: len(keys) > 0, RebuildKeys: keys})
}
