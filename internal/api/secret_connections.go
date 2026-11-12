package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SecretConnectionService is the connection surface the handlers need; *secretconn.Service satisfies it.
type SecretConnectionService interface {
	List(ctx context.Context) ([]secretconn.Connection, error)
	Get(ctx context.Context, name string) (secretconn.Connection, error)
	Create(ctx context.Context, actor, name string, in secretconn.Input) (secretconn.Connection, error)
	Update(ctx context.Context, name string, in secretconn.Input) (secretconn.Connection, error)
	Delete(ctx context.Context, name string) error
	Test(ctx context.Context, name, path string) (secretconn.TestResult, error)
}

// SetSecretConnections wires the connection service after the router is built, as the boot registry needs the router's settings.
func (rt *Router) SetSecretConnections(s SecretConnectionService) { rt.secretConns = s }

// Audit actions for connection changes. Rows carry names and field names, never credential values.
const (
	auditSecretConnCreated = "secret_connection.created"
	auditSecretConnUpdated = "secret_connection.updated"
	auditSecretConnDeleted = "secret_connection.deleted"
	auditSecretConnTested  = "secret_connection.tested"
)

const secretConnectionsPath = "/api/v1/settings/secret-connections" //nolint:gosec // route and column names, not credentials

// Reads need AbilityRead; every write and the live test need AbilityRoot.
func (rt *Router) registerSecretConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+secretConnectionsPath, rt.requireAbility(AbilityRead, rt.handleListSecretConnections))
	mux.HandleFunc("POST "+secretConnectionsPath, rt.requireAbility(AbilityRoot, rt.handleCreateSecretConnection))
	mux.HandleFunc("PUT "+secretConnectionsPath+"/{name}", rt.requireAbility(AbilityRoot, rt.handleUpdateSecretConnection))
	mux.HandleFunc("DELETE "+secretConnectionsPath+"/{name}", rt.requireAbility(AbilityRoot, rt.handleDeleteSecretConnection))
	mux.HandleFunc("POST "+secretConnectionsPath+"/{name}/test", rt.requireAbility(AbilityRoot, rt.handleTestSecretConnection))
}

type secretConnectionConfig struct {
	Address     string `json:"address,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Mount       string `json:"mount,omitempty"`
	AuthMethod  string `json:"auth_method,omitempty"`
	RoleID      string `json:"role_id,omitempty"`
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Path        string `json:"path,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
}

// secretConnectionResource never carries a credential; credential_source says only whether one is stored or referenced.
type secretConnectionResource struct {
	Name             string                 `json:"name"`
	Type             string                 `json:"type"`
	Legacy           bool                   `json:"legacy"`
	ManagedBy        string                 `json:"managed_by,omitempty"`
	Config           secretConnectionConfig `json:"config"`
	HasCredential    bool                   `json:"has_credential"`
	CredentialSource string                 `json:"credential_source,omitempty"`
	// AllowedPrefixes and AllowedEnvs let a writer without root point apps at this connection, inside those limits.
	AllowedPrefixes []string   `json:"allowed_prefixes,omitempty"`
	AllowedEnvs     []string   `json:"allowed_envs,omitempty"`
	CreatedBy       string     `json:"created_by,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type secretConnectionListResponse struct {
	Connections []secretConnectionResource `json:"connections"`
}

type secretConnectionRequest struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	// Credential is write-only; on update an empty value keeps the stored one.
	Credential    string                 `json:"credential,omitempty"`
	CredentialRef string                 `json:"credential_ref,omitempty"`
	Config        secretConnectionConfig `json:"config"`
	// A present list (even empty) replaces that allow list; an absent one keeps it.
	AllowedPrefixes *[]string `json:"allowed_prefixes,omitempty"`
	AllowedEnvs     *[]string `json:"allowed_envs,omitempty"`
}

// secretConnectionTestRequest is the optional body of the test route; an empty
// Path proves login only.
type secretConnectionTestRequest struct {
	Path string `json:"path,omitempty"`
}

type secretConnectionTestResponse struct {
	OK     bool   `json:"ok"`
	Class  string `json:"class"`
	Reason string `json:"reason,omitempty"`
}

func toSecretConnectionResource(c secretconn.Connection) secretConnectionResource {
	res := secretConnectionResource{
		Name: c.Name, Type: c.Type, Legacy: c.Legacy, HasCredential: c.HasCredential, CreatedBy: c.CreatedBy,
		Config: secretConnectionConfig(c.Config), AllowedPrefixes: c.Policy.AllowedPrefixes, AllowedEnvs: c.Policy.AllowedEnvs,
	}
	switch {
	case c.CredentialRef != "":
		res.CredentialSource = "reference"
	case c.HasCredential:
		res.CredentialSource = "stored"
	}
	if c.Legacy {
		res.ManagedBy = "/api/v1/settings/secret-providers"
		if c.Type == secretconn.TypeVault {
			res.ManagedBy = "/api/v1/settings/vault"
		}
	}
	if !c.CreatedAt.IsZero() {
		t := c.CreatedAt
		res.CreatedAt = &t
	}
	if !c.UpdatedAt.IsZero() {
		t := c.UpdatedAt
		res.UpdatedAt = &t
	}
	return res
}

func (rt *Router) secretConnections(w http.ResponseWriter) (SecretConnectionService, bool) {
	if rt.secretConns == nil {
		writeError(w, http.StatusNotImplemented, "secret connections need a master key on this control plane")
		return nil, false
	}
	return rt.secretConns, true
}

func (rt *Router) writeSecretConnectionError(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, secretconn.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, secretconn.ErrConflict):
		writeError(w, http.StatusConflict, "a connection with this name already exists")
	case errors.Is(err, secretconn.ErrNotFound):
		writeError(w, http.StatusNotFound, "secret connection not found")
	case errors.Is(err, secretconn.ErrLegacy):
		writeError(w, http.StatusConflict, "this connection is managed through the Vault or secret provider settings")
	default:
		rt.internalError(w, msg, err)
	}
}

func (rt *Router) handleListSecretConnections(w http.ResponseWriter, r *http.Request) {
	svc := rt.secretConns
	if svc == nil {
		writeJSON(w, http.StatusOK, secretConnectionListResponse{Connections: []secretConnectionResource{}})
		return
	}
	list, err := svc.List(r.Context())
	if err != nil {
		rt.internalError(w, "api: list secret connections failed", err)
		return
	}
	out := secretConnectionListResponse{Connections: make([]secretConnectionResource, 0, len(list))}
	for _, c := range list {
		out.Connections = append(out.Connections, toSecretConnectionResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateSecretConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.secretConnections(w)
	if !ok {
		return
	}
	var req secretConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, actor, _, _ := rt.callerPrincipal(r)
	c, err := svc.Create(r.Context(), actor, req.Name, req.input())
	if err != nil {
		rt.writeSecretConnectionError(w, "api: create secret connection failed", err)
		return
	}
	rt.auditSecretConnection(r, auditSecretConnCreated, c.Name, c.Type, req.credentialNote(), c.Policy)
	writeJSON(w, http.StatusCreated, toSecretConnectionResource(c))
}

func (rt *Router) handleUpdateSecretConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.secretConnections(w)
	if !ok {
		return
	}
	var req secretConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := r.PathValue("name")
	c, err := svc.Update(r.Context(), name, req.input())
	if err != nil {
		rt.writeSecretConnectionError(w, "api: update secret connection failed", err)
		return
	}
	rt.auditSecretConnection(r, auditSecretConnUpdated, c.Name, c.Type, req.credentialNote(), c.Policy)
	writeJSON(w, http.StatusOK, toSecretConnectionResource(c))
}

func (rt *Router) handleDeleteSecretConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.secretConnections(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	prev, _ := svc.Get(r.Context(), name)
	if err := svc.Delete(r.Context(), name); err != nil {
		rt.writeSecretConnectionError(w, "api: delete secret connection failed", err)
		return
	}
	rt.auditSecretConnection(r, auditSecretConnDeleted, name, prev.Type, "", prev.Policy)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleTestSecretConnection(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.secretConnections(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var req secretConnectionTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := svc.Test(r.Context(), name, strings.TrimSpace(req.Path))
	if err != nil {
		rt.writeSecretConnectionError(w, "api: test secret connection failed", err)
		return
	}
	c, _ := svc.Get(r.Context(), name)
	rt.auditSecretConnection(r, auditSecretConnTested, name, c.Type, res.Class, secretconn.Policy{})
	writeJSON(w, http.StatusOK, secretConnectionTestResponse{OK: res.OK, Class: res.Class, Reason: res.Reason})
}

func (q secretConnectionRequest) input() secretconn.Input {
	return secretconn.Input{Type: q.Type, Config: secretconn.Config(q.Config), Credential: q.Credential, CredentialRef: q.CredentialRef,
		AllowedPrefixes: q.AllowedPrefixes, AllowedEnvs: q.AllowedEnvs}
}

// credentialNote says what happened to the credential without carrying it.
func (q secretConnectionRequest) credentialNote() string {
	switch {
	case q.Credential != "":
		return "credential_set"
	case q.CredentialRef != "":
		return "credential_ref_set"
	}
	return "credential_unchanged"
}

func (rt *Router) auditSecretConnection(r *http.Request, action, name, typ, note string, pol secretconn.Policy) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: secret connection audit id failed", slog.String("error", err.Error()))
		return
	}
	ptype, pid, _, perr := rt.callerPrincipal(r)
	if perr != nil {
		ptype, pid = auditActorSystem, "unknown"
	}
	fields := map[string]string{"connection": name, "type": typ, "note": note}
	if !pol.IsZero() {
		fields["allowed_prefixes"] = strings.Join(pol.AllowedPrefixes, ",")
		fields["allowed_envs"] = strings.Join(pol.AllowedEnvs, ",")
	}
	detail, _ := json.Marshal(fields)
	entry := store.AuditEntry{
		ID: id, ActorType: ptype, ActorID: pid, ActorName: pid, Ability: AbilityRoot, Method: r.Method,
		Path: secretConnectionsPath + "/" + name, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: action, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: save secret connection audit failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

// syncSecretConnections re-registers sources after a legacy Vault or provider change, so a new or removed name takes effect at once.
func (rt *Router) syncSecretConnections(ctx context.Context) {
	s, ok := rt.secretConns.(interface{ Sync(context.Context) error })
	if !ok {
		return
	}
	if err := s.Sync(ctx); err != nil {
		rt.logger.Warn("api: sync secret connections failed", slog.String("error", err.Error()))
	}
}
