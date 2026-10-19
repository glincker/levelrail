package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Env finding kinds and fix actions.
const (
	envFindingEmptyValue       = "empty_value"
	envFindingShadowed         = "shadowed"
	envFindingCaseDuplicate    = "case_duplicate"
	envFindingMissingSecret    = "missing_secret"
	envFindingDanglingDatabase = "dangling_database"

	envFixRemoveKey         = "remove_key"
	envFixRemoveDatabaseRef = "remove_database_ref"
	envFixSetSecret         = "set_secret"
	envFixRename            = "rename"

	envFixAuto   = "auto"
	envFixManual = "manual"

	envSeverityWarning = "warning"
	envSeverityError   = "error"

	envChangeSourceAPI = "api"
	envChangeSourceFix = "fix"

	envHistoryDefaultLimit = 50
	envHistoryMaxLimit     = 200
)

type envChangeStore interface {
	RecordEnvChanges(ctx context.Context, appName, actor, source string, changes []store.EnvChange) error
	ListEnvChanges(ctx context.Context, appName string, limit, offset int) ([]store.EnvChange, error)
}

type envFixResource struct {
	Type   string `json:"type"`
	Action string `json:"action"`
}

type envFindingResource struct {
	Kind     string         `json:"kind"`
	Key      string         `json:"key"`
	Severity string         `json:"severity"`
	Detail   string         `json:"detail,omitempty"`
	Fix      envFixResource `json:"fix"`
}

type envCheckResponse struct {
	Findings []envFindingResource `json:"findings"`
}

type envChangeResource struct {
	ID     int64  `json:"id"`
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Key    string `json:"key"`
	Action string `json:"action"`
	Source string `json:"source"`
}

type envHistoryResponse struct {
	Changes []envChangeResource `json:"changes"`
}

// diffEnvKeys lists the keys added, changed or removed between two env maps, sorted by key.
func diffEnvKeys(oldEnv, newEnv map[string]string) []store.EnvChange {
	var out []store.EnvChange
	for k, nv := range newEnv {
		ov, ok := oldEnv[k]
		switch {
		case !ok:
			out = append(out, store.EnvChange{Key: k, Action: store.EnvChangeAdded})
		case ov != nv:
			out = append(out, store.EnvChange{Key: k, Action: store.EnvChangeChanged})
		}
	}
	for k := range oldEnv {
		if _, ok := newEnv[k]; !ok {
			out = append(out, store.EnvChange{Key: k, Action: store.EnvChangeRemoved})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// checkAppEnv finds problems in an app's variables. secretSet holds the secret names that have a stored value.
func checkAppEnv(svc *store.DesiredService, secretSet map[string]bool, secretsKnown bool, databaseExists func(string) bool) []envFindingResource {
	var out []envFindingResource
	declared := map[string]bool{}
	for _, s := range svc.SecretEnv {
		declared[s.Name] = true
	}
	for k := range svc.DatabaseEnv {
		declared[k] = true
	}
	for k := range svc.VaultEnv {
		declared[k] = true
	}
	lowered := map[string][]string{}
	for k, v := range svc.Env {
		lowered[strings.ToLower(k)] = append(lowered[strings.ToLower(k)], k)
		if declared[k] {
			out = append(out, envFindingResource{Kind: envFindingShadowed, Key: k, Severity: envSeverityWarning,
				Fix: envFixResource{Type: envFixAuto, Action: envFixRemoveKey}})
			continue
		}
		if strings.TrimSpace(v) == "" {
			out = append(out, envFindingResource{Kind: envFindingEmptyValue, Key: k, Severity: envSeverityWarning,
				Fix: envFixResource{Type: envFixAuto, Action: envFixRemoveKey}})
		}
	}
	for _, keys := range lowered {
		if len(keys) < 2 {
			continue
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, envFindingResource{Kind: envFindingCaseDuplicate, Key: k, Severity: envSeverityWarning,
				Detail: strings.Join(keys, ", "), Fix: envFixResource{Type: envFixManual, Action: envFixRename}})
		}
	}
	if secretsKnown {
		for _, s := range svc.SecretEnv {
			if s.Required && !secretSet[s.Name] {
				out = append(out, envFindingResource{Kind: envFindingMissingSecret, Key: s.Name, Severity: envSeverityError,
					Fix: envFixResource{Type: envFixManual, Action: envFixSetSecret}})
			}
		}
	}
	for k, ref := range svc.DatabaseEnv {
		if !databaseExists(ref.Database) {
			out = append(out, envFindingResource{Kind: envFindingDanglingDatabase, Key: k, Severity: envSeverityError,
				Detail: ref.Database, Fix: envFixResource{Type: envFixAuto, Action: envFixRemoveDatabaseRef}})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func (rt *Router) envFindings(ctx context.Context, svc *store.DesiredService) ([]envFindingResource, error) {
	secretSet := map[string]bool{}
	secretsKnown := rt.secrets != nil
	if secretsKnown {
		keys, err := rt.secrets.ListKeys(ctx, svc.Name)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			secretSet[k.Key] = true
		}
	}
	dbExists := func(string) bool { return true }
	if len(svc.DatabaseEnv) > 0 {
		known := map[string]bool{}
		for _, ref := range svc.DatabaseEnv {
			if _, seen := known[ref.Database]; seen {
				continue
			}
			_, err := rt.databases.GetDesiredDatabase(ctx, ref.Database)
			known[ref.Database] = !errors.Is(err, store.ErrDatabaseNotFound)
		}
		dbExists = func(name string) bool { return known[name] }
	}
	return checkAppEnv(svc, secretSet, secretsKnown, dbExists), nil
}

// handleCheckEnv handles GET /api/v1/apps/{name}/env/check.
func (rt *Router) handleCheckEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	findings, err := rt.envFindings(r.Context(), svc)
	if err != nil {
		rt.internalError(w, "api: env check failed", err, slog.String("name", name))
		return
	}
	if findings == nil {
		findings = []envFindingResource{}
	}
	writeJSON(w, http.StatusOK, envCheckResponse{Findings: findings})
}

type envFixRequest struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

// handleFixEnv handles POST /api/v1/apps/{name}/env/check/fix: it applies the
// automatic fix of one current finding and records the change.
func (rt *Router) handleFixEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req envFixRequest
	if !decodeReq(w, r, &req) {
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	findings, err := rt.envFindings(r.Context(), svc)
	if err != nil {
		rt.internalError(w, "api: env fix check failed", err, slog.String("name", name))
		return
	}
	var match *envFindingResource
	for i := range findings {
		if findings[i].Kind == req.Kind && findings[i].Key == req.Key {
			match = &findings[i]
			break
		}
	}
	if match == nil {
		writeError(w, http.StatusConflict, "this problem is already fixed")
		return
	}
	if match.Fix.Type != envFixAuto {
		writeError(w, http.StatusBadRequest, "this problem needs a manual fix")
		return
	}
	updated := *svc
	updated.Env = make(map[string]string, len(svc.Env))
	for k, v := range svc.Env {
		updated.Env[k] = v
	}
	switch match.Fix.Action {
	case envFixRemoveKey:
		delete(updated.Env, req.Key)
	case envFixRemoveDatabaseRef:
		updated.DatabaseEnv = make(map[string]store.DatabaseEnvRef, len(svc.DatabaseEnv))
		for k, v := range svc.DatabaseEnv {
			if k != req.Key {
				updated.DatabaseEnv[k] = v
			}
		}
	}
	updated.EnvDirty = true
	if err := rt.apps.SaveDesiredService(r.Context(), updated); err != nil {
		rt.internalError(w, "api: env fix save failed", err, slog.String("name", name))
		return
	}
	rt.recordEnvChanges(r, name, svc.Env, updated.Env, envChangeSourceFix)
	remaining, err := rt.envFindings(r.Context(), &updated)
	if err != nil {
		remaining = nil
	}
	if remaining == nil {
		remaining = []envFindingResource{}
	}
	writeJSON(w, http.StatusOK, envCheckResponse{Findings: remaining})
}

// recordEnvChanges stores the key-level diff of an env edit. A failure is logged, never fatal.
func (rt *Router) recordEnvChanges(r *http.Request, app string, oldEnv, newEnv map[string]string, source string) {
	if rt.envProtect.changes == nil {
		return
	}
	changes := diffEnvKeys(oldEnv, newEnv)
	if len(changes) == 0 {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if err := rt.envProtect.changes.RecordEnvChanges(r.Context(), app, actor, source, changes); err != nil {
		rt.logger.Warn("api: record env changes failed", slog.String("error", err.Error()), slog.String("name", app))
	}
}

// handleListEnvHistory handles GET /api/v1/apps/{name}/env/history (limit, offset).
func (rt *Router) handleListEnvHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := rt.loadRevealApp(w, r, name); !ok {
		return
	}
	limit, offset := envHistoryPage(r)
	out := envHistoryResponse{Changes: []envChangeResource{}}
	if rt.envProtect.changes != nil {
		rows, err := rt.envProtect.changes.ListEnvChanges(r.Context(), name, limit, offset)
		if err != nil {
			rt.internalError(w, "api: list env history failed", err, slog.String("name", name))
			return
		}
		for _, c := range rows {
			out.Changes = append(out.Changes, envChangeResource{ID: c.ID, At: c.At.UTC().Format(time.RFC3339), Actor: c.Actor, Key: c.Key, Action: c.Action, Source: c.Source})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func envHistoryPage(r *http.Request) (limit, offset int) {
	limit = envHistoryDefaultLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, envHistoryMaxLimit)
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && n > 0 {
		offset = n
	}
	return limit, offset
}
