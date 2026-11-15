package api

import (
	"context"
	"encoding/json"
	"errors"
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
	Bindings []connhub.Binding `json:"bindings"`
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
		src.Notify = rt.notificationChannels
	}
	return connhub.New(src, func(ctx context.Context) time.Duration {
		if rt.runtimeSettings == nil {
			return 0
		}
		return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyConnectionExpiryWarn)
	})
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
	if _, err := rt.connectionHub().Get(r.Context(), ref); errors.Is(err, connhub.ErrNotFound) {
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
	if cur.ScopeType == store.ConnectionScopeApp {
		if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusBadRequest, "unknown app for scope_id"), appRef(cur.ScopeID)) {
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
	list, err := rt.connectionHub().AppBindings(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list app bindings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, appBindingsResponse{Bindings: list})
}
