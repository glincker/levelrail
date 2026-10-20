package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// How a simulated decision was reached.
const (
	decidedExplicitDeny  = "explicit_deny"
	decidedBaseAbility   = "base_ability"
	decidedExplicitAllow = "explicit_allow"
	decidedNoGrant       = "no_grant"
	decidedHidden        = "hidden"
	decidedInactive      = "inactive_principal"
)

type statementRef struct {
	PolicyID   string   `json:"policy_id"`
	PolicyName string   `json:"policy_name"`
	Index      int      `json:"statement_index"`
	Effect     Effect   `json:"effect"`
	Action     []string `json:"action"`
	Resource   []string `json:"resource"`
}

type simulation struct {
	PrincipalType        string         `json:"principal_type"`
	PrincipalID          string         `json:"principal_id"`
	PrincipalName        string         `json:"principal_name"`
	Action               string         `json:"action"`
	Resource             string         `json:"resource"`
	Allowed              bool           `json:"allowed"`
	DecidedBy            string         `json:"decided_by"`
	Deciding             *statementRef  `json:"deciding_statement"`
	Matched              []statementRef `json:"matched_statements"`
	BaseGrants           bool           `json:"base_ability_grants"`
	EnvironmentResources []string       `json:"environment_resources"`
}

// matchingStatements lists, in evaluation order, every statement of every
// policy that matches, using the evaluator's own statementMatchesAny.
func matchingStatements(policies []store.Policy, ability, resource string, extra []string) []statementRef {
	out := []statementRef{}
	for _, p := range policies {
		doc, err := ParseDocument(p.Document)
		if err != nil {
			continue
		}
		for i, s := range doc.Statement {
			if statementMatchesAny(s, ability, resource, extra) {
				out = append(out, statementRef{PolicyID: p.ID, PolicyName: p.Name, Index: i, Effect: s.Effect, Action: s.Action, Resource: s.Resource})
			}
		}
	}
	return out
}

// simulateAccess answers "can this principal do ability on resource" with the
// real decision path (authorizeResourceInEnvironment) and then explains it.
func (rt *Router) simulateAccess(ctx context.Context, p iamPrincipal, policies []store.Policy, ability, resource string) (simulation, error) {
	allowed, hidden, err := rt.authorizeResourceInEnvironment(ctx, p.Type, p.ID, p.Abilities, policies, ability, resource)
	if err != nil {
		return simulation{}, fmt.Errorf("simulate: %w", err)
	}
	extra, err := rt.simulationEnvironment(ctx, p, policies, resource)
	if err != nil {
		return simulation{}, err
	}
	sim := simulation{
		PrincipalType: p.Type, PrincipalID: p.ID, PrincipalName: p.Name, Action: ability, Resource: resource,
		Allowed: allowed && !hidden, BaseGrants: hasAbility(p.Abilities, ability), EnvironmentResources: extra,
		Matched: matchingStatements(policies, ability, resource, extra),
	}
	if sim.EnvironmentResources == nil {
		sim.EnvironmentResources = []string{}
	}
	sim.DecidedBy, sim.Deciding = classifyDecision(sim, hidden)
	if !p.Active {
		sim.Allowed, sim.DecidedBy = false, decidedInactive
	}
	return sim, nil
}

func classifyDecision(sim simulation, hidden bool) (string, *statementRef) {
	if hidden {
		return decidedHidden, nil
	}
	for i := range sim.Matched {
		if sim.Matched[i].Effect == EffectDeny {
			return decidedExplicitDeny, &sim.Matched[i]
		}
	}
	if sim.BaseGrants {
		return decidedBaseAbility, nil
	}
	for i := range sim.Matched {
		if sim.Matched[i].Effect == EffectAllow {
			return decidedExplicitAllow, &sim.Matched[i]
		}
	}
	return decidedNoGrant, nil
}

// simulationEnvironment mirrors authorizeResourceInEnvironment's own rule for
// when a resource's environment matters, so the explanation lists exactly what
// the decision saw.
func (rt *Router) simulationEnvironment(ctx context.Context, p iamPrincipal, policies []store.Policy, resource string) ([]string, error) {
	prefix, name, ok := splitResource(resource)
	if !ok {
		return nil, nil
	}
	vis, err := rt.principalVisibility(ctx, p.Type, p.ID)
	if err != nil {
		return nil, fmt.Errorf("simulate: visibility: %w", err)
	}
	if !vis.Restricted && !policiesMentionEnvironments(policies) {
		return nil, nil
	}
	ref, err := rt.environmentForResource(ctx, prefix, name)
	if err != nil {
		return nil, fmt.Errorf("simulate: environment: %w", err)
	}
	return environmentResources(ref), nil
}

// handleIAMSimulate handles GET /api/v1/iam/simulate. It persists nothing.
func (rt *Router) handleIAMSimulate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ptype, pid, ability, resource := q.Get("principal_type"), q.Get("principal_id"), q.Get("action"), strings.TrimSpace(q.Get("resource"))
	if !slices.Contains(validPrincipalTypes, ptype) || pid == "" {
		writeError(w, http.StatusBadRequest, "principal_type must be user or token and principal_id is required")
		return
	}
	if !isKnownAbility(ability) {
		writeError(w, http.StatusBadRequest, (&unknownActionError{action: ability}).Error())
		return
	}
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource is required, for example app:web")
		return
	}
	ctx := r.Context()
	all, err := rt.loadIAMPrincipals(ctx)
	if err != nil {
		rt.internalError(w, "api: iam simulate: load principals failed", err)
		return
	}
	p, ok := findPrincipal(all, ptype, pid)
	if !ok {
		writeError(w, http.StatusNotFound, "principal not found")
		return
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, ptype, pid)
	if err != nil {
		rt.internalError(w, "api: iam simulate: list policies failed", err, slog.String("principal_id", pid))
		return
	}
	sim, err := rt.simulateAccess(ctx, p, policies, ability, resource)
	if err != nil {
		rt.internalError(w, "api: iam simulate failed", err, slog.String("principal_id", pid))
		return
	}
	writeJSON(w, http.StatusOK, sim)
}

type effectiveAbility struct {
	Ability         string   `json:"ability"`
	Risk            string   `json:"risk"`
	Flat            bool     `json:"flat"`
	All             bool     `json:"all"`
	Allowed         int      `json:"allowed"`
	Total           int      `json:"total"`
	Apps            []string `json:"apps"`
	Databases       []string `json:"databases"`
	GrantedByPolicy int      `json:"granted_by_policy"`
	DeniedByPolicy  int      `json:"denied_by_policy"`
}

type policyBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type effectiveResponse struct {
	Principal  iamPrincipal       `json:"principal"`
	Policies   []policyBrief      `json:"policies"`
	Restricted bool               `json:"restricted"`
	Abilities  []effectiveAbility `json:"abilities"`
}

// effectiveFor computes what one principal can do on every app and database
// under the given policy set, with the same scope.authorize list endpoints use.
func (rt *Router) effectiveFor(ctx context.Context, p iamPrincipal, policies []store.Policy, inv *iamInventory) ([]effectiveAbility, bool, error) {
	scope, err := rt.newResourceScope(ctx, p.Type, p.ID, policies)
	if err != nil {
		return nil, false, fmt.Errorf("effective: scope: %w", err)
	}
	out := make([]effectiveAbility, 0, len(validAbilities))
	for _, ability := range validAbilities {
		e := effectiveAbility{Ability: ability, Risk: abilityRisk(ability), Flat: hasAbility(p.Abilities, ability), Apps: []string{}, Databases: []string{}}
		for _, set := range []struct {
			prefix string
			names  []string
			dst    *[]string
		}{{resourcePrefixApp, inv.apps, &e.Apps}, {resourcePrefixDatabase, inv.databases, &e.Databases}} {
			for _, n := range set.names {
				e.Total++
				ok := scope.authorize(p.Abilities, policies, ability, set.prefix, n)
				switch {
				case ok:
					e.Allowed++
					*set.dst = append(*set.dst, n)
					if !e.Flat {
						e.GrantedByPolicy++
					}
				case e.Flat && scope.visible(set.prefix, n):
					e.DeniedByPolicy++
				}
			}
		}
		e.All = e.Total > 0 && e.Allowed == e.Total
		out = append(out, e)
	}
	return out, scope.restricted(), nil
}

// handleIAMEffective handles GET /api/v1/iam/principals/{type}/{id}/effective.
func (rt *Router) handleIAMEffective(w http.ResponseWriter, r *http.Request) {
	ptype, pid := r.PathValue("principal_type"), r.PathValue("principal_id")
	ctx := r.Context()
	all, err := rt.loadIAMPrincipals(ctx)
	if err != nil {
		rt.internalError(w, "api: iam effective: load principals failed", err)
		return
	}
	p, ok := findPrincipal(all, ptype, pid)
	if !ok {
		writeError(w, http.StatusNotFound, "principal not found")
		return
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, ptype, pid)
	if err != nil {
		rt.internalError(w, "api: iam effective: list policies failed", err, slog.String("principal_id", pid))
		return
	}
	inv, err := rt.loadIAMInventory(ctx)
	if err != nil {
		rt.internalError(w, "api: iam effective: inventory failed", err)
		return
	}
	abilities, restricted, err := rt.effectiveFor(ctx, p, policies, inv)
	if err != nil {
		rt.internalError(w, "api: iam effective failed", err, slog.String("principal_id", pid))
		return
	}
	resp := effectiveResponse{Principal: p, Restricted: restricted, Abilities: abilities, Policies: []policyBrief{}}
	for _, pol := range policies {
		resp.Policies = append(resp.Policies, policyBrief{ID: pol.ID, Name: pol.Name})
		resp.Principal.Policies = append(resp.Principal.Policies, pol.ID)
	}
	writeJSON(w, http.StatusOK, resp)
}
