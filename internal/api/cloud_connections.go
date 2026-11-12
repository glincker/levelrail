package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/store"
)

const cloudConnectionsPath = "/api/v1/settings/cloud-connections"

// Audit actions for cloud connection changes. Rows carry names and classes, never credential material.
const (
	auditCloudConnCreated = "cloud_connection.created"
	auditCloudConnUpdated = "cloud_connection.updated"
	auditCloudConnDeleted = "cloud_connection.deleted"
	auditCloudConnTested  = "cloud_connection.tested"
)

// cloudConnStoreOf returns the store's cloud connection table, or nil when it lacks one.
func cloudConnStoreOf(s Store) cloudconn.Rows {
	r, _ := s.(cloudconn.Rows)
	return r
}

// SetCloudProviders replaces the built-in providers, for tests and for boot code that adds real clients.
func (rt *Router) SetCloudProviders(r cloudconn.Registry) { rt.cloudProviders = r }

// Reads need AbilityRead; every write and the live test need AbilityRoot.
func (rt *Router) registerCloudConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+cloudConnectionsPath, rt.requireAbility(AbilityRead, rt.handleListCloudConnections))
	mux.HandleFunc("POST "+cloudConnectionsPath, rt.requireAbility(AbilityRoot, rt.handleCreateCloudConnection))
	mux.HandleFunc("PUT "+cloudConnectionsPath+"/{name}", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudConnection))
	mux.HandleFunc("DELETE "+cloudConnectionsPath+"/{name}", rt.requireAbility(AbilityRoot, rt.handleDeleteCloudConnection))
	mux.HandleFunc("POST "+cloudConnectionsPath+"/{name}/test", rt.requireAbility(AbilityRoot, rt.handleTestCloudConnection))
}

type cloudConnectionConfig struct {
	Region         string `json:"region,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	Compartment    string `json:"compartment,omitempty"`
	RoleARN        string `json:"role_arn,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
	ResourceGroup  string `json:"resource_group,omitempty"`
	OIDCAudience   string `json:"oidc_audience,omitempty"`
}

// cloudConnectionResource never carries credential material; credential_ref is a reference, not a value.
type cloudConnectionResource struct {
	Name           string                `json:"name"`
	Provider       string                `json:"provider"`
	TeamID         string                `json:"team_id,omitempty"`
	AuthMethod     string                `json:"auth_method"`
	Config         cloudConnectionConfig `json:"config"`
	CredentialRef  string                `json:"credential_ref,omitempty"`
	Preview        bool                  `json:"preview"`
	CreatedBy      string                `json:"created_by,omitempty"`
	CreatedAt      *time.Time            `json:"created_at,omitempty"`
	UpdatedAt      *time.Time            `json:"updated_at,omitempty"`
	LastCheckedAt  *time.Time            `json:"last_checked_at,omitempty"`
	LastCheckClass string                `json:"last_check_class,omitempty"`
}

type cloudConnectionListResponse struct {
	Connections []cloudConnectionResource `json:"connections"`
}

type cloudConnectionRequest struct {
	Name          string                `json:"name,omitempty"`
	Provider      string                `json:"provider,omitempty"`
	TeamID        string                `json:"team_id,omitempty"`
	AuthMethod    string                `json:"auth_method,omitempty"`
	CredentialRef string                `json:"credential_ref,omitempty"`
	Config        cloudConnectionConfig `json:"config"`
}

type cloudConnectionTestResponse struct {
	OK     bool   `json:"ok"`
	Class  string `json:"class"`
	Reason string `json:"reason,omitempty"`
}

func toCloudConnectionResource(c cloudconn.Connection) cloudConnectionResource {
	res := cloudConnectionResource{
		Name: c.Name, Provider: c.Provider, TeamID: c.TeamID, AuthMethod: c.AuthMethod, Config: cloudConnectionConfig(c.Config),
		CredentialRef: c.CredentialRef, Preview: true, CreatedBy: c.CreatedBy, LastCheckClass: c.LastCheckClass,
	}
	for dst, src := range map[**time.Time]time.Time{&res.CreatedAt: c.CreatedAt, &res.UpdatedAt: c.UpdatedAt, &res.LastCheckedAt: c.LastCheckedAt} {
		if !src.IsZero() {
			t := src
			*dst = &t
		}
	}
	return res
}

// cloudConnections returns the service, or writes the refusal: 403 while the preview setting is off, 501 without a store table.
func (rt *Router) cloudConnections(w http.ResponseWriter, r *http.Request) (*cloudconn.Service, bool) {
	if rt.runtimeSettings == nil || !rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyCloudConnectionsEnabled) {
		writeError(w, http.StatusForbidden, "cloud connections are a preview feature and are off; enable the runtime setting cloud.connections_enabled")
		return nil, false
	}
	if rt.cloudConnRows == nil {
		writeError(w, http.StatusNotImplemented, "cloud connections are not available on this store")
		return nil, false
	}
	reg := rt.cloudProviders
	if reg == nil {
		reg = cloudconn.DefaultRegistry()
	}
	svc := &cloudconn.Service{Rows: rt.cloudConnRows, Providers: reg}
	// Sources are the secret connection resolvers; absent, secret references classify as unresolved.
	if sc, ok := rt.secretConns.(*secretconn.Service); ok && sc != nil {
		svc.Sources = sc.Sources
	}
	return svc, true
}

func (rt *Router) writeCloudConnectionError(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, cloudconn.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, cloudconn.ErrConflict):
		writeError(w, http.StatusConflict, "a cloud connection with this name already exists")
	case errors.Is(err, cloudconn.ErrNotFound):
		writeError(w, http.StatusNotFound, "cloud connection not found")
	default:
		rt.internalError(w, msg, err)
	}
}

func (q cloudConnectionRequest) input() cloudconn.Input {
	return cloudconn.Input{Provider: q.Provider, TeamID: q.TeamID, AuthMethod: q.AuthMethod, Config: cloudconn.Config(q.Config), CredentialRef: q.CredentialRef}
}

func (rt *Router) handleListCloudConnections(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cloudConnections(w, r)
	if !ok {
		return
	}
	list, err := svc.List(r.Context())
	if err != nil {
		rt.internalError(w, "api: list cloud connections failed", err)
		return
	}
	out := cloudConnectionListResponse{Connections: make([]cloudConnectionResource, 0, len(list))}
	for _, c := range list {
		out.Connections = append(out.Connections, toCloudConnectionResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateCloudConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cloudConnections(w, r)
	if !ok {
		return
	}
	var req cloudConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, actor, _, _ := rt.callerPrincipal(r)
	c, err := svc.Create(r.Context(), actor, req.Name, req.input())
	if err != nil {
		rt.writeCloudConnectionError(w, "api: create cloud connection failed", err)
		return
	}
	rt.auditCloudConnection(r, auditCloudConnCreated, c, c.AuthMethod)
	writeJSON(w, http.StatusCreated, toCloudConnectionResource(c))
}

func (rt *Router) handleUpdateCloudConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cloudConnections(w, r)
	if !ok {
		return
	}
	var req cloudConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c, err := svc.Update(r.Context(), r.PathValue("name"), req.input())
	if err != nil {
		rt.writeCloudConnectionError(w, "api: update cloud connection failed", err)
		return
	}
	rt.auditCloudConnection(r, auditCloudConnUpdated, c, c.AuthMethod)
	writeJSON(w, http.StatusOK, toCloudConnectionResource(c))
}

func (rt *Router) handleDeleteCloudConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cloudConnections(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	prev, err := svc.Get(r.Context(), name)
	if err == nil {
		err = svc.Delete(r.Context(), name)
	}
	if err != nil {
		rt.writeCloudConnectionError(w, "api: delete cloud connection failed", err)
		return
	}
	rt.auditCloudConnection(r, auditCloudConnDeleted, prev, "")
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleTestCloudConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cloudConnections(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	res, err := svc.Test(r.Context(), name)
	if err != nil {
		rt.writeCloudConnectionError(w, "api: test cloud connection failed", err)
		return
	}
	c, _ := svc.Get(r.Context(), name)
	rt.auditCloudConnection(r, auditCloudConnTested, c, res.Class)
	writeJSON(w, http.StatusOK, cloudConnectionTestResponse{OK: res.OK, Class: res.Class, Reason: res.Reason})
}

func (rt *Router) auditCloudConnection(r *http.Request, action string, c cloudconn.Connection, note string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: cloud connection audit id failed", slog.String("error", err.Error()))
		return
	}
	ptype, pid, _, perr := rt.callerPrincipal(r)
	if perr != nil {
		ptype, pid = auditActorSystem, "unknown"
	}
	detail, _ := json.Marshal(map[string]string{"connection": c.Name, "provider": c.Provider, "team": c.TeamID, "note": note})
	entry := store.AuditEntry{
		ID: id, ActorType: ptype, ActorID: pid, ActorName: pid, Ability: AbilityRoot, Method: r.Method,
		Path: cloudConnectionsPath + "/" + c.Name, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: action, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: save cloud connection audit failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
