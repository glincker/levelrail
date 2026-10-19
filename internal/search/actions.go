package search

import (
	"context"
	"strings"
)

// Scope says what an action targets.
type Scope string

// Action scopes.
const (
	ScopeApp      Scope = "app"
	ScopeDatabase Scope = "database"
	ScopeGlobal   Scope = "global"
)

// ActionDef is one entry of the action registry. Ability and Confirm mirror
// the route the action calls, so a hit never offers more than the API allows.
type ActionDef struct {
	ID       string
	Title    string
	Keywords string
	Scope    Scope
	Ability  string
	Confirm  bool
	Href     string
	// MCPTool is the MCP tool that performs the action; empty when none exists.
	MCPTool string
	// MCPNeeds are inputs the agent must supply on top of the target name.
	MCPNeeds []string
}

// Abilities used by the registry; the strings match internal/api's constants.
const (
	abilityRead           = "read"
	abilityWrite          = "write"
	abilityWriteSensitive = "write:sensitive"
	abilityDeploy         = "deploy"
	abilityRoot           = "root"
)

// Actions is the registry. Href may contain {name}.
var Actions = []ActionDef{
	{ID: "app.deploy", Title: "Deploy", Keywords: "deploy release ship redeploy", Scope: ScopeApp, Ability: abilityDeploy, Confirm: true, Href: "/apps/{name}?action=deploy", MCPTool: "deploy_app", MCPNeeds: []string{"image"}},
	{ID: "app.restart", Title: "Restart", Keywords: "restart reboot bounce", Scope: ScopeApp, Ability: abilityDeploy, Confirm: true, Href: "/apps/{name}?action=restart", MCPTool: "restart_app"},
	{ID: "app.logs", Title: "Open logs for", Keywords: "logs tail output", Scope: ScopeApp, Ability: abilityRead, Href: "/apps/{name}/logs", MCPTool: "get_app_logs"},
	{ID: "app.rollback", Title: "Rollback", Keywords: "rollback revert previous", Scope: ScopeApp, Ability: abilityDeploy, Confirm: true, Href: "/apps/{name}/deploys?action=rollback", MCPTool: "rollback_app_to_deploy", MCPNeeds: []string{"deploy_id"}},
	{ID: "app.stop", Title: "Stop", Keywords: "stop halt pause", Scope: ScopeApp, Ability: abilityDeploy, Confirm: true, Href: "/apps/{name}?action=stop"},
	{ID: "app.add-domain", Title: "Add a domain to", Keywords: "add domain hostname dns", Scope: ScopeApp, Ability: abilityWrite, Href: "/apps/{name}/domains", MCPTool: "set_app_domains", MCPNeeds: []string{"domains"}},
	{ID: "db.backup-now", Title: "Back up now", Keywords: "backup now snapshot dump", Scope: ScopeDatabase, Ability: abilityWriteSensitive, Confirm: true, Href: "/databases/{name}?action=backup"},
	{ID: "global.new-app", Title: "Create a new app", Keywords: "new app create deploy", Scope: ScopeGlobal, Ability: abilityWrite, Href: "/apps/new"},
	{ID: "global.add-node", Title: "Add a node", Keywords: "add node server enroll join", Scope: ScopeGlobal, Ability: abilityRoot, Href: "/nodes"},
}

// ActionsProvider emits the registry once per target resource.
type ActionsProvider struct {
	AppNames      func(ctx context.Context) ([]string, error)
	DatabaseNames func(ctx context.Context) ([]string, error)
}

// Kind implements Provider.
func (ActionsProvider) Kind() Kind { return KindAction }

// Docs implements Provider.
func (p ActionsProvider) Docs(ctx context.Context) ([]Doc, error) {
	apps, err := p.AppNames(ctx)
	if err != nil {
		return nil, err
	}
	dbs, err := p.DatabaseNames(ctx)
	if err != nil {
		return nil, err
	}
	var out []Doc
	for _, a := range Actions {
		switch a.Scope {
		case ScopeApp:
			for _, n := range apps {
				out = append(out, actionDoc(a, "app:", n, a.Title+" "+n, n))
			}
		case ScopeDatabase:
			for _, n := range dbs {
				out = append(out, actionDoc(a, "database:", n, a.Title+" "+n, ""))
			}
		default:
			out = append(out, actionDoc(a, "global:", a.ID, a.Title, ""))
		}
	}
	return out, nil
}

var actionByID = func() map[string]ActionDef {
	m := make(map[string]ActionDef, len(Actions))
	for _, a := range Actions {
		m[a.ID] = a
	}
	return m
}()

// actionCall tells an agent which MCP tool to call for an action hit and with
// what; the target name is filled in, MCPNeeds lists what it must still supply.
func actionCall(d Doc) (tool string, args map[string]string, needs []string) {
	def, ok := actionByID[d.ActionID]
	if !ok || def.MCPTool == "" {
		return "", nil, nil
	}
	return def.MCPTool, map[string]string{"name": d.Name}, def.MCPNeeds
}

func actionDoc(a ActionDef, prefix, name, title, app string) Doc {
	ref := "action:" + a.ID
	if a.Scope != ScopeGlobal {
		ref += ":" + name
	}
	return Doc{
		Ref: ref, Kind: KindAction, Title: title, Body: a.Keywords, App: app,
		Prefix: prefix, Name: name, Ability: a.Ability, Confirm: a.Confirm,
		Href: strings.ReplaceAll(a.Href, "{name}", name), ActionID: a.ID,
	}
}
