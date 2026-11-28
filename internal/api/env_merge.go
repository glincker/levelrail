package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GLINCKER/levelrail/internal/envrules"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// envMergeStore is the column-scoped env writer; *store.DB satisfies it.
type envMergeStore interface {
	MergeServiceEnv(ctx context.Context, name string, set map[string]string, unset []string) (store.EnvMergeResult, error)
}

type envMergeRequest struct {
	Set   map[string]string `json:"set"`
	Unset []string          `json:"unset"`
}

// envMergeResource carries names only, never a value.
type envMergeResource struct {
	App         string   `json:"app"`
	Changed     bool     `json:"changed"`
	EnvDirty    bool     `json:"env_dirty"`
	Set         []string `json:"set"`
	Removed     []string `json:"removed"`
	Missing     []string `json:"missing"`
	Keys        []string `json:"keys"`
	RebuildKeys []string `json:"rebuild_keys,omitempty"`
	EnvVersion  int      `json:"env_version,omitempty"`
	// Warnings are findings on the written keys that did not block the write; no values.
	Warnings []envrules.Issue `json:"warnings,omitempty"`
}

// handlePutAppEnv handles PUT /api/v1/apps/{name}/env: a batch of set and unset
// merged into the stored env in one transaction, touching no other app field.
func (rt *Router) handlePutAppEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.refuseIfProtected(w, r, name) {
		return
	}
	var req envMergeRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if len(req.Set) == 0 && len(req.Unset) == 0 {
		writeError(w, http.StatusBadRequest, "send at least one key in set or unset")
		return
	}
	for k, v := range req.Set {
		if k == "" || strings.ContainsAny(k, "=\x00") {
			writeError(w, http.StatusBadRequest, "env keys must be non-empty and contain no = or NUL")
			return
		}
		if envredact.IsPlaceholder(v) {
			writeError(w, http.StatusBadRequest, "a masked placeholder is not a value; send the real value or leave the key out")
			return
		}
	}
	for _, k := range req.Unset {
		if k == "" || strings.ContainsAny(k, "=\x00") {
			writeError(w, http.StatusBadRequest, "env keys must be non-empty and contain no = or NUL")
			return
		}
		if _, both := req.Set[k]; both {
			writeError(w, http.StatusBadRequest, "a key cannot be in both set and unset")
			return
		}
	}
	st, ok := rt.apps.(envMergeStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "env merge is not available on this control plane")
		return
	}
	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: env merge: read app failed", err, slog.String("name", name))
		return
	}
	warnings, ok := rt.checkEnvWrite(r.Context(), w, req.Set, existingEnv(existing), envrules.Options{Multiline: true})
	if !ok {
		return
	}
	if !rt.enforceSecretPolicy(r.Context(), w, existing.EnvironmentID, literalSecretKeys(req.Set, existing.Env, rt.envOverrides(r.Context(), name))) {
		return
	}
	res, err := st.MergeServiceEnv(r.Context(), name, req.Set, req.Unset)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if errors.Is(err, store.ErrEnvKeyIsSecret) {
		writeError(w, http.StatusConflict, "a key is stored as a secret; change it with PUT /api/v1/apps/{name}/secrets/{key} (CLI: apps secrets set), or drop the secret first with apps secrets delete")
		return
	}
	if err != nil {
		rt.internalError(w, "api: env merge failed", err, slog.String("name", name))
		return
	}
	out := envMergeResource{Warnings: warnings, App: name, Changed: res.Changed, EnvDirty: res.EnvDirty, Set: []string{}, Removed: []string{}, Missing: []string{}, Keys: []string{}}
	for k, v := range req.Set {
		if old, had := res.Before[k]; !had || old != v {
			out.Set = append(out.Set, k)
		}
	}
	for _, k := range req.Unset {
		if _, had := res.Before[k]; had {
			out.Removed = append(out.Removed, k)
		} else {
			out.Missing = append(out.Missing, k)
		}
	}
	for k := range res.After {
		out.Keys = append(out.Keys, k)
	}
	for _, l := range [][]string{out.Set, out.Removed, out.Missing, out.Keys} {
		slices.Sort(l)
	}
	if res.Changed {
		rt.recordEnvChanges(r, name, res.Before, res.After, envChangeSourceAPI)
		out.RebuildKeys = buildTimeEnvChanges(res.Before, res.After)
		rt.nudgeReconciler()
		rt.changes.Publish(changeApps, name)
	}
	out.EnvVersion = rt.envVersionOf(r.Context(), name)
	rt.auditEnvMerge(r, out)
	writeJSON(w, http.StatusOK, out)
}

// auditEnvMerge logs names only, never a value.
func (rt *Router) auditEnvMerge(r *http.Request, res envMergeResource) {
	detail, _ := json.Marshal(map[string]any{"app": res.App, "set": res.Set, "removed": res.Removed})
	a := rt.accessActor(r)
	rt.saveAppExpiryAudit(r.Context(), store.AuditEntry{
		ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPut, Path: "/api/v1/apps/" + res.App + "/env",
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: "app.env.merge", Detail: string(detail),
	})
}
