package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/connhub"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type connectionListResponse struct {
	Connections []connhub.Connection `json:"connections"`
}

type connectionCatalogResponse struct {
	Catalog []connhub.CatalogEntry `json:"catalog"`
}

type appBindingsResponse struct {
	Bindings []connhub.Binding         `json:"bindings"`
	Resolved []connhub.ResolvedBinding `json:"resolved"`
}

// connectionMetaRequest: an empty scope_type keeps the scope; a nil time keeps it and "" clears it.
type connectionMetaRequest struct {
	ScopeType   string  `json:"scope_type"`
	ScopeID     string  `json:"scope_id"`
	ExpiresAt   *string `json:"expires_at"`
	RotateAfter *string `json:"rotate_after"`
}

// connMetaWriter is the sidecar write surface; stores without it answer 501.
type connMetaWriter interface {
	GetConnectionMeta(ctx context.Context, ref string) (store.ConnectionMeta, error)
	UpsertConnectionMeta(ctx context.Context, m store.ConnectionMeta) error
}

// connectionHub assembles the read model from whatever the store implements; a missing table only empties its kind.
func (rt *Router) connectionHub() *connhub.Service {
	var src connhub.Sources
	st := any(rt.apps)
	src.Git, _ = st.(connhub.GitReader)
	src.Registry, _ = st.(connhub.RegistryReader)
	src.Secrets, _ = st.(connhub.SecretReader)
	src.DNS, _ = st.(connhub.DNSReader)
	src.Cloud, _ = st.(connhub.CloudReader)
	src.Webhooks, _ = st.(connhub.WebhookReader)
	src.Meta, _ = st.(connhub.MetaReader)
	if rt.notificationChannels != nil {
		src.Notify = notifyChannelReader{rt.notificationChannels}
	}
	return connhub.New(src, func(ctx context.Context) time.Duration {
		if rt.runtimeSettings == nil {
			return 0
		}
		return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyConnectionExpiryWarn)
	})
}

// notifyChannelReader maps alerting channels to the hub's narrow view, dropping secrets and URLs.
type notifyChannelReader struct{ src NotificationChannels }

func (n notifyChannelReader) ListNotificationChannels(ctx context.Context) ([]connhub.NotifyChannel, error) {
	chans, err := n.src.ListNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]connhub.NotifyChannel, 0, len(chans))
	for _, c := range chans {
		out = append(out, connhub.NotifyChannel{ID: c.ID, Name: c.Name, Kind: string(c.Kind), Enabled: c.Enabled, CreatedAt: c.CreatedAt})
	}
	return out, nil
}

func (rt *Router) handleListConnections(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := rt.connectionHub().List(r.Context(), connhub.Filter{Kind: q.Get("kind"), Scope: q.Get("scope"), Health: q.Get("health")})
	if err != nil {
		rt.internalError(w, "api: list connections failed", err)
		return
	}
	if list == nil {
		list = []connhub.Connection{}
	}
	writeJSON(w, http.StatusOK, connectionListResponse{Connections: list})
}

func (rt *Router) handleConnectionCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, connectionCatalogResponse{Catalog: connhub.Catalog()})
}

func (rt *Router) handleGetConnection(w http.ResponseWriter, r *http.Request) {
	c, err := rt.connectionHub().Get(r.Context(), r.PathValue("ref"))
	if errors.Is(err, connhub.ErrNotFound) {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get connection failed", err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func parseMetaTime(raw *string, cur time.Time) (time.Time, bool) {
	if raw == nil {
		return cur, true
	}
	if *raw == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, *raw)
	return t.UTC(), err == nil
}

func (rt *Router) handleSetConnectionMeta(w http.ResponseWriter, r *http.Request) {
	mw, ok := any(rt.apps).(connMetaWriter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "connection metadata is not available on this store")
		return
	}
	ref := r.PathValue("ref")
	conn, err := rt.connectionHub().Get(r.Context(), ref)
	if errors.Is(err, connhub.ErrNotFound) {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set connection meta: load connection failed", err)
		return
	}
	var req connectionMetaRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cur, err := mw.GetConnectionMeta(r.Context(), ref)
	if err != nil && !errors.Is(err, store.ErrConnectionMetaNotFound) {
		rt.internalError(w, "api: set connection meta: load meta failed", err)
		return
	}
	cur.Ref = ref
	oldType, oldID := cur.ScopeType, cur.ScopeID
	if oldType == "" {
		oldType = store.ConnectionScopeInstance
	}
	if req.ScopeType != "" {
		cur.ScopeType, cur.ScopeID = req.ScopeType, strings.TrimSpace(req.ScopeID)
	}
	switch cur.ScopeType {
	case "", store.ConnectionScopeInstance:
		cur.ScopeType, cur.ScopeID = store.ConnectionScopeInstance, ""
	case store.ConnectionScopeOrg, store.ConnectionScopeApp:
		if cur.ScopeID == "" {
			writeError(w, http.StatusBadRequest, "scope_id is required for an org or app scope")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "scope_type must be instance, org or app")
		return
	}
	// Every meta write, expiry and rotation included, needs authority over the current scope.
	if !rt.authorizeScopeOwner(w, r, oldType, oldID) {
		return
	}
	if cur.ScopeType != oldType || cur.ScopeID != oldID {
		if !rt.authorizeScopeTarget(w, r, cur.ScopeType, cur.ScopeID) {
			return
		}
		if !rt.refuseOrphanedBindings(w, r, conn, cur.ScopeType, cur.ScopeID) {
			return
		}
	}
	var okExp, okRot bool
	if cur.ExpiresAt, okExp = parseMetaTime(req.ExpiresAt, cur.ExpiresAt); !okExp {
		writeError(w, http.StatusBadRequest, "expires_at must be an RFC3339 time or empty")
		return
	}
	if cur.RotateAfter, okRot = parseMetaTime(req.RotateAfter, cur.RotateAfter); !okRot {
		writeError(w, http.StatusBadRequest, "rotate_after must be an RFC3339 time or empty")
		return
	}
	if err := mw.UpsertConnectionMeta(r.Context(), cur); err != nil {
		rt.internalError(w, "api: set connection meta failed", err)
		return
	}
	c, err := rt.connectionHub().Get(r.Context(), ref)
	if err != nil {
		rt.internalError(w, "api: set connection meta: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (rt *Router) handleListAppBindings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list app bindings: load app failed", err)
		return
	}
	hub := rt.connectionHub()
	list, err := hub.AppBindings(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list app bindings failed", err)
		return
	}
	target, byRef, err := rt.appBindingScope(r.Context(), hub, name)
	if err != nil {
		rt.internalError(w, "api: app bindings: load scope failed", err)
		return
	}
	resolved, err := rt.resolveAppBindings(r.Context(), list, target, byRef)
	if err != nil {
		rt.internalError(w, "api: resolve app bindings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, appBindingsResponse{Bindings: connhub.CoveredBindings(list, byRef, target), Resolved: resolved})
}

// appBindingScope returns the app as a binding target and every connection by
// ref, the inputs of the scope rule that decides which stored rows count.
func (rt *Router) appBindingScope(ctx context.Context, hub *connhub.Service, name string) (connhub.Target, map[string]connhub.Connection, error) {
	team, err := rt.appTeam(ctx, name)
	if err != nil {
		return connhub.Target{}, nil, err
	}
	conns, err := hub.List(ctx, connhub.Filter{})
	if err != nil {
		return connhub.Target{}, nil, err
	}
	byRef := make(map[string]connhub.Connection, len(conns))
	for _, c := range conns {
		byRef[c.Ref] = c
	}
	return connhub.Target{Type: store.BindingTargetApp, ID: name, TeamID: team}, byRef, nil
}

// resolveAppBindings adds the app's org defaults under the app's own rows and
// drops any stored row whose connection scope no longer covers the app.
func (rt *Router) resolveAppBindings(ctx context.Context, own []connhub.Binding, target connhub.Target, byRef map[string]connhub.Connection) ([]connhub.ResolvedBinding, error) {
	var org []connhub.Binding
	if bs, ok := any(rt.apps).(bindingStore); ok {
		rows, err := bs.ListConnectionBindingsByTarget(ctx, store.BindingTargetOrg, target.TeamID)
		if err != nil {
			return nil, err
		}
		for _, b := range rows {
			org = append(org, bindingView(b))
		}
	}
	return connhub.Resolve(own, org, byRef, target), nil
}

// authorizeScopeOwner needs authority over the current scope: write:sensitive
// on an app, admin on an org, an instance admin for an instance connection or
// one whose app or org was deleted.
func (rt *Router) authorizeScopeOwner(w http.ResponseWriter, r *http.Request, oldType, oldID string) bool {
	if oldType == store.ConnectionScopeInstance || oldID == "" {
		ok, err := rt.instanceAdministrable(r)
		if err != nil {
			rt.internalError(w, "api: set connection meta: instance access", err)
			return false
		}
		if !ok {
			writeError(w, http.StatusForbidden, "changing an instance connection needs an instance admin")
		}
		return ok
	}
	switch oldType {
	case store.ConnectionScopeApp:
		if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusForbidden, "changing this connection's scope needs write access to its current app"), appRef(oldID)) {
			return false
		}
	case store.ConnectionScopeOrg:
		ok, err := rt.orgAdministrable(r, oldID)
		if err != nil {
			rt.internalError(w, "api: set connection meta: org access", err)
			return false
		}
		if !ok {
			writeError(w, http.StatusForbidden, "changing this connection's scope needs admin on its current organization")
			return false
		}
	}
	return true
}

// authorizeScopeTarget needs authority over the new scope too, so a scope
// never moves onto a resource the caller does not manage.
func (rt *Router) authorizeScopeTarget(w http.ResponseWriter, r *http.Request, newType, newID string) bool {
	switch newType {
	case store.ConnectionScopeApp:
		if _, err := rt.apps.GetDesiredService(r.Context(), newID); errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusBadRequest, "unknown app for scope_id")
			return false
		} else if err != nil {
			rt.internalError(w, "api: set connection meta: load app", err)
			return false
		}
		return rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusBadRequest, "unknown app for scope_id"), appRef(newID))
	case store.ConnectionScopeOrg:
		if rt.organizations == nil {
			writeError(w, http.StatusBadRequest, "unknown organization for scope_id")
			return false
		}
		if _, err := rt.organizations.GetOrganization(r.Context(), newID); errors.Is(err, store.ErrOrganizationNotFound) {
			writeError(w, http.StatusBadRequest, "unknown organization for scope_id")
			return false
		} else if err != nil {
			rt.internalError(w, "api: set connection meta: load organization", err)
			return false
		}
		ok, err := rt.orgAdministrable(r, newID)
		if err != nil {
			rt.internalError(w, "api: set connection meta: org access", err)
			return false
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown organization for scope_id")
		}
		return ok
	}
	return true
}

// refuseOrphanedBindings keeps every existing binding inside the scope.
func (rt *Router) refuseOrphanedBindings(w http.ResponseWriter, r *http.Request, c connhub.Connection, scopeType, scopeID string) bool {
	bs, ok := any(rt.apps).(bindingStore)
	if !ok {
		return true
	}
	c.ScopeType, c.ScopeID = scopeType, scopeID
	n, err := rt.bindingsOutsideScope(r.Context(), bs, c)
	if err != nil {
		rt.internalError(w, "api: set connection meta: check bindings", err)
		return false
	}
	if n > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("%d bindings fall outside the new scope; unbind them first", n))
		return false
	}
	return true
}
