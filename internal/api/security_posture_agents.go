package api

import (
	"context"
	"slices"

	"github.com/GLINCKER/levelrail/internal/policy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// agentPostureInput is what the agent and deploy-safety checks read: counts,
// flags and settings only, never a token value or a secret.
type agentPostureInput struct {
	AIKnown         bool
	AIMode          string
	AIEnvKinds      []string
	ProdKnown       bool
	ProdEnvOpen     []string
	ProdApps        int
	GuardKnown      bool
	ProdAppsNoGuard []string
	SinksKnown      bool
	AuditSinks      int
}

func agentNameOf(a *agentIdentity) string {
	if a == nil {
		return ""
	}
	return a.Name
}

func canMutate(abilities []string) bool {
	return hasAbility(abilities, AbilityWrite) || hasAbility(abilities, AbilityWriteSensitive) || hasAbility(abilities, AbilityDeploy)
}

// collectPostureAgents gathers the AI control, production environment, deploy
// guard and audit sink facts, leaving a group unknown when its source fails.
func (rt *Router) collectPostureAgents(ctx context.Context, in *postureInput) {
	if rt.aiControl != nil {
		if s, err := rt.aiControl.GetAIControlSettings(ctx); err == nil {
			in.Agent.AIKnown, in.Agent.AIMode, in.Agent.AIEnvKinds = true, s.Mode, s.AllowedEnvKinds
		}
	}
	if rt.dataExport != nil {
		if dests, err := rt.dataExport.List(ctx); err == nil {
			in.Agent.SinksKnown = true
			for _, d := range dests {
				if d.Enabled && slices.Contains([]store.ExportKind{store.ExportKindAuditWebhook, store.ExportKindAuditS3,
					store.ExportKindAuditSplunk, store.ExportKindAuditSyslog, store.ExportKindAuditOTLP}, d.Kind) {
					in.Agent.AuditSinks++
				}
			}
		}
	}
	if rt.environments == nil || rt.apps == nil {
		return
	}
	envs, err := rt.environments.ListAllEnvironments(ctx)
	if err != nil {
		return
	}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return
	}
	prod := map[string]bool{}
	for _, e := range envs {
		if e.Kind != store.EnvironmentKindProduction {
			continue
		}
		prod[e.ID] = true
		if !e.Protected {
			in.Agent.ProdEnvOpen = append(in.Agent.ProdEnvOpen, e.Name)
		}
	}
	in.Agent.ProdKnown = true
	defaults := map[string]string{}
	if d, ok := any(rt.projects).(interface {
		DefaultEnvironmentIDs(context.Context) (map[string]string, error)
	}); ok {
		if m, derr := d.DefaultEnvironmentIDs(ctx); derr == nil {
			defaults = m
		}
	}
	var prodSvcs []store.DesiredService
	for _, s := range svcs {
		if s.EnvironmentID == "" {
			s.EnvironmentID = defaults[s.ProjectID]
		}
		if s.EnvironmentID != "" && prod[s.EnvironmentID] {
			prodSvcs = append(prodSvcs, s)
		}
	}
	in.Agent.ProdApps = len(prodSvcs)
	rt.collectPostureGuards(ctx, in, prodSvcs)
}

// collectPostureGuards names production apps with no enforcing deploy policy
// rule. Freeze windows are recurring and hold only automatic deploys, so they
// do not count as a standing guard.
func (rt *Router) collectPostureGuards(ctx context.Context, in *postureInput, prodSvcs []store.DesiredService) {
	db := rt.deployPolicyDB()
	if db == nil {
		return
	}
	stored, err := db.ListDeployPolicies(ctx)
	if err != nil {
		return
	}
	byScope := map[string]policy.Set{}
	for _, p := range stored {
		byScope[p.Scope+"/"+p.ScopeID] = p.Rules
	}
	for _, s := range prodSvcs {
		set := policy.Merge(byScope[policy.ScopeOrg+"/"+policy.OrgScopeID], byScope[policy.ScopeProject+"/"+s.ProjectID],
			byScope[policy.ScopeEnvironment+"/"+s.EnvironmentID])
		if !hasEnforcedRule(set) {
			in.Agent.ProdAppsNoGuard = append(in.Agent.ProdAppsNoGuard, s.Name)
		}
	}
	in.Agent.GuardKnown = true
}

func hasEnforcedRule(set policy.Set) bool {
	for _, r := range set {
		if r.Mode == policy.ModeEnforce {
			return true
		}
	}
	return false
}

// agentPostureRules are recommended until an app is live (or strict mode), so a
// fresh install is not scored down for gates it has nothing to protect with.
func agentPostureRules(in postureInput) []postureItem {
	return []postureItem{
		recommendUntilLive(in, agentTokenUnnamedRule(in)),
		recommendUntilLive(in, aiControlAdminRule(in)),
		recommendUntilLive(in, agentTokensProdRule(in)),
		recommendUntilLive(in, prodEnvUnprotectedRule(in)),
		recommendUntilLive(in, prodGuardRule(in)),
		recommendUntilLive(in, auditSinkRule(in)),
	}
}

// agentTokenUnnamedRule: a token without an agent name is invisible to AI control.
func agentTokenUnnamedRule(in postureInput) postureItem {
	return tokenRule(in, "agent_token_unnamed", postureMedium, linkFix("/settings/tokens"), func(t store.APIToken) bool {
		return t.AgentName == "" && canMutate(t.Abilities)
	})
}

func aiControlAdminRule(in postureInput) postureItem {
	const id = "ai_control_admin"
	if !in.Agent.AIKnown {
		return unknown(id, postureMedium, "AI control could not be read")
	}
	if in.Agent.AIMode != store.AIControlModeAdmin {
		return pass(id, postureMedium)
	}
	return failing(id, postureMedium, nil, linkFix("/settings/ai-control"))
}

// agentTokensProdRule passes only when AI control keeps agents read-only. The
// environment guard covers app and database routes only, so in operate or admin
// mode a labelled write token can still reach other routes that change production.
func agentTokensProdRule(in postureInput) postureItem {
	const id = "agent_tokens_production"
	if !in.TokensKnown || !in.Agent.AIKnown {
		return unknown(id, postureMedium, "tokens or AI control could not be read")
	}
	if in.Agent.AIMode == store.AIControlModeOff || in.Agent.AIMode == store.AIControlModeObserve {
		return pass(id, postureMedium)
	}
	var names []string
	for _, t := range liveTokens(in) {
		if t.AgentName != "" && canMutate(t.Abilities) {
			names = append(names, t.Name)
		}
	}
	if len(names) == 0 {
		return pass(id, postureMedium)
	}
	it := failing(id, postureMedium, names, linkFix("/settings/ai-control"))
	it.Detail = agentProdReachDetail
	return it
}

const agentProdReachDetail = "agent tokens can still reach routes the production guard does not cover: apply, environments"

func prodEnvUnprotectedRule(in postureInput) postureItem {
	const id = "prod_env_unprotected"
	switch {
	case !in.Agent.ProdKnown:
		return unknown(id, postureHigh, "environments could not be read")
	case len(in.Agent.ProdEnvOpen) == 0:
		return pass(id, postureHigh)
	}
	return failing(id, postureHigh, slices.Clone(in.Agent.ProdEnvOpen), linkFix("/settings/environments"))
}

func prodGuardRule(in postureInput) postureItem {
	const id = "prod_apps_unguarded"
	switch {
	case !in.Agent.GuardKnown:
		return unknown(id, postureMedium, "deploy policies or freeze windows could not be read")
	case len(in.Agent.ProdAppsNoGuard) == 0:
		return pass(id, postureMedium)
	}
	return failing(id, postureMedium, slices.Clone(in.Agent.ProdAppsNoGuard), linkFix("/settings/deploy-policies"))
}

func auditSinkRule(in postureInput) postureItem {
	const id = "audit_sink_none"
	switch {
	case !in.Agent.SinksKnown:
		return unknown(id, postureLow, "export destinations could not be read")
	case in.Agent.AuditSinks > 0:
		return pass(id, postureLow)
	}
	return failing(id, postureLow, nil, linkFix("/settings/data-export"))
}
