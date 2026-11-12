package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// secretPolicyMode is the effective mode for an app's environment: off unless the
// environment is listed in secrets.require_ref_environments. The rule is runtime
// settings only, so it applies live.
func (rt *Router) secretPolicyMode(ctx context.Context, envID string) (mode, envName string) {
	if envID == "" || rt.environments == nil || rt.runtimeSettings == nil {
		return runtimesettings.SecretRefModeOff, ""
	}
	mode = rt.runtimeSettings.String(ctx, runtimesettings.KeySecretsRequireRefMode)
	if mode != runtimesettings.SecretRefModeWarn && mode != runtimesettings.SecretRefModeEnforce {
		return runtimesettings.SecretRefModeOff, ""
	}
	env, err := rt.environments.GetEnvironment(ctx, envID)
	if err != nil {
		return runtimesettings.SecretRefModeOff, ""
	}
	for _, listed := range rt.runtimeSettings.List(ctx, runtimesettings.KeySecretsRequireRefEnvironments) {
		if strings.EqualFold(strings.TrimSpace(listed), env.Name) {
			return mode, env.Name
		}
	}
	return runtimesettings.SecretRefModeOff, ""
}

// literalSecretKeys lists, sorted, the env keys whose literal value is secret-looking and
// differs from prior. Placeholders and unchanged values are not new literals.
func literalSecretKeys(env, prior map[string]string, overrides map[string]bool) []string {
	var keys []string
	for k, v := range env {
		if v == "" || envredact.IsPlaceholder(v) {
			continue
		}
		if old, ok := prior[k]; ok && old == v {
			continue
		}
		var ov *bool
		if o, ok := overrides[k]; ok {
			ov = &o
		}
		if sensitive, _ := envredact.Classify(k, v, ov); sensitive {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// secretPolicyRule names the rule in messages, never a value.
func secretPolicyRule(mode, envName string) string {
	return fmt.Sprintf("environment %q requires secret references (secrets.require_ref_environments, secrets.require_ref_mode=%s)", envName, mode)
}

// enforceSecretPolicy writes 422 and returns false when mode is enforce and keys hold new literal secrets.
func (rt *Router) enforceSecretPolicy(ctx context.Context, w http.ResponseWriter, envID string, keys []string) bool {
	if len(keys) == 0 {
		return true
	}
	mode, envName := rt.secretPolicyMode(ctx, envID)
	if mode != runtimesettings.SecretRefModeEnforce {
		return true
	}
	writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s: %s must be a secret:// reference, not a literal value", secretPolicyRule(mode, envName), strings.Join(keys, ", ")))
	return false
}

// secretPolicyWarnings lists literal secrets already held by svc in a warn or enforce environment.
func (rt *Router) secretPolicyWarnings(ctx context.Context, svc store.DesiredService) []string {
	mode, envName := rt.secretPolicyMode(ctx, svc.EnvironmentID)
	if mode == runtimesettings.SecretRefModeOff {
		return nil
	}
	var overrides map[string]bool
	if s, ok := rt.envSettingsFor(ctx, svc.Name); ok {
		overrides = s.SensitiveOverrides
	}
	keys := literalSecretKeys(svc.Env, nil, overrides)
	for _, n := range store.SecretEnvNames(svc.SecretEnv) {
		if !slices.Contains(keys, n) {
			keys = append(keys, n)
		}
	}
	slices.Sort(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s holds a literal secret value, but %s", k, secretPolicyRule(mode, envName)))
	}
	return out
}

// attachSecretPolicy sets the response-only warnings on res.
func (rt *Router) attachSecretPolicy(ctx context.Context, res *appResource, svc store.DesiredService) {
	res.RequireRefWarnings = rt.secretPolicyWarnings(ctx, svc)
}

// envOverrides reads an app's per-key sensitivity overrides; an unreadable row means none.
func (rt *Router) envOverrides(ctx context.Context, name string) map[string]bool {
	s, _ := rt.envSettingsFor(ctx, name)
	return s.SensitiveOverrides
}

// createLiteralSecretKeys lists the literal secrets a create request carries: inline secret
// values and secret-looking env entries.
func createLiteralSecretKeys(req appResource) []string {
	keys := literalSecretKeys(req.Env, nil, nil)
	for k := range req.Secrets {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}
