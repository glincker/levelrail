package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/policy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	policyOverrideForbiddenCode = "policy_override_forbidden"
	policyOverrideReasonCode    = "policy_override_reason_required"
	appEventPolicyOverride      = "policy.override"
)

// deployPolicyStore is the persistence deploy guardrails need; *store.DB
// satisfies it.
type deployPolicyStore interface {
	GetDeployPolicy(ctx context.Context, scope, scopeID string) (policy.Set, error)
	SetDeployPolicy(ctx context.Context, scope, scopeID string, set policy.Set, by string) error
	DeleteDeployPolicy(ctx context.Context, scope, scopeID string) error
	ListDeployPolicies(ctx context.Context) ([]store.DeployPolicy, error)
}

func (rt *Router) deployPolicyDB() deployPolicyStore {
	for _, candidate := range []any{rt.apps, rt.projects, rt.policies} {
		if s, ok := candidate.(deployPolicyStore); ok {
			return s
		}
	}
	return nil
}

// effectivePolicy merges the org, project and environment rule sets, the
// most specific scope winning per rule.
func (rt *Router) effectivePolicy(ctx context.Context, projectID, environmentID string) (policy.Set, error) {
	db := rt.deployPolicyDB()
	if db == nil {
		return nil, nil
	}
	org, err := db.GetDeployPolicy(ctx, policy.ScopeOrg, policy.OrgScopeID)
	if err != nil {
		return nil, err
	}
	layers := []policy.Set{org}
	for _, l := range []struct{ scope, id string }{{policy.ScopeProject, projectID}, {policy.ScopeEnvironment, environmentID}} {
		if l.id == "" {
			continue
		}
		s, err := db.GetDeployPolicy(ctx, l.scope, l.id)
		if err != nil {
			return nil, err
		}
		layers = append(layers, s)
	}
	return policy.Merge(layers...), nil
}

func deployPolicySubject(svc store.DesiredService) policy.Subject {
	subj := policy.Subject{Image: svc.Image, Labels: svc.Labels, Replicas: svc.Replicas}
	if svc.Resources != nil {
		subj.MemoryBytes, subj.NanoCPUs = svc.Resources.MemoryBytes, svc.Resources.NanoCPUs
	}
	if svc.Health != nil {
		subj.HasReadiness = svc.Health.Readiness != nil
	}
	for _, bm := range svc.BindMounts {
		subj.HostMounts = append(subj.HostMounts, bm.HostPath)
	}
	return subj
}

// policyOverride is the audited escape hatch from enforced guardrails.
type policyOverride struct {
	PolicyOverride bool   `json:"override_policy,omitempty"`
	PolicyReason   string `json:"override_policy_reason,omitempty"`
}

type policyViolationBody struct {
	Error      string             `json:"error"`
	Code       string             `json:"code"`
	Violations []policy.Violation `json:"violations"`
}

// policyGate refuses a deploy that breaks an enforced rule. An override
// needs the root ability and a reason, and leaves an app event behind. ok
// false means a response was already written.
func (rt *Router) policyGate(w http.ResponseWriter, r *http.Request, svc store.DesiredService, o policyOverride) (note string, ok bool) {
	set, err := rt.effectivePolicy(r.Context(), svc.ProjectID, svc.EnvironmentID)
	if err != nil {
		rt.internalError(w, "api: deploy policy: load", err, slog.String("name", svc.Name))
		return "", false
	}
	vs := policy.Evaluate(set, deployPolicySubject(svc))
	for _, v := range vs {
		if !v.Blocks() {
			rt.logger.Warn("api: deploy policy warning", slog.String("name", svc.Name), slog.String("code", v.Code))
		}
	}
	blocking := policy.Blocking(vs)
	if len(blocking) == 0 {
		return "", true
	}
	if !o.PolicyOverride {
		writeJSON(w, http.StatusUnprocessableEntity, policyViolationBody{
			Error:      blocking[0].Message + ". " + blocking[0].NextAction,
			Code:       policy.CodeViolation,
			Violations: blocking,
		})
		return "", false
	}
	if !rt.callerHasAbility(r, AbilityRoot) {
		writeCodedError(w, http.StatusForbidden, policyOverrideForbiddenCode, "overriding a deploy policy needs the root ability")
		return "", false
	}
	reason := strings.TrimSpace(o.PolicyReason)
	if reason == "" {
		writeCodedError(w, http.StatusBadRequest, policyOverrideReasonCode, "override_policy_reason is required to override a deploy policy")
		return "", false
	}
	codes := make([]string, 0, len(blocking))
	for _, v := range blocking {
		codes = append(codes, v.Code)
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: appEventPolicyOverride, Title: "Deploy policy overridden", Detail: reason, Keys: codes})
	return "PolicyOverride: " + reason, true
}

type deployPolicyResource struct {
	Scope     string     `json:"scope"`
	ScopeID   string     `json:"scope_id"`
	Rules     policy.Set `json:"rules"`
	UpdatedBy string     `json:"updated_by,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func toDeployPolicyResource(p store.DeployPolicy) deployPolicyResource {
	res := deployPolicyResource{Scope: p.Scope, ScopeID: p.ScopeID, Rules: p.Rules, UpdatedBy: p.UpdatedBy}
	if !p.UpdatedAt.IsZero() {
		t := p.UpdatedAt
		res.UpdatedAt = &t
	}
	return res
}

func (rt *Router) deployPolicyOrNotImplemented(w http.ResponseWriter) (deployPolicyStore, bool) {
	db := rt.deployPolicyDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "deploy policies are not available on this control plane")
	}
	return db, db != nil
}

func validPolicyScope(scope string) bool {
	return scope == policy.ScopeOrg || scope == policy.ScopeProject || scope == policy.ScopeEnvironment
}

// handleListDeployPolicies handles GET /api/v1/deploy-policies.
func (rt *Router) handleListDeployPolicies(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.deployPolicyOrNotImplemented(w)
	if !ok {
		return
	}
	list, err := db.ListDeployPolicies(r.Context())
	if err != nil {
		rt.internalError(w, "api: deploy policy: list", err)
		return
	}
	out := make([]deployPolicyResource, 0, len(list))
	for _, p := range list {
		out = append(out, toDeployPolicyResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

type deployPolicyBody struct {
	Rules policy.Set `json:"rules"`
}

// handleSetDeployPolicy handles PUT /api/v1/deploy-policies/{scope}/{id}.
// The org scope uses the id "org".
func (rt *Router) handleSetDeployPolicy(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.deployPolicyOrNotImplemented(w)
	if !ok {
		return
	}
	scope, id := r.PathValue("scope"), r.PathValue("id")
	if !validPolicyScope(scope) || (scope == policy.ScopeOrg && id != policy.OrgScopeID) {
		writeError(w, http.StatusBadRequest, "scope must be org (id org), project or environment")
		return
	}
	var body deployPolicyBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Rules == nil {
		body.Rules = policy.Set{}
	}
	if err := policy.Validate(body.Rules); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.SetDeployPolicy(r.Context(), scope, id, body.Rules, rt.eventActor(r)); err != nil {
		rt.internalError(w, "api: deploy policy: save", err)
		return
	}
	writeJSON(w, http.StatusOK, deployPolicyResource{Scope: scope, ScopeID: id, Rules: body.Rules})
}

// handleDeleteDeployPolicy handles DELETE /api/v1/deploy-policies/{scope}/{id}.
func (rt *Router) handleDeleteDeployPolicy(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.deployPolicyOrNotImplemented(w)
	if !ok {
		return
	}
	scope, id := r.PathValue("scope"), r.PathValue("id")
	if !validPolicyScope(scope) {
		writeError(w, http.StatusBadRequest, "scope must be org, project or environment")
		return
	}
	if err := db.DeleteDeployPolicy(r.Context(), scope, id); err != nil {
		rt.internalError(w, "api: deploy policy: delete", err)
		return
	}
	writeJSON(w, http.StatusOK, deployPolicyResource{Scope: scope, ScopeID: id, Rules: policy.Set{}})
}

type policyCheckBody struct {
	// App limits the check to one app; empty checks every app.
	App string `json:"app,omitempty"`
	// Scope, ScopeID and Rules preview an unsaved rule set in place of the
	// stored one for that scope.
	Scope   string     `json:"scope,omitempty"`
	ScopeID string     `json:"scope_id,omitempty"`
	Rules   policy.Set `json:"rules,omitempty"`
}

type policyCheckApp struct {
	App         string             `json:"app"`
	ProjectID   string             `json:"project_id,omitempty"`
	Environment string             `json:"environment_id,omitempty"`
	Violations  []policy.Violation `json:"violations"`
	WouldFail   bool               `json:"would_fail"`
}

type policyCheckResult struct {
	Checked int              `json:"checked"`
	Failing int              `json:"failing"`
	Warning int              `json:"warning"`
	Apps    []policyCheckApp `json:"apps"`
}

// handleCheckDeployPolicy handles POST /api/v1/deploy-policies/check: a
// dry run of what would fail against current apps. It never changes anything.
func (rt *Router) handleCheckDeployPolicy(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.deployPolicyOrNotImplemented(w)
	if !ok {
		return
	}
	var body policyCheckBody
	if r.ContentLength != 0 {
		if err := decodeJSONBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if body.Rules != nil {
		if !validPolicyScope(body.Scope) || body.ScopeID == "" {
			writeError(w, http.StatusBadRequest, "scope and scope_id are required with rules")
			return
		}
		if err := policy.Validate(body.Rules); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: deploy policy: list apps", err)
		return
	}
	stored, err := db.ListDeployPolicies(r.Context())
	if err != nil {
		rt.internalError(w, "api: deploy policy: list", err)
		return
	}
	byScope := map[string]policy.Set{}
	for _, p := range stored {
		byScope[p.Scope+"/"+p.ScopeID] = p.Rules
	}
	if body.Rules != nil {
		byScope[body.Scope+"/"+body.ScopeID] = body.Rules
	}
	res := policyCheckResult{Apps: []policyCheckApp{}}
	for _, svc := range svcs {
		if body.App != "" && svc.Name != body.App {
			continue
		}
		res.Checked++
		set := policy.Merge(byScope[policy.ScopeOrg+"/"+policy.OrgScopeID], byScope[policy.ScopeProject+"/"+svc.ProjectID], byScope[policy.ScopeEnvironment+"/"+svc.EnvironmentID])
		vs := policy.Evaluate(set, deployPolicySubject(svc))
		if len(vs) == 0 {
			continue
		}
		blocked := len(policy.Blocking(vs)) > 0
		if blocked {
			res.Failing++
		} else {
			res.Warning++
		}
		res.Apps = append(res.Apps, policyCheckApp{App: svc.Name, ProjectID: svc.ProjectID, Environment: svc.EnvironmentID, Violations: vs, WouldFail: blocked})
	}
	writeJSON(w, http.StatusOK, res)
}
