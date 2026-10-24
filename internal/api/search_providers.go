package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/search"
	"github.com/GLINCKER/levelrail/internal/store"
)

const searchDeploymentWindow = 300

// searchProviders lists every index feeder. TestSearchProvidersAllGoThroughAuthz
// holds each of them to the authorization hook.
func (rt *Router) searchProviders() []search.Provider {
	return []search.Provider{
		rt.appSearchProvider(),
		rt.domainSearchProvider(),
		funcProvider{search.KindDatabase, rt.databaseSearchDocs},
		funcProvider{search.KindNode, rt.nodeSearchDocs},
		funcProvider{search.KindProject, rt.projectSearchDocs},
		funcProvider{search.KindEnv, rt.environmentSearchDocs},
		funcProvider{search.KindTag, rt.tagSearchDocs},
		funcProvider{search.KindDeployment, rt.deploymentSearchDocs},
		funcProvider{search.KindTemplate, templateSearchDocs},
		funcProvider{search.KindSetting, settingSearchDocs},
		funcProvider{search.KindDoc, docSearchDocs},
		search.ActionsProvider{AppNames: rt.searchAppNames, DatabaseNames: rt.searchDatabaseNames},
	}
}

type funcProvider struct {
	kind search.Kind
	fn   func(context.Context) ([]search.Doc, error)
}

func (p funcProvider) Kind() search.Kind                              { return p.kind }
func (p funcProvider) Docs(ctx context.Context) ([]search.Doc, error) { return p.fn(ctx) }

func (rt *Router) searchAppNames(ctx context.Context) ([]string, error) {
	svcs, err := rt.listPublicApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	out := make([]string, len(svcs))
	for i, s := range svcs {
		out[i] = s.Name
	}
	return out, nil
}

func (rt *Router) searchDatabaseNames(ctx context.Context) ([]string, error) {
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	out := make([]string, len(dbs))
	for i, d := range dbs {
		out[i] = d.Name
	}
	return out, nil
}

type appDocsProvider struct{ rt *Router }

func (rt *Router) appSearchProvider() search.Provider { return appDocsProvider{rt} }

func (appDocsProvider) Kind() search.Kind { return search.KindApp }

func (p appDocsProvider) Docs(ctx context.Context) ([]search.Doc, error) {
	svcs, err := p.rt.listPublicApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	names := make([]string, len(svcs))
	for i, s := range svcs {
		names[i] = s.Name
	}
	tags, err := p.rt.tags.ListTagsForApps(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("list app tags: %w", err)
	}
	envs := p.rt.searchAppEnvironments(ctx)
	nodes := p.rt.searchNodeNames(ctx)
	out := make([]search.Doc, 0, len(svcs))
	for _, s := range svcs {
		body := strings.TrimSpace(s.Image + " " + strings.Join(s.Domains, " "))
		var tagNames []string
		for _, t := range tags[s.Name] {
			tagNames = append(tagNames, t.Name)
		}
		out = append(out, search.Doc{
			Ref: "app:" + s.Name, Kind: search.KindApp, Title: s.Name, Body: body, Tags: tagNames,
			Env: envs[s.Name], Node: nodes[s.NodeID],
			Prefix: resourcePrefixApp, Name: s.Name, Ability: AbilityRead, Href: "/apps/" + s.Name,
		})
	}
	return out, nil
}

func (rt *Router) searchAppEnvironments(ctx context.Context) map[string]string {
	out := map[string]string{}
	db := rt.accessDB()
	if db == nil {
		return out
	}
	m, err := db.EnvironmentsOfApps(ctx)
	if err != nil {
		return out
	}
	for app, ref := range m {
		if ref != nil {
			out[app] = ref.Name
		}
	}
	return out
}

func (rt *Router) searchNodeNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return out
	}
	for _, n := range nodes {
		out[n.ID] = n.Name
	}
	return out
}

type domainDocsProvider struct{ rt *Router }

func (rt *Router) domainSearchProvider() search.Provider { return domainDocsProvider{rt} }

func (domainDocsProvider) Kind() search.Kind { return search.KindDomain }

func (p domainDocsProvider) Docs(ctx context.Context) ([]search.Doc, error) {
	svcs, err := p.rt.listPublicApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	var out []search.Doc
	for _, s := range svcs {
		for _, d := range s.Domains {
			out = append(out, search.Doc{
				Ref: "domain:" + d, Kind: search.KindDomain, Title: d, Body: "serves " + s.Name, App: s.Name,
				Prefix: resourcePrefixApp, Name: s.Name, Ability: AbilityRead, Href: "/apps/" + s.Name + "/domains",
			})
		}
	}
	return out, nil
}

func (rt *Router) databaseSearchDocs(ctx context.Context) ([]search.Doc, error) {
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	out := make([]search.Doc, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, search.Doc{
			Ref: "database:" + d.Name, Kind: search.KindDatabase, Title: d.Name, Body: strings.TrimSpace(d.Engine + " " + d.Version),
			Prefix: resourcePrefixDatabase, Name: d.Name, Ability: AbilityRead, Href: "/databases/" + d.Name,
		})
	}
	return out, nil
}

// Node listing is root only (GET /api/v1/nodes), so node docs require root.
func (rt *Router) nodeSearchDocs(ctx context.Context) ([]search.Doc, error) {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	out := make([]search.Doc, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, search.Doc{
			Ref: "node:" + n.ID, Kind: search.KindNode, Title: n.Name, Body: strings.TrimSpace(n.Address + " " + string(n.Status)),
			Prefix: resourcePrefixNode, Name: n.ID, Ability: AbilityRoot, Href: "/nodes/" + n.ID,
		})
	}
	return out, nil
}

func (rt *Router) projectSearchDocs(ctx context.Context) ([]search.Doc, error) {
	ps, err := rt.projects.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]search.Doc, 0, len(ps))
	for _, p := range ps {
		out = append(out, search.Doc{
			Ref: "project:" + p.ID, Kind: search.KindProject, Title: p.Name,
			Prefix: searchPrefixProject, Name: p.ID, Ability: AbilityRead, Href: "/projects/" + p.ID,
		})
	}
	return out, nil
}

func (rt *Router) environmentSearchDocs(ctx context.Context) ([]search.Doc, error) {
	envs, err := rt.environments.ListAllEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	out := make([]search.Doc, 0, len(envs))
	for _, e := range envs {
		out = append(out, search.Doc{
			Ref: "environment:" + e.ID, Kind: search.KindEnv, Title: e.Name, Body: e.Kind, Env: e.Name,
			Prefix: resourcePrefixEnvironment, Name: e.ID, Ability: AbilityRead, Href: "/settings/environments",
		})
	}
	return out, nil
}

func (rt *Router) tagSearchDocs(ctx context.Context) ([]search.Doc, error) {
	tags, err := rt.tags.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	out := make([]search.Doc, 0, len(tags))
	for _, t := range tags {
		out = append(out, search.Doc{
			Ref: "tag:" + t.ID, Kind: search.KindTag, Title: t.Name, Body: "tag",
			Prefix: searchPrefixGlobal, Name: "tag:" + t.ID, Ability: AbilityRead, Href: "/apps?tag=" + t.Name,
		})
	}
	return out, nil
}

// Deployments inherit their app's visibility, so a hidden app hides its deploys.
func (rt *Router) deploymentSearchDocs(ctx context.Context) ([]search.Doc, error) {
	ds, ok := rt.deployAttempts.(deploymentStore)
	if !ok {
		return nil, nil
	}
	rows, _, err := ds.ListDeployments(ctx, store.DeploymentFilter{Limit: searchDeploymentWindow})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	out := make([]search.Doc, 0, len(rows))
	for _, d := range rows {
		a := d.Attempt
		body := strings.TrimSpace(strings.Join([]string{d.Status, a.Branch, shortSHA(a.CommitSHA), a.CommitMessage, d.Trigger}, " "))
		out = append(out, search.Doc{
			Ref: "deployment:" + a.ID, Kind: search.KindDeployment, Title: "Deploy " + a.ServiceName + " " + shortSHA(a.CommitSHA),
			Body: body, App: a.ServiceName, Env: d.Environment, UpdatedAt: a.StartedAt,
			Prefix: resourcePrefixApp, Name: a.ServiceName, Ability: AbilityRead, Href: "/apps/" + a.ServiceName + "/deploys",
		})
	}
	return out, nil
}

func templateSearchDocs(context.Context) ([]search.Doc, error) {
	out := make([]search.Doc, 0, len(catalog.Templates))
	for _, t := range catalog.Templates {
		out = append(out, search.Doc{
			Ref: "template:" + t.ID, Kind: search.KindTemplate, Title: t.Name, Body: strings.TrimSpace(t.Slogan + " " + t.Category),
			Prefix: searchPrefixGlobal, Name: "template:" + t.ID, Ability: AbilityRead, Href: "/templates/" + t.ID,
		})
	}
	return out, nil
}

type staticPage struct{ id, title, body, href, ability string }

var settingPages = []staticPage{
	{"general", "General settings", "dashboard url instance name", "/settings/general", AbilityRoot},
	{"security", "Security center", "security hardening passkeys two factor", "/settings/security", AbilityRoot},
	{"account", "My account", "profile password passkeys sessions tokens", "/settings/account", AbilityRead},
	{"roles", "Roles and access", "roles users invite permissions", "/settings/roles", AbilityRoot},
	{"iam", "IAM policies", "iam policy allow deny", "/settings/iam-policies", AbilityRoot},
	{"audit", "Audit log", "audit history who changed", "/settings/audit-log", AbilityRoot},
	{"certificates", "Certificates", "tls certificates acme renew", "/settings/certificates", AbilityRoot},
	{"backup-targets", "Backup targets", "backups s3 storage destination", "/settings/backup-targets", AbilityRoot},
	{"cp-backup", "Control plane backup", "disaster recovery backup restore", "/settings/control-plane-backup", AbilityRoot},
	{"notifications", "Notification channels", "slack discord email webhook alerts", "/settings/notification-channels", AbilityRoot},
	{"email", "Email", "smtp mail sender", "/settings/email", AbilityRoot},
	{"sso", "Single sign-on", "sso saml oidc", "/settings/sso", AbilityRoot},
	{"oauth", "OAuth providers", "oauth github google login", "/settings/oauth", AbilityRoot},
	{"environments", "Environments", "environments production staging", "/settings/environments", AbilityRead},
	{"janitor", "Cleanup: disk and Docker", "janitor cleanup disk images volumes docker", "/settings/cleanup?tab=disk", AbilityRoot},
	{"cleanup", "Cleanup", "cleanup retention data logs audit tokens history", "/settings/cleanup", AbilityRoot},
	{"maintenance", "Maintenance", "maintenance windows scheduled", "/settings/maintenance", AbilityRoot},
	{"cli-access", "CLI access", "cli device login token", "/settings/cli-access", AbilityRead},
	{"api-explorer", "API explorer", "api openapi endpoints", "/settings/api-explorer", AbilityRead},
	{"plugins", "Plugins", "plugins marketplace extensions", "/settings/plugins", AbilityRoot},
	{"beta", "Beta features", "experimental feature flags", "/settings/beta-features", AbilityRoot},
	{"registry", "Registry credentials", "docker registry credentials", "/settings/registry-credentials", AbilityRoot},
	{"status-page", "Status page", "public status page uptime", "/settings/status-page", AbilityRoot},
	{"deploy-policies", "Deploy policies", "deploy gate freeze approval", "/settings/deploy-policies", AbilityRoot},
	{"observability", "Observability", "metrics logs retention", "/settings/observability", AbilityRoot},
	{"node-providers", "Node providers", "cloud provider hetzner digitalocean provision", "/settings/node-providers", AbilityRoot},
}

func settingSearchDocs(context.Context) ([]search.Doc, error) {
	return staticDocs(search.KindSetting, "setting:", settingPages), nil
}

var docPages = []staticPage{
	{"rollback", "Rollback runbook", "roll back a bad deploy previous release image", "/help", AbilityRead},
	{"backups", "Backups runbook", "database backup restore verify schedule", "/help", AbilityRead},
	{"domains", "Domains runbook", "domain dns certificate go live", "/help", AbilityRead},
	{"upgrades", "Upgrades runbook", "upgrade control plane agent version", "/help", AbilityRead},
	{"access-control", "Access control", "roles iam policies teams visibility", "/help", AbilityRead},
	{"benchmarks", "Benchmarks", "performance load numbers", "/help", AbilityRead},
}

func docSearchDocs(context.Context) ([]search.Doc, error) {
	return staticDocs(search.KindDoc, "doc:", docPages), nil
}

func staticDocs(kind search.Kind, refPrefix string, pages []staticPage) []search.Doc {
	out := make([]search.Doc, 0, len(pages))
	for _, p := range pages {
		out = append(out, search.Doc{
			Ref: refPrefix + p.id, Kind: kind, Title: p.title, Body: p.body, UpdatedAt: time.Time{},
			Prefix: searchPrefixGlobal, Name: refPrefix + p.id, Ability: p.ability, Href: p.href,
		})
	}
	return out
}
