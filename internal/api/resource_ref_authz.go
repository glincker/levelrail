package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type refKind string

const (
	refKindApp         refKind = "app"
	refKindDatabase    refKind = "database"
	refKindEnvironment refKind = "environment"
	refKindNode        refKind = "node"
	refKindProject     refKind = "project"
	refKindModel       refKind = "model"
	refKindAssetBucket refKind = "asset_bucket"
)

// resourceRef is a resource a request names in its body or query rather
// than its path.
type resourceRef struct {
	kind refKind
	id   string
}

func appRef(name string) resourceRef            { return resourceRef{refKindApp, name} }
func databaseRef(name string) resourceRef       { return resourceRef{refKindDatabase, name} }
func environmentRef(id string) resourceRef      { return resourceRef{refKindEnvironment, id} }
func nodeRef(id string) resourceRef             { return resourceRef{refKindNode, id} }
func modelRef(name string) resourceRef          { return resourceRef{refKindModel, name} }
func projectRef(id string) resourceRef          { return resourceRef{refKindProject, id} }
func assetBucketRef(id string) resourceRef      { return resourceRef{refKindAssetBucket, id} }
func appRefs(names []string) []resourceRef      { return mapRefs(names, appRef) }
func databaseRefs(names []string) []resourceRef { return mapRefs(names, databaseRef) }

func mapRefs(ids []string, f func(string) resourceRef) []resourceRef {
	out := make([]resourceRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, f(id))
	}
	return out
}

func (ref resourceRef) notFoundMessage() string {
	return string(ref.kind) + " not found"
}

// refDenial is the answer for a refused reference. Callers pick their own
// unknown-resource status and message so a refusal is not an existence oracle.
type refDenial struct {
	status int
	msg    string
}

func notFoundAs(status int, msg string) refDenial { return refDenial{status, msg} }

// authorizeRefs checks every referenced resource with the same engine as the
// path routes and writes deny on the first refusal. Empty ids are skipped.
func (rt *Router) authorizeRefs(w http.ResponseWriter, r *http.Request, ability string, deny refDenial, refs ...resourceRef) bool {
	denied, err := rt.firstDeniedRef(r, ability, refs)
	if err != nil {
		rt.internalError(w, "api: resource ref authz", err)
		return false
	}
	if denied == nil {
		return true
	}
	msg := deny.msg
	if msg == "" {
		msg = denied.notFoundMessage()
	}
	writeError(w, deny.status, msg)
	return false
}

func (rt *Router) firstDeniedRef(r *http.Request, ability string, refs []resourceRef) (*resourceRef, error) {
	refs = slices.DeleteFunc(slices.Clone(refs), func(ref resourceRef) bool { return ref.id == "" })
	if len(refs) == 0 {
		return nil, nil
	}
	c, err := rt.newRefCaller(r)
	if err != nil {
		return nil, err
	}
	for i := range refs {
		ok, err := rt.refAllowed(r.Context(), c, ability, refs[i])
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", refs[i].kind, refs[i].id, err)
		}
		if !ok {
			return &refs[i], nil
		}
	}
	return nil, nil
}

func (rt *Router) newRefCaller(r *http.Request) (*refCaller, error) {
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, fmt.Errorf("resolve caller: %w", err)
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	return &refCaller{principalType: principalType, principalID: principalID, abilities: abilities, policies: policies}, nil
}

// authorizeReadableServices drops the services the caller may not read, the
// same answer a filtered list gives.
func (rt *Router) authorizeReadableServices(r *http.Request, svcs []store.DesiredService) ([]store.DesiredService, error) {
	if len(svcs) == 0 {
		return svcs, nil
	}
	c, err := rt.newRefCaller(r)
	if err != nil {
		return nil, err
	}
	out := make([]store.DesiredService, 0, len(svcs))
	for _, svc := range svcs {
		ok, err := rt.refAllowed(r.Context(), c, AbilityRead, appRef(svc.Name))
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", svc.Name, err)
		}
		if ok {
			out = append(out, svc)
		}
	}
	return out, nil
}

type refCaller struct {
	principalType, principalID string
	abilities                  []string
	policies                   []store.Policy
	teamsLoaded                bool
	teams                      *teamScope
	visLoaded                  bool
	vis                        store.Visibility
}

func (rt *Router) refAllowed(ctx context.Context, c *refCaller, ability string, ref resourceRef) (bool, error) {
	switch ref.kind {
	case refKindApp, refKindDatabase:
		allowed, hidden, err := rt.authorizeResourceInEnvironment(ctx, c.principalType, c.principalID, c.abilities, c.policies, ability, string(ref.kind)+":"+ref.id)
		return allowed && !hidden, err
	case refKindNode:
		return authorizeResource(c.abilities, c.policies, ability, resourcePrefixNode+ref.id), nil
	case refKindModel:
		return authorizeResource(c.abilities, c.policies, ability, "model:"+ref.id), nil
	case refKindEnvironment:
		return rt.environmentRefAllowed(ctx, c, ability, ref.id)
	case refKindProject:
		return rt.projectRefAllowed(ctx, c, ability, ref.id)
	case refKindAssetBucket:
		return rt.assetBucketAllowed(ctx, c.abilities, c.policies, ability, ref.id), nil
	}
	return false, fmt.Errorf("unknown resource kind %q", ref.kind)
}

// environmentRefAllowed mirrors the team gate on /environments/{id}, plus
// environment visibility and IAM on environment and environment-kind.
func (rt *Router) environmentRefAllowed(ctx context.Context, c *refCaller, ability, id string) (bool, error) {
	if !c.teamsLoaded {
		teams, err := rt.principalTeamScope(ctx, c.principalType, c.principalID)
		if err != nil {
			return false, err
		}
		c.teams, c.teamsLoaded = teams, true
	}
	abilities := c.abilities
	if c.teams != nil {
		owner, err := rt.ownerOfObject(ctx, string(refKindEnvironment), id)
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		role := c.teams.roleFor(&owner)
		if role == "" {
			return false, nil
		}
		abilities = capAbilities(abilities, owner.TeamID, role)
	}
	if !c.visLoaded {
		vis, err := rt.principalVisibility(ctx, c.principalType, c.principalID)
		if err != nil {
			return false, err
		}
		c.vis, c.visLoaded = vis, true
	}
	if !c.vis.Allows(id) {
		return false, nil
	}
	envRef := &store.EnvironmentRef{ID: id}
	if rt.environments != nil {
		if env, err := rt.environments.GetEnvironment(ctx, id); err == nil {
			envRef.Kind = env.Kind
		}
	}
	extra := environmentResources(envRef)
	return authorizeResource(abilities, c.policies, ability, extra[0], extra[1:]...), nil
}

// projectRefAllowed applies the team gate and IAM to a project named in a body.
func (rt *Router) projectRefAllowed(ctx context.Context, c *refCaller, ability, id string) (bool, error) {
	if !c.teamsLoaded {
		teams, err := rt.principalTeamScope(ctx, c.principalType, c.principalID)
		if err != nil {
			return false, err
		}
		c.teams, c.teamsLoaded = teams, true
	}
	abilities := c.abilities
	if c.teams != nil {
		owner, err := rt.ownerOfObject(ctx, string(refKindProject), id)
		if errors.Is(err, store.ErrProjectNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		role := c.teams.roleFor(&owner)
		if role == "" {
			return false, nil
		}
		abilities = capAbilities(abilities, owner.TeamID, role)
	}
	return authorizeResource(abilities, c.policies, ability, "project:"+id), nil
}

// authorizePlacementTarget is validatePlacementTarget for a request: a node
// an IAM Deny hides from the caller reads as unknown.
func (rt *Router) authorizePlacementTarget(r *http.Request, nodeID string) error {
	denied, err := rt.firstDeniedRef(r, AbilityWrite, []resourceRef{nodeRef(nodeID)})
	if err != nil {
		return err
	}
	if denied != nil {
		return store.ErrNodeNotFound
	}
	return rt.validatePlacementTarget(r.Context(), nodeID)
}

// authorizeRefPredicate returns a per-reference check that loads the caller
// once, for filtering a server-computed plan.
func (rt *Router) authorizeRefPredicate(r *http.Request, ability string) (func(resourceRef) (bool, error), error) {
	c, err := rt.newRefCaller(r)
	if err != nil {
		return nil, err
	}
	return func(ref resourceRef) (bool, error) {
		if ref.id == "" {
			return true, nil
		}
		return rt.refAllowed(r.Context(), c, ability, ref)
	}, nil
}

// authorizeServiceNames refuses a multi-service deploy under appName that
// would overwrite an existing service outside that app the caller may not
// write. It answers like a create name conflict, which is no new oracle.
func (rt *Router) authorizeServiceNames(w http.ResponseWriter, r *http.Request, appName string, names []string) bool {
	previous, err := rt.appGroups.ListServicesByApp(r.Context(), appName)
	if err != nil {
		rt.internalError(w, "api: service names: list members", err)
		return false
	}
	member := map[string]bool{appName: true}
	for _, p := range previous {
		member[p.Name] = true
	}
	for _, n := range names {
		if member[n] {
			continue
		}
		if _, err := rt.apps.GetDesiredService(r.Context(), n); errors.Is(err, store.ErrServiceNotFound) {
			continue
		} else if err != nil {
			rt.internalError(w, "api: service names: check existing service", err)
			return false
		}
		if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusConflict, "an app named "+n+" already exists"), appRef(n)) {
			return false
		}
	}
	return true
}

// specServiceNames mirrors deploy.DeploySpec's service naming.
func specServiceNames(appName, single string, services map[string]spec.Service) []string {
	if single != "" {
		return []string{single}
	}
	out := make([]string, 0, len(services))
	for key := range services {
		out = append(out, appName+"-"+key)
	}
	return out
}
