package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/envrules"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// envPolicy reads the live env validation settings; without a settings service the defaults apply.
func (rt *Router) envPolicy(ctx context.Context) envrules.Policy {
	p := envrules.DefaultPolicy()
	if rt.runtimeSettings == nil {
		return p
	}
	rs := rt.runtimeSettings
	p.KeyMode = rs.String(ctx, runtimesettings.KeyEnvKeyPolicy)
	p.KeyMaxLen = rs.Int(ctx, runtimesettings.KeyEnvKeyMaxLength)
	p.ValueMaxBytes = rs.Int(ctx, runtimesettings.KeyEnvValueMaxBytes)
	p.ReservedPrefixes = rs.List(ctx, runtimesettings.KeyEnvReservedPrefixes)
	p.LoaderMode = rs.String(ctx, runtimesettings.KeyEnvDangerousKeysMode)
	p.PathMode = rs.String(ctx, runtimesettings.KeyEnvPathOverrideMode)
	return p
}

// existingEnv describes what an app already holds, for the validator.
func existingEnv(s *store.DesiredService) envrules.Existing {
	return envrules.Existing{Plain: s.Env, Names: store.SecretEnvNames(s.SecretEnv)}
}

// checkEnvWrite validates the keys being written and answers 400 on a refusal.
// It returns the warnings to pass on, and false when the write was refused.
func (rt *Router) checkEnvWrite(ctx context.Context, w http.ResponseWriter, set map[string]string, ex envrules.Existing, o envrules.Options) ([]envrules.Issue, bool) {
	res := envrules.Validate(set, ex, rt.envPolicy(ctx), o)
	if msg := res.Err(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return nil, false
	}
	return res.Warnings, true
}

// checkEnvKeys validates keys that only name an entry (unset, delete), refusing unusable names.
func checkEnvKeys(w http.ResponseWriter, keys []string) bool {
	if msg := envrules.ValidateKeys(keys).Err(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return false
	}
	return true
}

// envRefusal validates a set outside an HTTP response and returns the first refusal, or "".
func (rt *Router) envRefusal(ctx context.Context, set map[string]string, ex envrules.Existing, o envrules.Options) string {
	return envrules.Validate(set, ex, rt.envPolicy(ctx), o).Err()
}

// templateEnvRefusal checks the install form's env keys and values; the app does not exist yet.
func (rt *Router) templateEnvRefusal(ctx context.Context, env map[string]string) string {
	return rt.envRefusal(ctx, env, envrules.Existing{}, envrules.Options{Multiline: true})
}

// bundleEnvRefusal checks an imported bundle's plain env and its secret names.
func (rt *Router) bundleEnvRefusal(ctx context.Context, plain map[string]string, secretNames []string) string {
	if msg := rt.envRefusal(ctx, plain, envrules.Existing{}, envrules.Options{Multiline: true}); msg != "" {
		return msg
	}
	names := make(map[string]string, len(secretNames))
	for _, n := range secretNames {
		names[n] = "x"
	}
	return rt.envRefusal(ctx, names, envrules.Existing{}, envrules.Options{Secret: true, Multiline: true})
}

// checkAppOverrideEnv validates a preview, branch or PR override against the app's
// stored keys, so it refuses and accepts exactly what PUT env does. It answers 404
// for a missing app and returns false whenever it already wrote the response.
func (rt *Router) checkAppOverrideEnv(ctx context.Context, w http.ResponseWriter, name, key, value string, secret bool) ([]envrules.Issue, bool) {
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	} else if err != nil {
		rt.internalError(w, "api: load app for env override failed", err, slog.String("name", name))
		return nil, false
	}
	return rt.checkEnvWrite(ctx, w, map[string]string{key: value}, existingEnv(svc), envrules.Options{Secret: secret, Multiline: true})
}

// updateEnvRefusal checks a full app save against the keys the app already holds,
// so untouched legacy keys pass and only new or changed ones are held to the rules.
func (rt *Router) updateEnvRefusal(ctx context.Context, env map[string]string, secretNames []string, existing *store.DesiredService) string {
	ex := existingEnv(existing)
	if msg := rt.envRefusal(ctx, env, ex, envrules.Options{Multiline: true}); msg != "" {
		return msg
	}
	names := make(map[string]string, len(secretNames))
	for _, n := range secretNames {
		names[n] = "x"
	}
	return rt.envRefusal(ctx, names, ex, envrules.Options{Secret: true, Multiline: true})
}

// composeEnvRefusal checks every service of a compose deploy. Dotted names are
// tolerated because templates ship them (Java style keys); everything else is strict.
func (rt *Router) composeEnvRefusal(ctx context.Context, services []store.DesiredService) string {
	for _, s := range services {
		if msg := rt.envRefusal(ctx, s.Env, envrules.Existing{}, envrules.Options{Multiline: true, ExistingUnknown: true}); msg != "" {
			return s.Name + ": " + msg
		}
	}
	return ""
}

// cloneEnvRefusal checks the env a clone would copy; names that already exist
// on the source are tolerated, so only genuinely unusable data is refused.
func (rt *Router) cloneEnvRefusal(ctx context.Context, sources []store.DesiredService) string {
	return rt.composeEnvRefusal(ctx, sources)
}

// checkSharedSecretEnv validates a shared-scope secret key against the keys the
// scope already holds, so a legacy name that exists stays editable and new ones are strict.
func (rt *Router) checkSharedSecretEnv(ctx context.Context, w http.ResponseWriter, scope sharedEnvSecretScope, id, key, value string) bool {
	vars, err := scope.listAll(ctx, id)
	if err != nil {
		rt.internalError(w, "api: "+scope.label+" secret env: load current failed", err, slog.String("id", id))
		return false
	}
	ex := envrules.Existing{Plain: map[string]string{}}
	for _, v := range vars {
		if v.Secret {
			ex.Names = append(ex.Names, v.Key)
		} else {
			ex.Plain[v.Key] = v.Value
		}
	}
	_, ok := rt.checkEnvWrite(ctx, w, map[string]string{key: value}, ex, envrules.Options{Secret: true, Multiline: true})
	return ok
}
