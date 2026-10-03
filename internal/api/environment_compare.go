package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Cross-environment env var diff (this file): merges the organization,
// project, and environment shared-env tiers in resolveEnv's own
// precedence (internal/reconcile/application), stopping short of any
// single service's own env since none is in scope for a
// project+environment comparison. store.ServiceBranchEnvOverride is
// excluded too: it keys by branch and service, not environment. Secret
// values are never resolved, not even to compare them: a key present on
// both sides is reported as masked rather than changed or same.

// Environment env diff statuses (environmentEnvDiffEntry.Status).
const (
	envDiffOnlyInA = "only_in_a"
	envDiffOnlyInB = "only_in_b"
	envDiffChanged = "changed"
	envDiffMasked  = "masked"
)

// environmentEnvEntryResource is one resolved key inside
// environmentCompareSide.Env: Value is only ever populated when Secret is
// false, the same "never echo a value" rule sharedEnvVarResource already
// follows for a secret-marked shared env var.
type environmentEnvEntryResource struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret"`
}

// environmentEnvDiffEntry is one key that differs between A and B. A/B
// are only ever populated for a non-secret key: see envDiffMasked's own
// doc comment for why a secret-marked key present on both sides never
// gets a from/to value.
type environmentEnvDiffEntry struct {
	Key    string `json:"key"`
	Secret bool   `json:"secret"`
	Status string `json:"status"`
	A      string `json:"a,omitempty"`
	B      string `json:"b,omitempty"`
}

// environmentCompareSide is one side (A or B) of GET .../environments/
// compare's response.
type environmentCompareSide struct {
	Environment environmentResource           `json:"environment"`
	Env         []environmentEnvEntryResource `json:"env"`
}

// environmentCompareResource is GET
// /api/v1/projects/{id}/environments/compare's response shape.
type environmentCompareResource struct {
	ProjectID string                    `json:"project_id"`
	A         environmentCompareSide    `json:"a"`
	B         environmentCompareSide    `json:"b"`
	Diff      []environmentEnvDiffEntry `json:"diff"`
	Note      string                    `json:"note"`
}

const environmentCompareNote = "Each side shows its resolved effective env vars: organization defaults, then project defaults, then this environment's own vars, each tier overriding the one before it (internal/reconcile/application's own resolveEnv precedence), stopping short of any single service's own env since no service is in scope for a project+environment comparison. A secret-marked key never shows a value, on either side. In the diff, a key present on only one side is reported regardless of whether it's secret; a secret-marked key present on both sides is reported as masked rather than changed or same, since this control plane cannot tell whether the two differ without decrypting them; a plain key present on both sides with the same value is not reported at all."

// handleCompareEnvironmentEnv handles GET
// /api/v1/projects/{id}/environments/compare?a={environmentId}&b={environmentId}.
func (rt *Router) handleCompareEnvironmentEnv(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	project, err := rt.projects.GetProject(r.Context(), projectID)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: compare environment env: load project failed", err, slog.String("project_id", projectID))
		return
	}

	aID := r.URL.Query().Get("a")
	bID := r.URL.Query().Get("b")
	if aID == "" || bID == "" {
		writeError(w, http.StatusBadRequest, "a and b environment ids are required")
		return
	}

	envA, ok := rt.loadProjectEnvironmentForCompare(w, r, project, aID, "a")
	if !ok {
		return
	}
	envB, ok := rt.loadProjectEnvironmentForCompare(w, r, project, bID, "b")
	if !ok {
		return
	}

	effA, err := rt.resolveEffectiveSharedEnv(r.Context(), project, envA)
	if err != nil {
		rt.internalError(w, "api: compare environment env: resolve effective env failed", err, slog.String("environment_id", envA.ID))
		return
	}
	effB, err := rt.resolveEffectiveSharedEnv(r.Context(), project, envB)
	if err != nil {
		rt.internalError(w, "api: compare environment env: resolve effective env failed", err, slog.String("environment_id", envB.ID))
		return
	}

	writeJSON(w, http.StatusOK, environmentCompareResource{
		ProjectID: project.ID,
		A:         environmentCompareSide{Environment: toEnvironmentResource(envA), Env: effectiveEnvResources(effA)},
		B:         environmentCompareSide{Environment: toEnvironmentResource(envB), Env: effectiveEnvResources(effB)},
		Diff:      diffEffectiveEnv(effA, effB),
		Note:      environmentCompareNote,
	})
}

// loadProjectEnvironmentForCompare loads environmentID, writing the
// 404/400 response itself (ok=false) on failure, so
// handleCompareEnvironmentEnv's two call sites (a, b) share one error
// path. side is "a" or "b", for the error message only.
func (rt *Router) loadProjectEnvironmentForCompare(w http.ResponseWriter, r *http.Request, project store.Project, environmentID, side string) (store.Environment, bool) {
	env, err := rt.environments.GetEnvironment(r.Context(), environmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, side+" environment not found: "+environmentID)
		return store.Environment{}, false
	}
	if err != nil {
		rt.internalError(w, "api: compare environment env: load environment failed", err, slog.String("environment_id", environmentID))
		return store.Environment{}, false
	}
	if env.ProjectID != project.ID {
		writeError(w, http.StatusBadRequest, side+" environment "+environmentID+" does not belong to project "+project.ID)
		return store.Environment{}, false
	}
	return env, true
}

// effectiveSharedEnvEntry is one resolved key's value and secret marker.
// A secret-marked key's Value is always "": see this file's own package
// doc comment on why its plaintext is never resolved here.
type effectiveSharedEnvEntry struct {
	Value  string
	Secret bool
}

// resolveEffectiveSharedEnv merges project's organization, project, and
// env's own shared env tiers in internal/reconcile/application's own
// resolveEnv precedence (organization lowest, then project, then this
// environment).
func (rt *Router) resolveEffectiveSharedEnv(ctx context.Context, project store.Project, env store.Environment) (map[string]effectiveSharedEnvEntry, error) {
	out := map[string]effectiveSharedEnvEntry{}

	applyTier := func(plain map[string]string, secretKeys []string) {
		for k, v := range plain {
			out[k] = effectiveSharedEnvEntry{Value: v}
		}
		for _, k := range secretKeys {
			out[k] = effectiveSharedEnvEntry{Secret: true}
		}
	}

	if project.OrgID != "" {
		orgVars, err := rt.organizations.ListOrganizationEnvVars(ctx, project.OrgID)
		if err != nil {
			return nil, fmt.Errorf("list organization env vars: %w", err)
		}
		orgSecretKeys, err := rt.organizations.ListOrganizationSecretEnvKeys(ctx, project.OrgID)
		if err != nil {
			return nil, fmt.Errorf("list organization secret env keys: %w", err)
		}
		applyTier(orgVars, orgSecretKeys)
	}

	projectVars, err := rt.projects.ListProjectEnvVars(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("list project env vars: %w", err)
	}
	projectSecretKeys, err := rt.projects.ListProjectSecretEnvKeys(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("list project secret env keys: %w", err)
	}
	applyTier(projectVars, projectSecretKeys)

	environmentVars, err := rt.environments.ListEnvironmentEnvVars(ctx, env.ID)
	if err != nil {
		return nil, fmt.Errorf("list environment env vars: %w", err)
	}
	environmentSecretKeys, err := rt.environments.ListEnvironmentSecretEnvKeys(ctx, env.ID)
	if err != nil {
		return nil, fmt.Errorf("list environment secret env keys: %w", err)
	}
	applyTier(environmentVars, environmentSecretKeys)

	return out, nil
}

// effectiveEnvResources renders eff as a sorted (by key) wire list. A
// secret-marked entry's Value is always left empty: eff itself never
// holds a secret's plaintext in the first place (resolveEffectiveSharedEnv's
// own doc comment), so there is nothing to redact here beyond not
// reading a field that was never set.
func effectiveEnvResources(eff map[string]effectiveSharedEnvEntry) []environmentEnvEntryResource {
	keys := make([]string, 0, len(eff))
	for k := range eff {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]environmentEnvEntryResource, len(keys))
	for i, k := range keys {
		e := eff[k]
		out[i] = environmentEnvEntryResource{Key: k, Secret: e.Secret}
		if !e.Secret {
			out[i].Value = e.Value
		}
	}
	return out
}

// diffEffectiveEnv reports every key that differs between a and b: a key
// present on only one side (status only_in_a/only_in_b, regardless of
// whether it's secret), or a plain key present on both sides with
// different values (status changed). A secret-marked key present on both
// sides is reported as masked rather than changed or same: see this
// file's own package doc comment for why. A plain key present on both
// sides with the same value is never reported: no drift, nothing to
// surface.
func diffEffectiveEnv(a, b map[string]effectiveSharedEnvEntry) []environmentEnvDiffEntry {
	keySet := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keySet[k] = struct{}{}
	}
	for k := range b {
		keySet[k] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []environmentEnvDiffEntry
	for _, k := range keys {
		av, inA := a[k]
		bv, inB := b[k]
		switch {
		case inA && !inB:
			entry := environmentEnvDiffEntry{Key: k, Secret: av.Secret, Status: envDiffOnlyInA}
			if !av.Secret {
				entry.A = av.Value
			}
			out = append(out, entry)
		case !inA && inB:
			entry := environmentEnvDiffEntry{Key: k, Secret: bv.Secret, Status: envDiffOnlyInB}
			if !bv.Secret {
				entry.B = bv.Value
			}
			out = append(out, entry)
		case av.Secret || bv.Secret:
			out = append(out, environmentEnvDiffEntry{Key: k, Secret: true, Status: envDiffMasked})
		case av.Value != bv.Value:
			out = append(out, environmentEnvDiffEntry{Key: k, Status: envDiffChanged, A: av.Value, B: bv.Value})
		}
	}
	return out
}
