package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/glincker/levelrail/internal/connhub"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// bindingStore is the write surface for connection bindings; stores without it answer 501.
type bindingStore interface {
	CreateConnectionBinding(ctx context.Context, b store.ConnectionBinding) (store.ConnectionBinding, error)
	DeleteConnectionBinding(ctx context.Context, id string) error
	ListConnectionBindingsByTarget(ctx context.Context, targetType, targetID string) ([]store.ConnectionBinding, error)
	ListConnectionBindingsByRef(ctx context.Context, ref string) ([]store.ConnectionBinding, error)
}

type bindingRequest struct {
	Ref   string          `json:"ref"`
	Role  string          `json:"role"`
	Rules json.RawMessage `json:"rules,omitempty"`
}

type orgBindingsResponse struct {
	Bindings []connhub.Binding `json:"bindings"`
}

const (
	msgConnectionNotFound = "connection not found"
	msgBindingNotFound    = "binding not found"
	// defaultBindingRulesMaxBytes applies only when no settings service is wired (tests, minimal stores).
	defaultBindingRulesMaxBytes = 4096
)

func (rt *Router) bindingStore(w http.ResponseWriter) (bindingStore, bool) {
	bs, ok := any(rt.apps).(bindingStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "connection bindings are not available on this store")
	}
	return bs, ok
}

func (rt *Router) bindingRulesMaxBytes(ctx context.Context) int {
	if rt.runtimeSettings == nil {
		return defaultBindingRulesMaxBytes
	}
	if n := rt.runtimeSettings.Int(ctx, runtimesettings.KeyConnectionBindingRulesMaxBytes); n > 0 {
		return n
	}
	return defaultBindingRulesMaxBytes
}

func (rt *Router) bindingRuleLimits(ctx context.Context) connhub.RuleLimits {
	lim := connhub.RuleLimits{MaxBytes: rt.bindingRulesMaxBytes(ctx), MaxValueLen: connhub.DefaultRuleValueMaxLen}
	if rt.runtimeSettings != nil {
		if n := rt.runtimeSettings.Int(ctx, runtimesettings.KeyConnectionBindingRuleValueMaxLen); n > 0 {
			lim.MaxValueLen = n
		}
	}
	return lim
}

func bindingView(b store.ConnectionBinding) connhub.Binding {
	return connhub.Binding{ID: b.ID, Ref: b.Ref, TargetType: b.TargetType, TargetID: b.TargetID, Role: b.Role, Rules: b.Rules, Origin: "binding"}
}

// appTeam returns the org that owns an app; apps outside any project belong to the default org.
func (rt *Router) appTeam(ctx context.Context, name string) (string, error) {
	db := rt.teamDB()
	if db == nil {
		return store.DefaultTeamID, nil
	}
	o, err := db.OwnerOfApp(ctx, name)
	if err != nil {
		return "", err
	}
	if o == nil || o.TeamID == "" {
		return store.DefaultTeamID, nil
	}
	return o.TeamID, nil
}

// bindingCallerScope is the gate's team scope when present, else resolved fresh.
func (rt *Router) bindingCallerScope(r *http.Request) (*teamScope, []string, error) {
	pt, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, nil, err
	}
	if s, ok := callerTeamScope(r.Context()); ok {
		return s, abilities, nil
	}
	s, err := rt.principalTeamScope(r.Context(), pt, pid)
	return s, abilities, err
}

// connectionReadable is "may this caller read the connection": instance
// connections need instance read:sensitive, org ones a role in that org that
// grants it, app ones read:sensitive on that app.
// A scope whose app or org was deleted has no id and reads like an instance one.
func (rt *Router) connectionReadable(r *http.Request, c connhub.Connection) (bool, error) {
	if c.ScopeType == store.ConnectionScopeApp && c.ScopeID != "" {
		denied, err := rt.firstDeniedRef(r, AbilityReadSensitive, []resourceRef{appRef(c.ScopeID)})
		return denied == nil, err
	}
	scope, abilities, err := rt.bindingCallerScope(r)
	if err != nil {
		return false, err
	}
	if c.ScopeType == store.ConnectionScopeOrg && c.ScopeID != "" {
		if scope == nil {
			return hasAbility(abilities, AbilityReadSensitive), nil
		}
		return hasAbility(capAbilities(abilities, c.ScopeID, scope.roleOfTeam(c.ScopeID)), AbilityReadSensitive), nil
	}
	if scope != nil && !scope.instance {
		return false, nil
	}
	return hasAbility(abilities, AbilityReadSensitive), nil
}

// orgAdministrable is "may this caller manage the org": an admin or owner
// role there with write:sensitive, or an unscoped instance caller holding it.
func (rt *Router) orgAdministrable(r *http.Request, orgID string) (bool, error) {
	scope, abilities, err := rt.bindingCallerScope(r)
	if err != nil {
		return false, err
	}
	if scope == nil {
		return hasAbility(abilities, AbilityWriteSensitive), nil
	}
	role := scope.roleOfTeam(orgID)
	return store.TeamRoleAtLeast(role, store.TeamRoleAdmin) && hasAbility(capAbilities(abilities, orgID, role), AbilityWriteSensitive), nil
}

// instanceAdministrable is "may this caller manage instance wide state": root,
// an unscoped caller with write:sensitive, or an admin of the default org.
func (rt *Router) instanceAdministrable(r *http.Request) (bool, error) {
	if rt.callerIsRoot(r) {
		return true, nil
	}
	scope, _, err := rt.bindingCallerScope(r)
	if err != nil {
		return false, err
	}
	if scope != nil && !scope.instance {
		return false, nil
	}
	return rt.orgAdministrable(r, store.DefaultTeamID)
}

// instanceScoped reports a connection every target can see.
func instanceScoped(c connhub.Connection) bool {
	return c.ScopeType == "" || c.ScopeType == store.ConnectionScopeInstance
}

// loadBindableConnection answers 404 both for a missing ref and one the
// caller may not read, so the endpoint is no existence oracle.
func (rt *Router) loadBindableConnection(w http.ResponseWriter, r *http.Request, ref string) (connhub.Connection, bool) {
	c, err := rt.connectionHub().Get(r.Context(), ref)
	if errors.Is(err, connhub.ErrNotFound) {
		writeError(w, http.StatusNotFound, msgConnectionNotFound)
		return c, false
	}
	if err != nil {
		rt.internalError(w, "api: binding: load connection failed", err)
		return c, false
	}
	ok, err := rt.connectionReadable(r, c)
	if err != nil {
		rt.internalError(w, "api: binding: connection access", err)
		return c, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, msgConnectionNotFound)
		return c, false
	}
	return c, true
}

func decodeBindingRequest(w http.ResponseWriter, r *http.Request, maxRules int) (bindingRequest, bool) {
	var req bindingRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(maxRules)+1024))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return req, false
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	req.Ref, req.Role = strings.TrimSpace(req.Ref), strings.TrimSpace(req.Role)
	if req.Ref == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "ref and role are required")
		return req, false
	}
	if !connhub.KnownRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be one of "+strings.Join(connhub.Roles(), ", "))
		return req, false
	}
	return req, true
}

// createBinding runs the checks shared by app and org targets: the caller can
// read the connection, its scope covers the target, it can fill the role, and
// the rules fit the role's schema.
func (rt *Router) createBinding(w http.ResponseWriter, r *http.Request, bs bindingStore, target connhub.Target) {
	lim := rt.bindingRuleLimits(r.Context())
	req, ok := decodeBindingRequest(w, r, lim.MaxBytes)
	if !ok {
		return
	}
	c, ok := rt.loadBindableConnection(w, r, req.Ref)
	if !ok {
		return
	}
	if !connhub.Covers(c, target) {
		writeError(w, http.StatusBadRequest, "this connection's scope does not include this "+target.Type)
		return
	}
	// An org default reaches every app in the org, so handing an instance
	// connection to an org other than the default one is a root decision.
	if target.Type == store.BindingTargetOrg && target.ID != store.DefaultTeamID && instanceScoped(c) && !rt.callerIsRoot(r) {
		writeError(w, http.StatusForbidden, "making an instance connection an organization default needs root")
		return
	}
	if !connhub.RoleSupported(c, req.Role) {
		writeError(w, http.StatusBadRequest, "this connection cannot be used for the "+req.Role+" role")
		return
	}
	rules, err := connhub.ValidateRules(req.Role, req.Rules, lim)
	if err != nil {
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), connhub.ErrInvalid.Error()+": "))
		return
	}
	_, actor, _, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: binding: resolve caller", err)
		return
	}
	b, err := bs.CreateConnectionBinding(r.Context(), store.ConnectionBinding{
		Ref: c.Ref, TargetType: target.Type, TargetID: target.ID, Role: req.Role, Rules: rules, CreatedBy: actor,
	})
	if errors.Is(err, store.ErrConnectionBindingExists) {
		writeError(w, http.StatusConflict, "this connection is already bound for that role")
		return
	}
	if err != nil {
		rt.internalError(w, "api: create binding failed", err)
		return
	}
	if !rt.bindingStillCovered(w, r, bs, b, target) {
		return
	}
	writeJSON(w, http.StatusCreated, bindingView(b))
}

// bindingStillCovered re-reads the connection after the insert: a scope change
// that landed in between removes the new row again, so no stored binding sits
// outside its connection's scope.
func (rt *Router) bindingStillCovered(w http.ResponseWriter, r *http.Request, bs bindingStore, b store.ConnectionBinding, target connhub.Target) bool {
	c, err := rt.connectionHub().Get(r.Context(), b.Ref)
	if err == nil && connhub.Covers(c, target) {
		return true
	}
	if err != nil && !errors.Is(err, connhub.ErrNotFound) {
		rt.logger.Warn("api: binding: re-check connection failed", slog.String("error", err.Error()))
	}
	if derr := bs.DeleteConnectionBinding(context.WithoutCancel(r.Context()), b.ID); derr != nil && !errors.Is(derr, store.ErrConnectionBindingNotFound) {
		rt.internalError(w, "api: binding: remove uncovered binding failed", derr)
		return false
	}
	writeError(w, http.StatusConflict, "this connection's scope changed while binding; try again")
	return false
}

// deleteBinding only finds ids among the target's own rows, so an id from
// another app or org answers 404.
func (rt *Router) deleteBinding(w http.ResponseWriter, r *http.Request, bs bindingStore, targetType, targetID, id string) {
	rows, err := bs.ListConnectionBindingsByTarget(r.Context(), targetType, targetID)
	if err != nil {
		rt.internalError(w, "api: delete binding: list failed", err)
		return
	}
	found := false
	for _, b := range rows {
		found = found || b.ID == id
	}
	if !found || id == "" {
		writeError(w, http.StatusNotFound, msgBindingNotFound)
		return
	}
	if err := bs.DeleteConnectionBinding(r.Context(), id); errors.Is(err, store.ErrConnectionBindingNotFound) {
		writeError(w, http.StatusNotFound, msgBindingNotFound)
		return
	} else if err != nil {
		rt.internalError(w, "api: delete binding failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) requireBindingApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: binding: load app failed", err)
		return "", false
	}
	return name, true
}

func (rt *Router) handleCreateAppBinding(w http.ResponseWriter, r *http.Request) {
	bs, ok := rt.bindingStore(w)
	if !ok {
		return
	}
	name, ok := rt.requireBindingApp(w, r)
	if !ok {
		return
	}
	team, err := rt.appTeam(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: create app binding: owner", err)
		return
	}
	rt.createBinding(w, r, bs, connhub.Target{Type: store.BindingTargetApp, ID: name, TeamID: team})
}

func (rt *Router) handleDeleteAppBinding(w http.ResponseWriter, r *http.Request) {
	bs, ok := rt.bindingStore(w)
	if !ok {
		return
	}
	name, ok := rt.requireBindingApp(w, r)
	if !ok {
		return
	}
	rt.deleteBinding(w, r, bs, store.BindingTargetApp, name, r.PathValue("id"))
}

func (rt *Router) requireOrg(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if rt.organizations == nil {
		writeError(w, http.StatusNotImplemented, "organizations are not available on this store")
		return "", false
	}
	if _, err := rt.organizations.GetOrganization(r.Context(), id); errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: binding: load organization failed", err)
		return "", false
	}
	return id, true
}

// requireOrgAdmin guards org default writes: they reach every app in the org.
func (rt *Router) requireOrgAdmin(w http.ResponseWriter, r *http.Request, id string) bool {
	ok, err := rt.orgAdministrable(r, id)
	if err != nil {
		rt.internalError(w, "api: binding: org access", err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "changing organization bindings needs an organization admin")
	}
	return ok
}

func (rt *Router) handleListOrgBindings(w http.ResponseWriter, r *http.Request) {
	bs, ok := rt.bindingStore(w)
	if !ok {
		return
	}
	id, ok := rt.requireOrg(w, r)
	if !ok {
		return
	}
	rows, err := bs.ListConnectionBindingsByTarget(r.Context(), store.BindingTargetOrg, id)
	if err != nil {
		rt.internalError(w, "api: list org bindings failed", err)
		return
	}
	out := make([]connhub.Binding, 0, len(rows))
	for _, b := range rows {
		out = append(out, bindingView(b))
	}
	writeJSON(w, http.StatusOK, orgBindingsResponse{Bindings: out})
}

func (rt *Router) handleCreateOrgBinding(w http.ResponseWriter, r *http.Request) {
	bs, ok := rt.bindingStore(w)
	if !ok {
		return
	}
	id, ok := rt.requireOrg(w, r)
	if !ok || !rt.requireOrgAdmin(w, r, id) {
		return
	}
	rt.createBinding(w, r, bs, connhub.Target{Type: store.BindingTargetOrg, ID: id, TeamID: id})
}

func (rt *Router) handleDeleteOrgBinding(w http.ResponseWriter, r *http.Request) {
	bs, ok := rt.bindingStore(w)
	if !ok {
		return
	}
	id, ok := rt.requireOrg(w, r)
	if !ok || !rt.requireOrgAdmin(w, r, id) {
		return
	}
	rt.deleteBinding(w, r, bs, store.BindingTargetOrg, id, r.PathValue("bid"))
}

// bindingsOutsideScope counts a connection's bindings the proposed scope would
// no longer cover; a scope change is refused while any remain.
func (rt *Router) bindingsOutsideScope(ctx context.Context, bs bindingStore, c connhub.Connection) (int, error) {
	rows, err := bs.ListConnectionBindingsByRef(ctx, c.Ref)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range rows {
		t := connhub.Target{Type: b.TargetType, ID: b.TargetID, TeamID: b.TargetID}
		if b.TargetType == store.BindingTargetApp {
			if t.TeamID, err = rt.appTeam(ctx, b.TargetID); err != nil {
				return 0, err
			}
		}
		if !connhub.Covers(c, t) {
			n++
		}
	}
	return n, nil
}
