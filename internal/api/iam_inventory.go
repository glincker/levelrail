package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	resourceKindApp      = "app"
	resourceKindDatabase = "database"
	matchSampleLimit     = 8
)

// iamInventory is every app, database and environment, loaded once per request.
type iamInventory struct {
	apps      []string
	databases []string
	appEnv    map[string]*store.EnvironmentRef
	dbEnv     map[string]*store.EnvironmentRef
	envs      []store.EnvironmentWithCounts
	projects  []store.Project
}

func (rt *Router) loadIAMInventory(ctx context.Context) (*iamInventory, error) {
	inv := &iamInventory{}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam inventory: list apps: %w", err)
	}
	for _, s := range svcs {
		inv.apps = append(inv.apps, s.Name)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam inventory: list databases: %w", err)
	}
	for _, d := range dbs {
		inv.databases = append(inv.databases, d.Name)
	}
	sort.Strings(inv.apps)
	sort.Strings(inv.databases)
	if db := rt.accessDB(); db != nil {
		if inv.appEnv, err = db.EnvironmentsOfApps(ctx); err != nil {
			return nil, fmt.Errorf("iam inventory: app environments: %w", err)
		}
		if inv.dbEnv, err = db.EnvironmentsOfDatabases(ctx); err != nil {
			return nil, fmt.Errorf("iam inventory: database environments: %w", err)
		}
	}
	if rt.environments != nil {
		if inv.envs, err = rt.environments.ListAllEnvironments(ctx); err != nil {
			return nil, fmt.Errorf("iam inventory: environments: %w", err)
		}
	}
	if rt.projects != nil {
		if inv.projects, err = rt.projects.ListProjects(ctx); err != nil {
			return nil, fmt.Errorf("iam inventory: projects: %w", err)
		}
	}
	return inv, nil
}

func (inv *iamInventory) envFor(prefix, name string) *store.EnvironmentRef {
	if prefix == resourcePrefixDatabase {
		return inv.dbEnv[name]
	}
	return inv.appEnv[name]
}

// resourceExists reports whether a concrete (non wildcard) resource identifier
// still refers to something. Anything it does not recognize counts as existing.
func (inv *iamInventory) resourceExists(r string) bool {
	switch {
	case strings.HasPrefix(r, resourcePrefixApp):
		return slices.Contains(inv.apps, strings.TrimPrefix(r, resourcePrefixApp))
	case strings.HasPrefix(r, resourcePrefixDatabase):
		return slices.Contains(inv.databases, strings.TrimPrefix(r, resourcePrefixDatabase))
	case strings.HasPrefix(r, resourcePrefixEnvironmentKind):
		return slices.Contains(environmentKinds, strings.TrimPrefix(r, resourcePrefixEnvironmentKind))
	case strings.HasPrefix(r, resourcePrefixEnvironment):
		id := strings.TrimPrefix(r, resourcePrefixEnvironment)
		return slices.ContainsFunc(inv.envs, func(e store.EnvironmentWithCounts) bool { return e.ID == id })
	}
	return true
}

type resourceMatch struct {
	Pattern   string   `json:"pattern"`
	Apps      int      `json:"apps"`
	Databases int      `json:"databases"`
	Sample    []string `json:"sample"`
	Error     string   `json:"error,omitempty"`
}

// matchPattern counts what one resource pattern selects, through the same
// statementMatchesAny the evaluator uses.
func (inv *iamInventory) matchPattern(pattern string) resourceMatch {
	m := resourceMatch{Pattern: pattern, Sample: []string{}}
	if strings.TrimSpace(pattern) == "" {
		m.Error = errStatementNoResource.Error()
		return m
	}
	if err := validateEnvironmentResource(pattern); err != nil {
		m.Error = err.Error()
		return m
	}
	s := Statement{Effect: EffectAllow, Action: []string{"*"}, Resource: []string{pattern}}
	scan := func(prefix string, names []string) int {
		n := 0
		for _, name := range names {
			if !statementMatchesAny(s, AbilityRead, prefix+name, environmentResources(inv.envFor(prefix, name))) {
				continue
			}
			n++
			if len(m.Sample) < matchSampleLimit {
				m.Sample = append(m.Sample, prefix+name)
			}
		}
		return n
	}
	m.Apps = scan(resourcePrefixApp, inv.apps)
	m.Databases = scan(resourcePrefixDatabase, inv.databases)
	return m
}

type iamResourcesResponse struct {
	Projects     []iamProjectRef     `json:"projects"`
	Environments []iamEnvironmentRef `json:"environments"`
	Apps         []iamResourceRef    `json:"apps"`
	Databases    []iamResourceRef    `json:"databases"`
}

type iamProjectRef struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Environments []string `json:"environment_ids"`
}

type iamEnvironmentRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ProjectID string `json:"project_id"`
	Apps      int    `json:"apps"`
	Databases int    `json:"databases"`
}

type iamResourceRef struct {
	Name          string `json:"name"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Kind          string `json:"environment_kind,omitempty"`
}

func (inv *iamInventory) resourcesResponse() iamResourcesResponse {
	out := iamResourcesResponse{Projects: []iamProjectRef{}, Environments: []iamEnvironmentRef{}, Apps: []iamResourceRef{}, Databases: []iamResourceRef{}}
	for _, e := range inv.envs {
		out.Environments = append(out.Environments, iamEnvironmentRef{ID: e.ID, Name: e.Name, Kind: e.Kind, ProjectID: e.ProjectID, Apps: e.AppCount, Databases: e.DatabaseCount})
	}
	for _, p := range inv.projects {
		ref := iamProjectRef{ID: p.ID, Name: p.Name, Environments: []string{}}
		for _, e := range inv.envs {
			if e.ProjectID == p.ID {
				ref.Environments = append(ref.Environments, e.ID)
			}
		}
		out.Projects = append(out.Projects, ref)
	}
	for _, n := range inv.apps {
		out.Apps = append(out.Apps, resourceRefFor(n, inv.appEnv[n]))
	}
	for _, n := range inv.databases {
		out.Databases = append(out.Databases, resourceRefFor(n, inv.dbEnv[n]))
	}
	return out
}

func resourceRefFor(name string, ref *store.EnvironmentRef) iamResourceRef {
	r := iamResourceRef{Name: name}
	if ref != nil {
		r.EnvironmentID, r.Kind = ref.ID, ref.Kind
	}
	return r
}

// handleIAMResources handles GET /api/v1/iam/resources: the picker's inventory.
func (rt *Router) handleIAMResources(w http.ResponseWriter, r *http.Request) {
	inv, err := rt.loadIAMInventory(r.Context())
	if err != nil {
		rt.internalError(w, "api: iam resources failed", err)
		return
	}
	writeJSON(w, http.StatusOK, inv.resourcesResponse())
}

type matchRequest struct {
	Resources []string `json:"resources"`
}

// handleIAMMatch handles POST /api/v1/iam/resources/match: live match counts
// for resource patterns. It persists nothing.
func (rt *Router) handleIAMMatch(w http.ResponseWriter, r *http.Request) {
	var req matchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errInvalidPolicyRequestBody)
		return
	}
	inv, err := rt.loadIAMInventory(r.Context())
	if err != nil {
		rt.internalError(w, "api: iam match failed", err)
		return
	}
	out := make([]resourceMatch, 0, len(req.Resources))
	for _, p := range req.Resources {
		out = append(out, inv.matchPattern(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// iamPrincipal is a user or token with the flat abilities the evaluator starts from.
type iamPrincipal struct {
	Type       string     `json:"principal_type"`
	ID         string     `json:"principal_id"`
	Name       string     `json:"name"`
	Detail     string     `json:"detail,omitempty"`
	Abilities  []string   `json:"abilities"`
	Active     bool       `json:"active"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Policies   []string   `json:"policy_ids"`
}

func (p iamPrincipal) isRoot() bool { return slices.Contains(p.Abilities, AbilityRoot) }

func (rt *Router) loadIAMPrincipals(ctx context.Context) ([]iamPrincipal, error) {
	users, err := rt.auth.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam principals: list users: %w", err)
	}
	tokens, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam principals: list tokens: %w", err)
	}
	now := time.Now()
	out := make([]iamPrincipal, 0, len(users)+len(tokens))
	for _, u := range users {
		name := u.DisplayName
		if name == "" {
			name = u.Email
		}
		out = append(out, iamPrincipal{Type: store.PrincipalTypeUser, ID: u.ID, Name: name, Detail: u.Email, Abilities: nonNilStrings(u.Abilities), Active: true, LastUsedAt: u.LastLoginAt, Policies: []string{}})
	}
	for _, t := range tokens {
		active := t.RevokedAt == nil && (t.ExpiresAt == nil || t.ExpiresAt.After(now))
		out = append(out, iamPrincipal{Type: store.PrincipalTypeToken, ID: t.ID, Name: t.Name, Detail: t.AgentName, Abilities: nonNilStrings(t.Abilities), Active: active, LastUsedAt: t.LastUsedAt, Policies: []string{}})
	}
	return out, nil
}

func findPrincipal(all []iamPrincipal, ptype, id string) (iamPrincipal, bool) {
	i := slices.IndexFunc(all, func(p iamPrincipal) bool { return p.Type == ptype && p.ID == id })
	if i < 0 {
		return iamPrincipal{}, false
	}
	return all[i], true
}

// handleIAMPrincipals handles GET /api/v1/iam/principals: every user and token
// with the policies attached to each.
func (rt *Router) handleIAMPrincipals(w http.ResponseWriter, r *http.Request) {
	all, err := rt.loadIAMPrincipals(r.Context())
	if err != nil {
		rt.internalError(w, "api: iam principals failed", err)
		return
	}
	for i := range all {
		pol, err := rt.policies.ListPoliciesForPrincipal(r.Context(), all[i].Type, all[i].ID)
		if err != nil {
			rt.internalError(w, "api: iam principals: list policies failed", err, slog.String("principal_id", all[i].ID))
			return
		}
		for _, p := range pol {
			all[i].Policies = append(all[i].Policies, p.ID)
		}
	}
	writeJSON(w, http.StatusOK, all)
}
