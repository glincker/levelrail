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

	services, err := compose.ToDesiredServices(name, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for i := range services {
		key := strings.TrimPrefix(services[i].Name, name+"-")
		services[i].SecretEnv = append(services[i].SecretEnv, secretEnv[key]...)
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
		rt.recordInstantDeployAttempt(r.Context(), svc.Name, svc.Image, store.DeployAttemptSourceCompose)
		out = append(out, toAppResource(svc))
	}

	fileNotices := file.Notices()
	notices := make([]composeNoticeResult, 0, len(fileNotices))
	for _, n := range fileNotices {
		notices = append(notices, composeNoticeResult{Level: string(n.Level), Message: n.Message})
	}

	writeJSON(w, http.StatusOK, composeDeployResponse{AppID: name, Services: out, Notices: notices})
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
