package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	resourcePrefixApp      = "app:"
	resourcePrefixDatabase = "database:"
)

// accessStore is the store surface environment aware authorization needs;
// *store.DB satisfies it.
type accessStore interface {
	UserVisibility(ctx context.Context, userID string) (store.Visibility, error)
	EnvironmentOfApp(ctx context.Context, appName string) (*store.EnvironmentRef, error)
	EnvironmentOfDatabase(ctx context.Context, databaseName string) (*store.EnvironmentRef, error)
	EnvironmentsOfApps(ctx context.Context) (map[string]*store.EnvironmentRef, error)
	EnvironmentsOfDatabases(ctx context.Context) (map[string]*store.EnvironmentRef, error)
}

func (rt *Router) accessDB() accessStore {
	if s, ok := rt.policies.(accessStore); ok {
		return s
	}
	if s, ok := rt.apps.(accessStore); ok {
		return s
	}
	return nil
}

// principalVisibility resolves what the caller may see: a user by its own
// role, a token by the role of the user who minted it. A token with no owner
// is a system token and unrestricted.
func (rt *Router) principalVisibility(ctx context.Context, principalType, principalID string) (store.Visibility, error) {
	db := rt.accessDB()
	if db == nil {
		return store.Visibility{}, nil
	}
	userID := principalID
	if principalType == store.PrincipalTypeToken {
		rec, err := rt.tokens.GetAPITokenByID(ctx, principalID)
		if err != nil {
			return store.Visibility{}, fmt.Errorf("api: visibility: load token: %w", err)
		}
		if rec.OwnerUserID == "" {
			return store.Visibility{}, nil
		}
		userID = rec.OwnerUserID
	}
	vis, err := db.UserVisibility(ctx, userID)
	if err != nil {
		return store.Visibility{}, fmt.Errorf("api: visibility: %w", err)
	}
	return vis, nil
}

// splitResource separates "app:web" into its kind prefix and name.
func splitResource(resource string) (prefix, name string, ok bool) {
	for _, p := range []string{resourcePrefixApp, resourcePrefixDatabase} {
		if strings.HasPrefix(resource, p) {
			return p, strings.TrimPrefix(resource, p), true
		}
	}
	return "", "", false
}

func (rt *Router) environmentForResource(ctx context.Context, prefix, name string) (*store.EnvironmentRef, error) {
	db := rt.accessDB()
	if db == nil {
		return nil, nil
	}
	if prefix == resourcePrefixDatabase {
		return db.EnvironmentOfDatabase(ctx, name)
	}
	return db.EnvironmentOfApp(ctx, name)
}

// resourceScope carries one caller's visibility and the environment of every
// app and database, loaded once so a list pays at most three queries.
type resourceScope struct {
	vis      store.Visibility
	appEnv   map[string]*store.EnvironmentRef
	dbEnv    map[string]*store.EnvironmentRef
	teams    *teamScope
	appOwner map[string]*store.ResourceOwner
	dbOwner  map[string]*store.ResourceOwner
}

func policiesMentionEnvironments(policies []store.Policy) bool {
	for _, p := range policies {
		doc, err := ParseDocument(p.Document)
		if err != nil {
			continue
		}
		for _, s := range doc.Statement {
			for _, r := range s.Resource {
				if strings.HasPrefix(r, resourcePrefixEnvironment) {
					return true
				}
			}
		}
	}
	return false
}

// newResourceScope loads environment maps only when the caller is restricted
// or holds a policy that names an environment; everyone else pays the single
// visibility lookup.
func (rt *Router) newResourceScope(ctx context.Context, principalType, principalID string, policies []store.Policy) (*resourceScope, error) {
	vis, err := rt.principalVisibility(ctx, principalType, principalID)
	if err != nil {
		return nil, err
	}
	s := &resourceScope{vis: vis}
	if s.teams, err = rt.principalTeamScope(ctx, principalType, principalID); err != nil {
		return nil, err
	}
	if err := rt.loadOwners(ctx, s); err != nil {
		return nil, err
	}
	db := rt.accessDB()
	if db == nil || (!vis.Restricted && s.teams == nil && !policiesMentionEnvironments(policies)) {
		return s, nil
	}
	if s.appEnv, err = db.EnvironmentsOfApps(ctx); err != nil {
		return nil, fmt.Errorf("api: scope: apps environments: %w", err)
	}
	if s.dbEnv, err = db.EnvironmentsOfDatabases(ctx); err != nil {
		return nil, fmt.Errorf("api: scope: databases environments: %w", err)
	}
	return s, nil
}

// restricted reports whether the scope hides anything from the caller.
func (s *resourceScope) restricted() bool { return s.vis.Restricted || s.teams != nil }

func (s *resourceScope) lookup(prefix, name string) *store.EnvironmentRef {
	if prefix == resourcePrefixDatabase {
		return s.dbEnv[name]
	}
	return s.appEnv[name]
}

// visible is false for a resource a restricted caller has no grant for,
// including every untagged resource.
func (s *resourceScope) visible(prefix, name string) bool {
	if s.teams != nil && s.teams.roleFor(s.owner(prefix, name)) == "" {
		return false
	}
	if !s.vis.Restricted {
		return true
	}
	ref := s.lookup(prefix, name)
	return ref != nil && s.vis.Allows(ref.ID)
}

func (s *resourceScope) owner(prefix, name string) *store.ResourceOwner {
	if prefix == resourcePrefixDatabase {
		return s.dbOwner[name]
	}
	return s.appOwner[name]
}

// authorize applies visibility, the team role, then the environment aware
// policy evaluation.
func (s *resourceScope) authorize(abilities []string, policies []store.Policy, ability, prefix, name string) bool {
	if !s.visible(prefix, name) {
		return false
	}
	ref := s.lookup(prefix, name)
	if s.teams != nil {
		owner := s.owner(prefix, name)
		role := s.teams.roleFor(owner)
		if protectedDenied(role, ability, ref) {
			return false
		}
		abilities = capAbilities(abilities, ownerTeam(owner), role)
	}
	return authorizeResource(abilities, policies, ability, prefix+name, environmentResources(ref)...)
}

func ownerTeam(o *store.ResourceOwner) string {
	if o == nil {
		return store.DefaultTeamID
	}
	return o.TeamID
}

// loadOwners fills the owner maps once for a team scoped caller.
func (rt *Router) loadOwners(ctx context.Context, s *resourceScope) error {
	db := rt.teamDB()
	if s.teams == nil || db == nil {
		return nil
	}
	var err error
	if s.appOwner, err = db.OwnersOfApps(ctx); err != nil {
		return fmt.Errorf("api: scope: app owners: %w", err)
	}
	if s.dbOwner, err = db.OwnersOfDatabases(ctx); err != nil {
		return fmt.Errorf("api: scope: database owners: %w", err)
	}
	return nil
}

// callerScope resolves the request's abilities, policies and scope once.
func (rt *Router) callerScope(r *http.Request) (abilities []string, policies []store.Policy, scope *resourceScope, err error) {
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve caller: %w", err)
	}
	policies, err = rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list caller policies: %w", err)
	}
	scope, err = rt.newResourceScope(r.Context(), principalType, principalID, policies)
	if err != nil {
		return nil, nil, nil, err
	}
	if narrowed := narrowTeamScope(r, scope.teams); narrowed != scope.teams {
		scope.teams = narrowed
		if err := rt.loadOwners(r.Context(), scope); err != nil {
			return nil, nil, nil, err
		}
		if db := rt.accessDB(); db != nil && scope.appEnv == nil {
			if scope.appEnv, err = db.EnvironmentsOfApps(r.Context()); err != nil {
				return nil, nil, nil, fmt.Errorf("api: scope: apps environments: %w", err)
			}
			if scope.dbEnv, err = db.EnvironmentsOfDatabases(r.Context()); err != nil {
				return nil, nil, nil, fmt.Errorf("api: scope: databases environments: %w", err)
			}
		}
	}
	return abilities, policies, scope, nil
}

// authorizeResourceInEnvironment is the resource gate's decision. For an app
// or database it also evaluates the environment the resource is tagged with.
// A resource hidden from a restricted caller reports hidden=true with
// allowed=true so the gate can answer 404 instead of leaking existence with a
// 403; callers must check hidden before running any handler.
func (rt *Router) authorizeResourceInEnvironment(ctx context.Context, principalType, principalID string, abilities []string, policies []store.Policy, ability, resource string) (allowed, hidden bool, err error) {
	prefix, name, ok := splitResource(resource)
	if !ok {
		return authorizeResource(abilities, policies, ability, resource), false, nil
	}
	vis, err := rt.principalVisibility(ctx, principalType, principalID)
	if err != nil {
		return false, false, err
	}
	teams, err := rt.principalTeamScope(ctx, principalType, principalID)
	if err != nil {
		return false, false, err
	}
	var ref *store.EnvironmentRef
	if vis.Restricted || teams != nil || policiesMentionEnvironments(policies) {
		if ref, err = rt.environmentForResource(ctx, prefix, name); err != nil {
			return false, false, fmt.Errorf("api: resource environment: %w", err)
		}
	}
	if vis.Restricted && (ref == nil || !vis.Allows(ref.ID)) {
		return true, true, nil
	}
	if teams != nil {
		owner, err := rt.ownerForResource(ctx, prefix, name)
		if err != nil {
			return false, false, fmt.Errorf("api: resource owner: %w", err)
		}
		role := teams.roleFor(owner)
		if role == "" {
			return true, true, nil
		}
		if protectedDenied(role, ability, ref) {
			return false, false, nil
		}
		abilities = capAbilities(abilities, ownerTeam(owner), role)
	}
	return authorizeResource(abilities, policies, ability, resource, environmentResources(ref)...), false, nil
}

func hiddenResourceMessage(resource string) string {
	if strings.HasPrefix(resource, resourcePrefixDatabase) {
		return "database not found"
	}
	return "app not found"
}
