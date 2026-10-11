package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Secret reveal policies (APP_SECRET_REVEAL).
const (
	revealPolicyStepUp   = "step_up"
	revealPolicyDisabled = "disabled"
	revealPolicyAllow    = "allow"

	envSecretReveal       = "APP_SECRET_REVEAL"        //nolint:gosec // an env var name, not a credential
	envSecretRevealTokens = "APP_SECRET_REVEAL_TOKENS" //nolint:gosec // an env var name, not a credential
	envStepUpTTL          = "APP_SECRET_STEP_UP_TTL"
	envRevealHideAfter    = "APP_SECRET_REVEAL_HIDE_AFTER"

	defaultStepUpTTL       = 5 * time.Minute
	defaultRevealHideAfter = 30 * time.Second
	maxStepUpTTL           = 15 * time.Minute
)

// appEnvSettingsStore is the per-app protection state; *store.DB implements it.
type appEnvSettingsStore interface {
	GetAppEnvSettings(ctx context.Context, name string) (store.AppEnvSettings, error)
	ListAppEnvSettings(ctx context.Context) (map[string]store.AppEnvSettings, error)
	SaveAppEnvSettings(ctx context.Context, s store.AppEnvSettings) error
}

// envProtection groups the env redaction and step-up reveal state.
type envProtection struct {
	settings appEnvSettingsStore
	grants   *stepUpGrants
	limiter  *loginLimiter
}

func (rt *Router) attachEnvProtection(s Store) {
	rt.envProtect.settings, _ = s.(appEnvSettingsStore)
	rt.envProtect.grants = newStepUpGrants()
	rt.envProtect.limiter = newLoginLimiter()
}

// revealPolicy reads APP_SECRET_REVEAL. An unrecognized value fails closed.
func revealPolicy() (policy string, valid bool) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envSecretReveal)))
	switch v {
	case "", revealPolicyStepUp, "step-up", "stepup":
		return revealPolicyStepUp, true
	case revealPolicyDisabled, revealPolicyAllow:
		return v, true
	}
	return revealPolicyDisabled, false
}

// tokenRevealAllowed reads APP_SECRET_REVEAL_TOKENS (default off).
func tokenRevealAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envSecretRevealTokens))) {
	case "on", "true", "1", "yes":
		return true
	}
	return false
}

func positiveEnvDuration(name string, def, ceiling time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil || d <= 0 {
		return def
	}
	if ceiling > 0 && d > ceiling {
		return ceiling
	}
	return d
}

func stepUpTTL() time.Duration {
	return positiveEnvDuration(envStepUpTTL, defaultStepUpTTL, maxStepUpTTL)
}

func revealHideAfter() time.Duration {
	return positiveEnvDuration(envRevealHideAfter, defaultRevealHideAfter, 0)
}

// envSettingsFor loads name's protection state. ok=false means it could not
// be read, and callers must then treat every value as sensitive.
func (rt *Router) envSettingsFor(ctx context.Context, name string) (store.AppEnvSettings, bool) {
	if rt.envProtect.settings == nil {
		return store.AppEnvSettings{AppName: name}, true
	}
	s, err := rt.envProtect.settings.GetAppEnvSettings(ctx, name)
	if err != nil {
		rt.logger.Error("api: load app env settings failed", slog.String("error", err.Error()), slog.String("name", name))
		return store.AppEnvSettings{AppName: name}, false
	}
	return s, true
}

func maskEverything(env map[string]string) map[string]bool {
	out := make(map[string]bool, len(env))
	for k := range env {
		out[k] = true
	}
	return out
}

// redactAppEnvFields masks every value-bearing env field of res from svc.
func redactAppEnvFields(res *appResource, svc store.DesiredService, overrides map[string]bool) {
	res.Env, res.EnvMeta = envredact.Map(svc.Env, overrides)
	res.PreviewEnvOverrides, _ = envredact.Map(res.PreviewEnvOverrides, overrides)
}

// appOut is toAppResource plus the app's own sensitivity overrides.
func (rt *Router) appOut(ctx context.Context, svc store.DesiredService) appResource {
	res := toAppResourceRaw(svc)
	rt.redactAppResponse(ctx, &res, svc)
	return res
}

// redactAppResponse masks res's env fields from svc using name's settings,
// masking everything when the settings cannot be read.
func (rt *Router) redactAppResponse(ctx context.Context, res *appResource, svc store.DesiredService) {
	settings, ok := rt.envSettingsFor(ctx, svc.Name)
	overrides := settings.SensitiveOverrides
	if !ok {
		overrides = maskEverything(svc.Env)
		for k := range res.PreviewEnvOverrides {
			overrides[k] = true
		}
	}
	env := svc.Env
	if env == nil && res.Env != nil {
		env = res.Env
	}
	res.Env, res.EnvMeta = envredact.Map(env, overrides)
	res.PreviewEnvOverrides, _ = envredact.Map(res.PreviewEnvOverrides, overrides)
	res.SecretsWriteOnly = settings.SecretsWriteOnly || !ok
}

// appOverridesForList batch-loads overrides for a list response. A failed
// load returns nil, ok=false so each row masks everything.
func (rt *Router) appOverridesForList(ctx context.Context) (map[string]store.AppEnvSettings, bool) {
	if rt.envProtect.settings == nil {
		return nil, true
	}
	all, err := rt.envProtect.settings.ListAppEnvSettings(ctx)
	if err != nil {
		rt.logger.Error("api: list app env settings failed", slog.String("error", err.Error()))
		return nil, false
	}
	return all, true
}

// redactListedApp masks one row of a list response with preloaded settings.
func redactListedApp(res *appResource, svc store.DesiredService, all map[string]store.AppEnvSettings, ok bool) {
	overrides := all[svc.Name].SensitiveOverrides
	if !ok {
		overrides = maskEverything(svc.Env)
	}
	redactAppEnvFields(res, svc, overrides)
	res.SecretsWriteOnly = all[svc.Name].SecretsWriteOnly || !ok
}

// mergeEnvPlaceholders resolves masked placeholders in an update against the
// stored env, writing a 400 when a placeholder has nothing to keep.
func (rt *Router) mergeEnvPlaceholders(w http.ResponseWriter, req *appResource, existing *store.DesiredService) bool {
	merged, err := envredact.Merge(req.Env, existing.Env)
	var pe *envredact.PlaceholderError
	if errors.As(err, &pe) {
		writeError(w, http.StatusBadRequest, pe.Error())
		return false
	}
	req.Env = merged
	if len(req.PreviewEnvOverrides) > 0 {
		mergedPreview, err := envredact.Merge(req.PreviewEnvOverrides, existing.PreviewEnvOverrides)
		if errors.As(err, &pe) {
			writeError(w, http.StatusBadRequest, pe.Error())
			return false
		}
		req.PreviewEnvOverrides = mergedPreview
	}
	return true
}

// copyEnvSettings carries a source app's protection state onto a clone, so
// a variable the operator flagged sensitive stays flagged.
func (rt *Router) copyEnvSettings(ctx context.Context, from, to string) {
	if rt.envProtect.settings == nil {
		return
	}
	src, err := rt.envProtect.settings.GetAppEnvSettings(ctx, from)
	if err == nil && len(src.SensitiveOverrides) == 0 && !src.SecretsWriteOnly {
		return
	}
	if err == nil {
		src.AppName = to
		err = rt.envProtect.settings.SaveAppEnvSettings(ctx, src)
	}
	if err != nil {
		rt.logger.Error("api: copy app env settings failed", slog.String("error", err.Error()), slog.String("name", from), slog.String("clone", to))
	}
}

// redactedEnvMap masks a shared env layer (organization, project,
// environment) by automatic classification.
func redactedEnvMap(vars map[string]string) map[string]string {
	out, _ := envredact.Map(vars, nil)
	return out
}
