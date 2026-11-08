package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// handleCreateApp handles POST /api/v1/apps. A duplicate name or a taken
// domain is a 409 (PUT is the update path). ProjectID is accepted only here
// and written via UpdateServiceProject, the single writer of project_id.
func (rt *Router) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDecodeBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req appResource
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateAppResource(req); err != nil {
		writeAppInvalid(w, err)
		return
	}
	if err := rt.checkReplicaCap(r.Context(), req.Replicas); err != nil {
		writeAppInvalid(w, err)
		return
	}
	// Create only: apps that predate this rule keep updating under their old names.
	if err := spec.ValidateServiceName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !rt.requireWildcardDNS01(w, r, req.Domains) {
		return
	}
	if _, err := envredact.Merge(req.Env, nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	autoDomain := rt.attachAppsBaseDomain(r.Context(), &req)
	if len(req.Secrets) > 0 && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	req.ProjectID = defaultProjectFor(r, req.ProjectID)
	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.internalError(w, "api: create app: check project failed", err, slog.String("project_id", req.ProjectID))
		return
	}

	defaultEnvID, envOK := rt.defaultEnvironmentFor(w, r, req.ProjectID)
	if !envOK {
		return
	}

	// node_id present in the body (even "") is an explicit placement
	// override, validated the same way handleSetAppNode validates one.
	// Omitted entirely lets simple spread scheduling (autoPlaceNode) pick
	// a node when more than one is registered; AutoPlaced only turns
	// true when that pick actually lands somewhere other than local. A
	// domain-bearing app prefers the control plane's own node, since
	// ingress has no mesh path to a worker node yet (CrossNodeIngress).
	cp := &createPlacement{Kind: store.NodeMoveKindApp, Name: req.Name, Needs: serviceRequests(req.Resources, req.Replicas), Rules: placementRules{Tolerations: req.Tolerations, Affinity: normalizeAffinity(req.Affinity)}}
	if !rt.resolveCreateNodePlacement(w, r, body, &req.NodeID, &req.AutoPlaced, len(req.Domains) > 0, cp, "api: create app") {
		return
	}
	if !rt.settleGPUCreatePlacement(w, r, body, &req) {
		return
	}

	_, err = rt.apps.GetDesiredService(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "an app with this name already exists")
		return
	}
	if eerr := rt.hostPortConflict(r.Context(), req.Name, req.NodeID, req.HostPort, nil); eerr != nil {
		writeError(w, eerr.status, eerr.msg)
		return
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: create app: check existing failed", err, slog.String("name", req.Name))
		return
	}

	desired := req.toDesiredService()
	// Required: false for every entry here, since this endpoint has no
	// { required: true } concept of its own to carry over (that only
	// ever comes from app.yaml, via internal/deploy's translate.go); the
	// same permissive default specServiceFromDesired's own doc comment
	// already documents for a plain manual build.
	desired.SecretEnv = store.SecretEnvRefsFromNames(unionSecretEnvNames(req.SecretEnv, req.Secrets))
	if len(req.VaultEnv) > 0 {
		desired.VaultEnv = make(map[string]store.VaultEnvRef, len(req.VaultEnv))
		for k, v := range req.VaultEnv {
			desired.VaultEnv[k] = store.VaultEnvRef{Path: v.Path, Key: v.Key}
		}
	}
	// Before admission: a registry round trip must not hold the port lock.
	resolution, createTrace := rt.resolveCreateImage(r.Context(), &desired)

	admitted := req.toDesiredService()
	admitted.NodeID = req.NodeID
	portsRelease, portsOK := rt.admitPublishedPorts(w, r, req.Name, nil, []store.DesiredService{admitted})
	if !portsOK {
		return
	}
	defer portsRelease()

	appReserve := serviceReservation(req.Resources, req.Replicas)
	quotaRelease, quotaOK := rt.admitQuotaOrWrite(w, r, req.ProjectID, store.ResourceOwner{},
		quota.Amounts{Apps: 1, MemoryBytes: appReserve.MemoryBytes, NanoCPUs: appReserve.NanoCPUs})
	if !quotaOK {
		return
	}
	defer quotaRelease()

	// Secret values are stored before SaveDesiredService, not after: the
	// reconciler converges as soon as the desired state exists, so a
	// required secret must already be resolvable by then rather than
	// leaving a window where the app exists but its secret doesn't yet.
	// internal/store's service_secret_values table has no foreign key to
	// desired_services, so this ordering is safe even though the service
	// row below doesn't exist yet.
	unlockName := lockAppName(req.Name)
	defer unlockName()
	for key, value := range req.Secrets {
		if err := rt.secrets.SetValueGuarded(r.Context(), req.Name, key, value, false); err != nil {
			if errors.Is(err, secrets.ErrSecretLocked) {
				writeError(w, http.StatusConflict, fmt.Sprintf("secret %q is locked", key))
				return
			}
			rt.internalError(w, "api: create app: set secret failed", err, slog.String("name", req.Name), slog.String("key", key))
			return
		}
	}

	if err := rt.apps.SaveDesiredService(r.Context(), desired); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.internalError(w, "api: create app failed", err, slog.String("name", req.Name))
		return
	}

	if req.ProjectID != "" {
		if err := rt.apps.UpdateServiceProject(r.Context(), req.Name, req.ProjectID); err != nil {
			rt.internalError(w, "api: create app: assign project failed", err, slog.String("name", req.Name), slog.String("project_id", req.ProjectID))
			return
		}
	}

	if defaultEnvID != "" {
		if err := rt.environments.SetServiceEnvironment(r.Context(), req.Name, defaultEnvID); err != nil {
			rt.internalError(w, "api: create app: assign default environment failed", err, slog.String("name", req.Name), slog.String("environment_id", defaultEnvID))
			return
		}
	}

	// SaveDesiredService never writes node_id (see toDesiredService above),
	// so an explicit or auto-placed non-local req.NodeID needs this
	// trailing call, the same pattern UpdateServiceProject just above
	// already establishes for ProjectID.
	if req.NodeID != "" {
		if err := rt.apps.UpdateServiceNode(r.Context(), req.Name, req.NodeID); err != nil {
			rt.internalError(w, "api: create app: assign node failed", err, slog.String("name", req.Name), slog.String("node_id", req.NodeID))
			return
		}
		rt.recordCreatePlacement(r.Context(), cp, req.NodeID)
	}

	// Every app gets a store.App row, single-service or not: see
	// ensureAppLinked's own doc comment for why this must not be
	// special-cased away for the common single-service case.
	appID, err := rt.ensureAppLinked(r.Context(), req.Name, req.Name)
	if err != nil {
		rt.internalError(w, "api: create app: link to app row failed", err, slog.String("name", req.Name))
		return
	}
	req.AppID = appID
	unlockName()

	// Skip the git-build path's ":pending" placeholder (cmd/levelrail-cli's
	// pendingImageTag): its own POST .../builds call records the real
	// history entry once a build actually succeeds.
	if !spec.IsPendingImage(req.Image) {
		rt.recordResolvedDeployAttempt(r.Context(), desired, desired.Image, store.DeployAttemptSourceImage, resolution, createTrace)
	}
	req.Image = desired.Image
	req.ImageDigest = appImageDigest(desired)

	// A new app is never dirty regardless of what the client sent:
	// toDesiredService never carries EnvDirty into the INSERT (it's
	// response-only), so echoing anything but false here would lie about
	// what was actually persisted.
	req.EnvDirty = false
	// SecretEnv echoes back the union actually stored (declared names
	// plus any inline value's own key); Secrets never round-trips a
	// value, matching PUT .../secrets/{key}'s own "never echo it back"
	// rule.
	req.SecretEnv = store.SecretEnvNames(desired.SecretEnv)
	req.Secrets = nil
	// VaultEnv echoes back exactly what was stored: unlike Secrets, this
	// never carried a value in the first place, so there is nothing to
	// scrub before echoing it.
	req.VaultEnv = toAppResource(desired).VaultEnv

	if autoDomain != "" {
		req.AutoDomain = autoDomain
		req.DNS = rt.dnsForAutoDomain(r.Context(), r, req.Name, autoDomain)
	}
	rt.redactAppResponse(r.Context(), &req, desired)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusCreated, req)
}

// resolveCreateImage pins desired's image the way a deploy does (registry
// digest, or the local image ID when only a local image exists) and returns
// the resolution to record on the first deploy attempt. A resolver failure
// leaves the image as given: creating an app must not depend on a registry.
func (rt *Router) resolveCreateImage(ctx context.Context, desired *store.DesiredService) (deploy.ImageResolution, *deploytrace.Tracer) {
	res := deploy.ImageResolution{Image: desired.Image}
	if spec.IsPendingImage(desired.Image) {
		return res, nil
	}
	tr := deploytrace.NewBuffered()
	got, err := deploy.ResolveImageTraced(ctx, tr, rt.imageResolver, desired.Image, nil, false, "")
	if err != nil {
		rt.logger.Warn("api: create app: resolve image failed, deploying it unresolved", slog.String("error", err.Error()), slog.String("name", desired.Name))
		return res, tr
	}
	desired.Image = got.Image
	desired.ImageID, desired.ImageIDRef = "", ""
	if got.LocalID != "" && docker.ImageDigestOf(got.Image) == "" {
		desired.ImageID, desired.ImageIDRef = got.LocalID, got.Image
	}
	return got, tr
}
