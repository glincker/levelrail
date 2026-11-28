package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

// envApplyStore is the transactional env writer; *store.DB satisfies it.
type envApplyStore interface {
	ApplyEnvPlan(ctx context.Context, name string, plan store.EnvPlan) (store.EnvApplyResult, error)
	EnvVersion(ctx context.Context, name string) (int, error)
}

// envValueSealer encrypts a value for one slot without saving it.
type envValueSealer interface {
	SealValue(ctx context.Context, serviceName, envKey, plaintext string) ([]byte, error)
}

// envApplyResource carries names and the version only, never a value.
type envApplyResource struct {
	App        string                `json:"app"`
	DryRun     bool                  `json:"dry_run"`
	Changed    bool                  `json:"changed"`
	EnvVersion int                   `json:"env_version"`
	Plan       []store.EnvPlanChange `json:"plan"`
}

// envApplyConflictResource is the 409 body for a stale expected_version.
type envApplyConflictResource struct {
	Error          string   `json:"error"`
	Code           string   `json:"code"`
	CurrentVersion int      `json:"current_version"`
	PlanKeys       []string `json:"plan_keys"`
}

// handleApplyEnvPlan handles POST /api/v1/apps/{name}/env/apply: plain env,
// secrets and vault references change together or not at all.
func (rt *Router) handleApplyEnvPlan(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.refuseIfProtected(w, r, name) {
		return
	}
	var req envApplyRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if msg := validateEnvPlan(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.touchesSensitive() && !rt.callerHasAbility(r, AbilityWriteSensitive) {
		writeError(w, http.StatusForbidden, "secret and vault changes need the write:sensitive ability")
		return
	}
	st, ok := rt.apps.(envApplyStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "env apply is not available on this control plane")
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	if msg := rt.validateEnvPlanRules(r.Context(), svc, req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	plan, ok := rt.buildEnvPlan(w, r, name, svc, req)
	if !ok {
		return
	}
	res, err := st.ApplyEnvPlan(r.Context(), name, plan)
	if rt.writeEnvApplyError(w, name, req, err) {
		return
	}
	out := envApplyResource{App: name, DryRun: req.DryRun, Changed: res.Changed, EnvVersion: res.Version, Plan: res.Changes}
	if out.Plan == nil {
		out.Plan = []store.EnvPlanChange{}
	}
	if !req.DryRun && res.Changed {
		rt.afterEnvApply(r, name, req, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// buildEnvPlan validates the plan against policy and seals secret values outside the transaction.
func (rt *Router) buildEnvPlan(w http.ResponseWriter, r *http.Request, name string, svc *store.DesiredService, req envApplyRequest) (store.EnvPlan, bool) {
	plan := store.EnvPlan{
		ExpectedVersion: req.ExpectedVersion, Set: req.Set, Unset: req.Unset, SecretDelete: req.SecretDelete,
		Convert: req.Convert, VaultClear: req.VaultClear, OverwriteLocked: req.OverwriteLocked, DryRun: req.DryRun,
	}
	secretValues := map[string]string{}
	for k, v := range req.SecretSet {
		secretValues[k] = v
	}
	for _, k := range req.Convert {
		v, exists := svc.Env[k]
		if !exists {
			writeError(w, http.StatusNotFound, "no plain variable "+k+" on this app to convert")
			return plan, false
		}
		if v == "" {
			writeError(w, http.StatusBadRequest, "an empty variable cannot become a secret")
			return plan, false
		}
		secretValues[k] = v
	}
	if len(secretValues) > 0 {
		if rt.secrets == nil {
			writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
			return plan, false
		}
		keys := make([]string, 0, len(secretValues))
		values := make([]string, 0, len(secretValues))
		for k, v := range secretValues {
			keys = append(keys, k)
			values = append(values, v)
		}
		sort.Strings(keys)
		if !rt.enforceSecretPolicy(r.Context(), w, svc.EnvironmentID, keys) || !rt.allowProviderRefs(w, r, values...) {
			return plan, false
		}
		sealer, ok := rt.secrets.(envValueSealer)
		if !ok {
			writeError(w, http.StatusNotImplemented, "env apply cannot seal secrets on this control plane")
			return plan, false
		}
		plan.SecretSet = map[string][]byte{}
		for k, v := range secretValues {
			ct, err := sealer.SealValue(r.Context(), name, k, v)
			if err != nil {
				rt.internalError(w, "api: env apply: seal failed", err, slog.String("name", name), slog.String("key", k))
				return plan, false
			}
			plan.SecretSet[k] = ct
		}
	}
	if len(req.VaultSet) > 0 {
		plan.VaultSet = map[string]store.VaultEnvRef{}
		for k, v := range req.VaultSet {
			ref, ok := rt.vaultRefForPlan(w, r, svc, k, v)
			if !ok {
				return plan, false
			}
			plan.VaultSet[k] = ref
		}
	}
	return plan, true
}

func (rt *Router) vaultRefForPlan(w http.ResponseWriter, r *http.Request, svc *store.DesiredService, envVar string, v setAppVaultEnvRequest) (store.VaultEnvRef, bool) {
	if v.Ref != "" {
		parsed, err := secretsource.ParseRef(v.Ref)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret reference: "+err.Error())
			return store.VaultEnvRef{}, false
		}
		v = setAppVaultEnvRequest{Conn: parsed.Conn, Path: parsed.Path, Key: parsed.Key, Version: parsed.Version}
	}
	if v.Path == "" || v.Key == "" {
		writeError(w, http.StatusBadRequest, "vault_set entries need a path and a key")
		return store.VaultEnvRef{}, false
	}
	if v.Conn != "" {
		parsed, err := envRefOf(appVaultEnvRef{Conn: v.Conn, Path: v.Path, Key: v.Key, Version: v.Version})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret reference: "+err.Error())
			return store.VaultEnvRef{}, false
		}
		if !rt.allowSecretRefs(w, r, svc.EnvironmentID, parsed) {
			return store.VaultEnvRef{}, false
		}
	}
	if v.Conn == "" && !rt.allowLegacyVaultPath(w, r, svc.VaultEnv[envVar], v.Path) {
		return store.VaultEnvRef{}, false
	}
	return store.VaultEnvRef{Path: v.Path, Key: v.Key, Conn: v.Conn, Version: v.Version}, true
}

// writeEnvApplyError maps a store error to a response and reports whether it wrote one.
func (rt *Router) writeEnvApplyError(w http.ResponseWriter, name string, req envApplyRequest, err error) bool {
	var conflict *store.EnvVersionConflict
	switch {
	case err == nil:
		return false
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, envApplyConflictResource{
			Error: "the env changed since expected_version; reload and retry", Code: "env_version_conflict",
			CurrentVersion: conflict.Current, PlanKeys: planKeyNames(req),
		})
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
	case errors.Is(err, store.ErrEnvLockedSecret):
		writeError(w, http.StatusConflict, "a locked secret is in the plan, set overwrite_locked to confirm replacing it")
	case errors.Is(err, store.ErrEnvDualHome):
		writeError(w, http.StatusConflict, "a key in the plan would be both a plain and a secret value; remove one copy first")
	case errors.Is(err, store.ErrEnvKeyIsSecret):
		writeError(w, http.StatusConflict, "a plain key in the plan is stored as a secret; remove the secret first")
	default:
		rt.internalError(w, "api: env apply failed", err, slog.String("name", name))
	}
	return true
}

func planKeyNames(req envApplyRequest) []string {
	seen := map[string]bool{}
	add := func(k string) { seen[k] = true }
	for k := range req.Set {
		add(k)
	}
	for k := range req.SecretSet {
		add(k)
	}
	for k := range req.VaultSet {
		add(k)
	}
	for _, l := range [][]string{req.Unset, req.SecretDelete, req.Convert, req.VaultClear} {
		for _, k := range l {
			add(k)
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// afterEnvApply records history, the audit row and the reconcile nudge; names only.
func (rt *Router) afterEnvApply(r *http.Request, name string, req envApplyRequest, res store.EnvApplyResult) {
	keys := make([]string, 0, len(res.Changes))
	for _, c := range res.Changes {
		keys = append(keys, c.Key)
	}
	_, _, actor, _ := rt.currentActor(r)
	if len(req.Set) > 0 || len(req.Unset) > 0 || len(req.Convert) > 0 {
		if svc, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
			rt.snapshotEnvRevision(r.Context(), name, actor, svc.Env)
		}
	}
	kind := store.AppEventEnvChange
	if len(req.SecretSet) > 0 || len(req.SecretDelete) > 0 || len(req.Convert) > 0 {
		kind = store.AppEventSecretChange
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: kind, Keys: keys, Title: "Env applied"})
	detail, _ := json.Marshal(map[string]any{"app": name, "version": res.Version, "plan": res.Changes})
	a := rt.accessActor(r)
	rt.saveAppExpiryAudit(r.Context(), store.AuditEntry{
		ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPost, Path: "/api/v1/apps/" + name + "/env/apply",
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: "app.env.apply", Detail: string(detail),
	})
	rt.nudgeReconciler()
}

// envVersionOf reads the app's env version, or 0 when the store keeps none.
func (rt *Router) envVersionOf(ctx context.Context, name string) int {
	st, ok := rt.apps.(envApplyStore)
	if !ok {
		return 0
	}
	v, err := st.EnvVersion(ctx, name)
	if err != nil {
		return 0
	}
	return v
}
