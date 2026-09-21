package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// composeDeployResponse is POST /api/v1/apps/{name}/compose's response
// shape: the services a compose.yaml fanned out into, reusing
// appResource so it round-trips through the same shape every other app
// read/write endpoint already uses.
type composeDeployResponse struct {
	AppID    string                `json:"app_id"`
	Services []appResource         `json:"services"`
	Notices  []composeNoticeResult `json:"notices,omitempty"`
}

// composeNoticeResult is the wire shape of compose.Notice: a compose
// keyword that parsed but doesn't behave the way it would under real
// Docker Compose, surfaced to the operator rather than silently dropped.
type composeNoticeResult struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// handleDeployCompose handles POST /api/v1/apps/{name}/compose: the
// request body is a compose.yaml document, name is the store.App it
// becomes (App.ID == App.Name, matching how migrations/0039_apps.sql's
// own backfill treats a single-service app's ID). Each resulting
// service is saved directly via SaveDesiredService, the same path
// POST /api/v1/apps (a pre-built image, no build step) already uses,
// since every compose service here already carries a resolved image.
// Each saved service also gets its own deploy_attempts row, so a
// compose/template deploy shows up in that service's own deploy history.
//
// This route's own gate (routes.go) is AbilityDeploy, but a compose file
// bind-mounting a host directory (internal/compose's own doc comment on
// volumes:) additionally requires AbilityRoot, checked here rather than
// at the route: only some compose files carry a bind mount, so the
// gate has to be conditional on the parsed body, the same "gate
// something tighter than the route itself" shape callerHasAbility's own
// doc comment (auth.go) describes.
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

	secretEnv, unresolved, err := compose.ResolveMagicVars(file, rt.generateComposeSecret(r.Context(), name), rt.persistComposeSecret(r.Context(), name))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(unresolved) > 0 {
		msgs := make([]string, len(unresolved))
		for i, u := range unresolved {
			msgs[i] = u.String()
		}
		writeError(w, http.StatusBadRequest, "unresolved template variable(s), no default and not auto-generatable: "+strings.Join(msgs, "; "))
		return
	}

	services, healthWarnings, err := compose.ToDesiredServices(name, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if hasBindMount(services) && !rt.callerHasAbility(r, AbilityRoot) {
		writeError(w, http.StatusForbidden, "this compose file bind-mounts a host directory, which requires the root ability")
		return
	}
	for _, warning := range healthWarnings {
		rt.logger.Warn("api: deploy compose: healthcheck not translatable to a readiness probe", slog.String("app", name), slog.String("detail", warning))
	}
	for i := range services {
		key := strings.TrimPrefix(services[i].Name, name+"-")
		services[i].SecretEnv = append(services[i].SecretEnv, store.SecretEnvRefsFromNames(secretEnv[key])...)
	}

	// Loaded before this deploy writes anything: staleComposeServices
	// below needs the previous member list to diff against, and name is
	// already this app's own ID (App.ID == App.Name, same convention
	// this handler's own SaveApp call below relies on), so this is safe
	// to look up even on a first-ever deploy (ListServicesByApp on an
	// app with no row yet just returns none).
	previousServices, err := rt.appGroups.ListServicesByApp(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: deploy compose: list previous services failed", err, slog.String("name", name))
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
		rt.recordInstantDeployAttempt(r.Context(), svc, svc.Image, store.DeployAttemptSourceCompose)
		out = append(out, toAppResource(svc))
	}

	rt.pruneStaleComposeServices(r.Context(), name, previousServices, services)

	fileNotices := file.Notices()
	notices := make([]composeNoticeResult, 0, len(fileNotices))
	for _, n := range fileNotices {
		notices = append(notices, composeNoticeResult{Level: string(n.Level), Message: n.Message})
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, composeDeployResponse{AppID: name, Services: out, Notices: notices})
}

// pruneStaleComposeServices deletes every member of appName's previous
// deploy (previous) that this deploy's own file no longer declares
// (current), plus their real containers: without this, a compose.yaml
// that drops a service (e.g. removing a "worker" entry) left that
// service's DesiredService row and containers running forever, since
// handleDeployCompose otherwise only ever saves what the new file
// declares and never diffs against what the last one did. Mirrors
// teardownPreviewApp's own delete-then-tear-down-containers shape
// (preview_environments.go), logged rather than failing the request on
// error: the new services this deploy actually asked for already
// deployed successfully by the time this runs, and a stale sibling
// left behind by a failed prune is a real but lesser problem than
// discarding an otherwise-successful deploy over it.
func (rt *Router) pruneStaleComposeServices(ctx context.Context, appName string, previous, current []store.DesiredService) {
	keep := make(map[string]bool, len(current))
	for _, svc := range current {
		keep[svc.Name] = true
	}
	for _, svc := range previous {
		if keep[svc.Name] {
			continue
		}
		if err := rt.apps.DeleteDesiredService(ctx, svc.Name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			rt.logger.Error("api: deploy compose: prune stale service failed", slog.String("error", err.Error()), slog.String("app", appName), slog.String("service", svc.Name))
			continue
		}
		rt.teardownServiceContainers(svc.Name, svc.NodeID)
	}
}

// generateComposeSecret resolves one compose.ResolveMagicVars
// generatable value: reuses whatever was already generated for this
// app + kind/key (Resolve keyed by appName, a synthetic bucket distinct
// from any real service, matching store.EmailSettingsSecretsKey's own
// non-service-key convention), or generates and persists a fresh one on
// first use. Without rt.composeSecrets configured, any compose file
// needing a generated value fails loudly rather than deploying with a
// broken literal token.
func (rt *Router) generateComposeSecret(ctx context.Context, appName string) func(kind, key string, length int) (string, error) {
	return func(kind, key string, length int) (string, error) {
		if rt.composeSecrets == nil {
			return "", fmt.Errorf("this compose file needs a generated secret (SERVICE_%s_%s) but no secrets master key is configured", kind, key)
		}
		storageKey := composeSecretStorageKey(kind, key)
		val, err := rt.composeSecrets.Resolve(ctx, appName, storageKey)
		if err == nil {
			return val, nil
		}
		if !errors.Is(err, secrets.ErrValueNotFound) {
			return "", fmt.Errorf("resolve existing value for SERVICE_%s_%s: %w", kind, key, err)
		}
		val, err = compose.GenerateValue(kind, length)
		if err != nil {
			return "", err
		}
		if err := rt.composeSecrets.SetValue(ctx, appName, storageKey, val); err != nil {
			return "", fmt.Errorf("save generated value for SERVICE_%s_%s: %w", kind, key, err)
		}
		return val, nil
	}
}

// persistComposeSecret writes a magic var's resolved value into the
// real per-service secret location the application controller's own
// secret resolver reads from at container-create time
// (internal/reconcile/application's WithSecretResolver, keyed by the
// real service name, not by an app-wide magic-var key).
func (rt *Router) persistComposeSecret(ctx context.Context, appName string) func(serviceKey, envKey, value string) error {
	return func(serviceKey, envKey, value string) error {
		if rt.composeSecrets == nil {
			return fmt.Errorf("no secrets master key configured")
		}
		return rt.composeSecrets.SetValue(ctx, appName+"-"+serviceKey, envKey, value)
	}
}

func composeSecretStorageKey(kind, key string) string {
	return "compose_" + strings.ToLower(kind) + "_" + strings.ToLower(key)
}

// hasBindMount reports whether any of services carries a bind mount, the
// signal handleDeployCompose uses to decide whether this request needs
// AbilityRoot on top of the route's own AbilityDeploy gate.
func hasBindMount(services []store.DesiredService) bool {
	for _, svc := range services {
		if len(svc.BindMounts) > 0 {
			return true
		}
	}
	return false
}
