package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envDiffAgainstPreview = "preview"
	envDiffAgainstEnv     = "environment:"
	envDiffAgainstApp     = "app:"
)

// envDiffSide is one side of a diff: key presence, plain values (used only
// for equality inside this file) and which keys hold secret or Vault values.
type envDiffSide struct {
	plain  map[string]string
	opaque map[string]bool
}

// envDiffResource lists keys only. No value, secret or plain, ever appears.
type envDiffResource struct {
	App           string   `json:"app"`
	Against       string   `json:"against"`
	Added         []string `json:"added"`
	Removed       []string `json:"removed"`
	Changed       []string `json:"changed"`
	Same          int      `json:"same"`
	SecretUnknown int      `json:"secret_unchecked"`
}

func (s envDiffSide) has(k string) bool {
	_, ok := s.plain[k]
	return ok || s.opaque[k]
}

func serviceDiffSide(svc *store.DesiredService, withPreview bool) envDiffSide {
	side := envDiffSide{plain: make(map[string]string, len(svc.Env)), opaque: map[string]bool{}}
	for k, v := range svc.Env {
		side.plain[k] = v
	}
	if withPreview {
		for k, v := range svc.PreviewEnvOverrides {
			side.plain[k] = v
		}
	}
	for _, k := range store.SecretEnvNames(svc.SecretEnv) {
		side.opaque[k] = true
	}
	for k := range svc.VaultEnv {
		side.opaque[k] = true
	}
	return side
}

// diffEnvSides compares base to other by key. Plain values are compared for
// equality only; opaque keys on both sides are counted, never compared.
func diffEnvSides(app, against string, base, other envDiffSide) envDiffResource {
	res := envDiffResource{App: app, Against: against, Added: []string{}, Removed: []string{}, Changed: []string{}}
	keys := map[string]bool{}
	for _, s := range []envDiffSide{base, other} {
		for k := range s.plain {
			keys[k] = true
		}
		for k := range s.opaque {
			keys[k] = true
		}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		inBase, inOther := base.has(k), other.has(k)
		switch {
		case inOther && !inBase:
			res.Added = append(res.Added, k)
		case inBase && !inOther:
			res.Removed = append(res.Removed, k)
		case base.opaque[k] != other.opaque[k]:
			res.Changed = append(res.Changed, k)
		case base.opaque[k]:
			res.SecretUnknown++
		case base.plain[k] != other.plain[k]:
			res.Changed = append(res.Changed, k)
		default:
			res.Same++
		}
	}
	return res
}

// handleEnvDiff handles GET /api/v1/apps/{name}/env-diff?against=: keys that
// are added, removed or changed against another app, a shared environment
// (environment:<id>) or this app's preview overrides (preview).
func (rt *Router) handleEnvDiff(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	name := r.PathValue("name")
	against := strings.TrimSpace(r.URL.Query().Get("against"))
	if against == "" {
		writeError(w, http.StatusBadRequest, "against is required: an app name, environment:<id> or preview")
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	base := serviceDiffSide(svc, false)
	other, ok := rt.envDiffOther(w, r, svc, against)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, diffEnvSides(name, against, base, other))
}

func (rt *Router) envDiffOther(w http.ResponseWriter, r *http.Request, svc *store.DesiredService, against string) (envDiffSide, bool) {
	switch {
	case against == envDiffAgainstPreview:
		return serviceDiffSide(svc, true), true
	case strings.HasPrefix(against, envDiffAgainstEnv):
		return rt.envDiffEnvironmentSide(w, r, strings.TrimPrefix(against, envDiffAgainstEnv))
	}
	other := strings.TrimPrefix(against, envDiffAgainstApp)
	if !rt.requireReadableApp(w, r, other) {
		return envDiffSide{}, false
	}
	osvc, ok := rt.loadRevealApp(w, r, other)
	if !ok {
		return envDiffSide{}, false
	}
	return serviceDiffSide(osvc, false), true
}

func (rt *Router) envDiffEnvironmentSide(w http.ResponseWriter, r *http.Request, id string) (envDiffSide, bool) {
	if rt.environments == nil || id == "" {
		writeError(w, http.StatusNotFound, "environment not found")
		return envDiffSide{}, false
	}
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "environment not found"), environmentRef(id)) {
		return envDiffSide{}, false
	}
	if _, err := rt.environments.GetEnvironment(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			writeError(w, http.StatusNotFound, "environment not found")
		} else {
			rt.internalError(w, "api: env diff: load environment failed", err, slog.String("id", id))
		}
		return envDiffSide{}, false
	}
	vars, err := rt.environments.ListEnvironmentEnvVars(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: env diff: list environment env failed", err, slog.String("id", id))
		return envDiffSide{}, false
	}
	keys, err := rt.environments.ListEnvironmentSecretEnvKeys(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: env diff: list environment secret keys failed", err, slog.String("id", id))
		return envDiffSide{}, false
	}
	side := envDiffSide{plain: vars, opaque: map[string]bool{}}
	for _, k := range keys {
		side.opaque[k] = true
	}
	return side, true
}
