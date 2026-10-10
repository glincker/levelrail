package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/extdb"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxExternalDatabaseBody = 16 << 10
	externalProbeDetached   = 3 * time.Minute
)

// ExternalDatabaseStore is the store surface for databases this platform
// connects to but does not run.
type ExternalDatabaseStore interface {
	CreateExternalDatabase(ctx context.Context, d store.ExternalDatabase) error
	UpdateExternalDatabase(ctx context.Context, d store.ExternalDatabase) error
	GetExternalDatabase(ctx context.Context, name string) (*store.ExternalDatabase, error)
	ListExternalDatabases(ctx context.Context) ([]store.ExternalDatabase, error)
	DeleteExternalDatabase(ctx context.Context, name string) error
	SetExternalDatabaseHealth(ctx context.Context, name, status, reason string, latencyMs int, checkedAt time.Time) error
}

// externalDatabaseHealth is the last probe outcome. Reason never holds a password.
type externalDatabaseHealth struct {
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	LatencyMs int    `json:"latency_ms"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// externalDatabaseResource is the wire shape. It has no password field of
// any kind: only HasPassword. The password is read back only through the
// explicit reveal route.
type externalDatabaseResource struct {
	Name            string                  `json:"name"`
	Engine          string                  `json:"engine"`
	Host            string                  `json:"host"`
	Port            int                     `json:"port"`
	Username        string                  `json:"username,omitempty"`
	Database        string                  `json:"database,omitempty"`
	TLSMode         string                  `json:"tls_mode"`
	Network         string                  `json:"network,omitempty"`
	NodeID          string                  `json:"node_id,omitempty"`
	ProjectID       string                  `json:"project_id,omitempty"`
	SourceContainer string                  `json:"source_container,omitempty"`
	HasPassword     bool                    `json:"has_password"`
	External        bool                    `json:"external"`
	Health          *externalDatabaseHealth `json:"health,omitempty"`
	CreatedAt       string                  `json:"created_at,omitempty"`
}

// externalDatabaseRequest is the create, update, test and adopt body.
type externalDatabaseRequest struct {
	Name            string `json:"name"`
	Engine          string `json:"engine"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Database        string `json:"database"`
	AuthDatabase    string `json:"auth_database"`
	TLSMode         string `json:"tls_mode"`
	Network         string `json:"network"`
	NodeID          string `json:"node_id"`
	ProjectID       string `json:"project_id"`
	SourceContainer string `json:"source_container"`
	Container       string `json:"container"`
}

func (rt *Router) toExternalDatabaseResource(ctx context.Context, d store.ExternalDatabase) externalDatabaseResource {
	res := externalDatabaseResource{
		Name: d.Name, Engine: d.Engine, Host: d.Host, Port: d.Port, Username: d.Username, Database: d.DatabaseName,
		TLSMode: d.TLSMode, Network: d.Network, NodeID: d.NodeID, ProjectID: d.ProjectID,
		SourceContainer: d.SourceContainer, External: true, CreatedAt: d.CreatedAt,
	}
	if rt.secrets != nil {
		if ok, err := rt.secrets.Exists(ctx, store.ExternalDatabaseSecretsKey(d.Name), store.ExternalDatabasePasswordKey); err == nil {
			res.HasPassword = ok
		}
	}
	if d.HealthStatus != "" {
		res.Health = &externalDatabaseHealth{Status: d.HealthStatus, Reason: d.HealthReason, LatencyMs: d.HealthLatencyMs, CheckedAt: d.HealthCheckedAt}
	}
	return res
}

func (req externalDatabaseRequest) conn() extdb.Conn {
	src := req.SourceContainer
	return extdb.Conn{
		Engine: req.Engine, Host: req.Host, Port: req.Port, User: req.Username, Password: req.Password,
		Database: req.Database, AuthDatabase: req.AuthDatabase, TLSMode: req.TLSMode, Network: req.Network,
		SourceContainer: src,
	}
}

// validateExternalConn applies the address policy, including the DNS check.
func (rt *Router) validateExternalConn(ctx context.Context, c *extdb.Conn) error {
	policy := extdb.PolicyFromEnv()
	if err := c.Validate(policy); err != nil {
		return err
	}
	return c.CheckResolved(ctx, policy, nil)
}

func (rt *Router) decodeExternalDatabaseRequest(w http.ResponseWriter, r *http.Request) (externalDatabaseRequest, bool) {
	var req externalDatabaseRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxExternalDatabaseBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	return req, true
}

func (rt *Router) loadExternalDatabase(w http.ResponseWriter, r *http.Request) (*store.ExternalDatabase, bool) {
	name := r.PathValue("name")
	d, err := rt.externalDatabases.GetExternalDatabase(r.Context(), name)
	if errors.Is(err, store.ErrExternalDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "external database not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: load external database failed", err, slog.String("name", name))
		return nil, false
	}
	return d, true
}

// externalProbe runs one probe on nodeID's Docker daemon.
func (rt *Router) externalProbe(ctx context.Context, name, nodeID string, c extdb.Conn) (extdb.Result, error) {
	if rt.execRuntime == nil {
		return extdb.Result{}, errors.New("probing is not available on this control plane")
	}
	runtime, err := rt.execRuntime(nodeID)
	if err != nil {
		return extdb.Result{}, fmt.Errorf("the node is not currently reachable: %w", err)
	}
	return (&extdb.Prober{Runtime: runtime, Logger: rt.logger}).Probe(ctx, name, c), nil
}

// handleListExternalDatabases handles GET /api/v1/external-databases.
func (rt *Router) handleListExternalDatabases(w http.ResponseWriter, r *http.Request) {
	recs, err := rt.externalDatabases.ListExternalDatabases(r.Context())
	if err != nil {
		rt.internalError(w, "api: list external databases failed", err)
		return
	}
	canSee, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list external databases: visibility", err)
		return
	}
	out := make([]externalDatabaseResource, 0, len(recs))
	for _, d := range recs {
		if canSee(d.Name) {
			out = append(out, rt.toExternalDatabaseResource(r.Context(), d))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetExternalDatabase handles GET /api/v1/external-databases/{name}.
func (rt *Router) handleGetExternalDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.loadExternalDatabase(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.toExternalDatabaseResource(r.Context(), *d))
}

// handleTestExternalDatabase handles POST /api/v1/external-databases/test. It
// probes with the supplied credentials and stores nothing.
func (rt *Router) handleTestExternalDatabase(w http.ResponseWriter, r *http.Request) {
	req, ok := rt.decodeExternalDatabaseRequest(w, r)
	if !ok {
		return
	}
	c := req.conn()
	if err := rt.validateExternalConn(r.Context(), &c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := rt.externalProbe(r.Context(), "test", req.NodeID, c)
	if err != nil {
		writeError(w, http.StatusBadGateway, c.Scrub(err.Error()))
		return
	}
	rt.logger.Info("api: external database test", slog.String("engine", c.Engine), slog.String("host", c.Host), slog.String("status", res.Status))
	writeJSON(w, http.StatusOK, res)
}

// handleCreateExternalDatabase handles POST /api/v1/external-databases.
func (rt *Router) handleCreateExternalDatabase(w http.ResponseWriter, r *http.Request) {
	req, ok := rt.decodeExternalDatabaseRequest(w, r)
	if !ok {
		return
	}
	rt.createExternalDatabase(w, r, req)
}

func (rt *Router) createExternalDatabase(w http.ResponseWriter, r *http.Request, req externalDatabaseRequest) {
	ctx := r.Context()
	if !extdb.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be lowercase letters, digits and hyphens, up to 63 characters")
		return
	}
	c := req.conn()
	if err := rt.validateExternalConn(ctx, &c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if c.Password != "" && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane, so a password cannot be stored")
		return
	}
	if _, err := rt.databases.GetDesiredDatabase(ctx, req.Name); err == nil {
		writeError(w, http.StatusConflict, "a managed database with this name already exists")
		return
	} else if !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.internalError(w, "api: create external database: check managed name failed", err, slog.String("name", req.Name))
		return
	}
	rec := store.ExternalDatabase{
		Name: req.Name, Engine: c.Engine, Host: c.Host, Port: c.Port, Username: c.User, DatabaseName: c.Database,
		TLSMode: c.TLSMode, Network: c.Network, NodeID: req.NodeID, ProjectID: req.ProjectID, SourceContainer: c.SourceContainer,
	}
	if err := rt.externalDatabases.CreateExternalDatabase(ctx, rec); errors.Is(err, store.ErrExternalDatabaseExists) {
		writeError(w, http.StatusConflict, "an external database with this name already exists")
		return
	} else if err != nil {
		rt.internalError(w, "api: create external database failed", err, slog.String("name", req.Name))
		return
	}
	if c.Password != "" {
		if err := rt.secrets.SetValueGuarded(ctx, store.ExternalDatabaseSecretsKey(req.Name), store.ExternalDatabasePasswordKey, c.Password, true); err != nil {
			_ = rt.externalDatabases.DeleteExternalDatabase(ctx, req.Name)
			rt.internalError(w, "api: create external database: store password failed", err, slog.String("name", req.Name))
			return
		}
	}
	rt.logger.Info("api: external database connected", slog.String("name", req.Name), slog.String("engine", c.Engine), slog.String("host", c.Host), slog.Int("port", c.Port))
	rt.probeExternalDetached(req.Name, req.NodeID, c)
	saved, err := rt.externalDatabases.GetExternalDatabase(ctx, req.Name)
	if err != nil {
		rt.internalError(w, "api: create external database: reload failed", err, slog.String("name", req.Name))
		return
	}
	writeJSON(w, http.StatusCreated, rt.toExternalDatabaseResource(ctx, *saved))
}

// probeExternalDetached records a first health reading without holding the
// request open: the helper image may need pulling.
func (rt *Router) probeExternalDetached(name, nodeID string, c extdb.Conn) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), externalProbeDetached)
		defer cancel()
		res, err := rt.externalProbe(ctx, name, nodeID, c)
		if err != nil {
			res = extdb.Result{Status: store.ExternalHealthUnknown, Reason: c.Scrub(err.Error())}
		}
		if serr := rt.externalDatabases.SetExternalDatabaseHealth(ctx, name, res.Status, res.Reason, res.LatencyMs, time.Now()); serr != nil {
			rt.logger.Warn("api: record external database health failed", slog.String("name", name), slog.String("error", serr.Error()))
		}
	}()
}

// handleUpdateExternalDatabase handles PUT /api/v1/external-databases/{name}.
// Name and engine are fixed; an empty password keeps the stored one.
func (rt *Router) handleUpdateExternalDatabase(w http.ResponseWriter, r *http.Request) {
	existing, ok := rt.loadExternalDatabase(w, r)
	if !ok {
		return
	}
	req, ok := rt.decodeExternalDatabaseRequest(w, r)
	if !ok {
		return
	}
	req.Name, req.Engine = existing.Name, existing.Engine
	if req.SourceContainer == "" {
		req.SourceContainer = existing.SourceContainer
	}
	c := req.conn()
	if err := rt.validateExternalConn(r.Context(), &c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if c.Password != "" && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane, so a password cannot be stored")
		return
	}
	rec := store.ExternalDatabase{
		Name: existing.Name, Host: c.Host, Port: c.Port, Username: c.User, DatabaseName: c.Database,
		TLSMode: c.TLSMode, Network: c.Network, NodeID: req.NodeID,
	}
	if err := rt.externalDatabases.UpdateExternalDatabase(r.Context(), rec); err != nil {
		rt.internalError(w, "api: update external database failed", err, slog.String("name", existing.Name))
		return
	}
	if c.Password != "" {
		if err := rt.secrets.SetValueGuarded(r.Context(), store.ExternalDatabaseSecretsKey(existing.Name), store.ExternalDatabasePasswordKey, c.Password, true); err != nil {
			rt.internalError(w, "api: update external database: store password failed", err, slog.String("name", existing.Name))
			return
		}
	}
	rt.logger.Info("api: external database updated", slog.String("name", existing.Name))
	saved, err := rt.externalDatabases.GetExternalDatabase(r.Context(), existing.Name)
	if err != nil {
		rt.internalError(w, "api: update external database: reload failed", err, slog.String("name", existing.Name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toExternalDatabaseResource(r.Context(), *saved))
}

// handleDeleteExternalDatabase handles DELETE /api/v1/external-databases/{name}.
// It removes this platform's record and stored password only. The remote
// database is never contacted, so its data and container are untouched.
func (rt *Router) handleDeleteExternalDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.loadExternalDatabase(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("force") != "true" {
		users, err := rt.appsUsingDatabase(r.Context(), d.Name)
		if err != nil {
			rt.internalError(w, "api: delete external database: list dependent apps failed", err, slog.String("name", d.Name))
			return
		}
		if len(users) > 0 {
			writeError(w, http.StatusConflict, "database is used by apps ("+strings.Join(users, ", ")+"); detach them or retry with force=true")
			return
		}
	}
	if err := rt.externalDatabases.DeleteExternalDatabase(r.Context(), d.Name); err != nil && !errors.Is(err, store.ErrExternalDatabaseNotFound) {
		rt.internalError(w, "api: delete external database failed", err, slog.String("name", d.Name))
		return
	}
	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(r.Context(), store.ExternalDatabaseSecretsKey(d.Name)); err != nil {
			rt.logger.Warn("api: delete external database: clear password failed", slog.String("name", d.Name), slog.String("error", err.Error()))
		}
	}
	rt.logger.Info("api: external database record deleted, remote database untouched", slog.String("name", d.Name))
	w.WriteHeader(http.StatusNoContent)
}

// handleProbeExternalDatabase handles POST /api/v1/external-databases/{name}/probe.
func (rt *Router) handleProbeExternalDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.loadExternalDatabase(w, r)
	if !ok {
		return
	}
	var pw extdb.PasswordSource
	if rt.secrets != nil {
		pw = rt.secrets
	}
	if runtime, rerr := rt.execRuntimeFor(d.NodeID); rerr == nil {
		if next, changed := extdb.Refresh(r.Context(), runtime, *d); changed {
			if uerr := rt.externalDatabases.UpdateExternalDatabase(r.Context(), next); uerr == nil {
				d = &next
			}
		}
	}
	c, err := extdb.ConnFromRecord(r.Context(), *d, pw)
	if err != nil {
		rt.internalError(w, "api: probe external database: load connection failed", err, slog.String("name", d.Name))
		return
	}
	res, err := rt.externalProbe(r.Context(), d.Name, d.NodeID, c)
	if err != nil {
		writeError(w, http.StatusBadGateway, c.Scrub(err.Error()))
		return
	}
	if serr := rt.externalDatabases.SetExternalDatabaseHealth(r.Context(), d.Name, res.Status, res.Reason, res.LatencyMs, time.Now()); serr != nil {
		rt.logger.Warn("api: record external database health failed", slog.String("name", d.Name), slog.String("error", serr.Error()))
	}
	writeJSON(w, http.StatusOK, res)
}

type externalDatabasePasswordResponse struct {
	Password string `json:"password"`
}

// handleRevealExternalDatabasePassword handles
// GET /api/v1/external-databases/{name}/password. Root only, never cached,
// and recorded by the audit middleware like every non-read route.
func (rt *Router) handleRevealExternalDatabasePassword(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.loadExternalDatabase(w, r)
	if !ok {
		return
	}
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane")
		return
	}
	key := store.ExternalDatabaseSecretsKey(d.Name)
	exists, err := rt.secrets.Exists(r.Context(), key, store.ExternalDatabasePasswordKey)
	if err != nil {
		rt.internalError(w, "api: reveal external database password: check failed", err, slog.String("name", d.Name))
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "no password is stored for this database")
		return
	}
	pw, err := rt.secrets.Resolve(r.Context(), key, store.ExternalDatabasePasswordKey)
	if err != nil {
		rt.internalError(w, "api: reveal external database password failed", err, slog.String("name", d.Name))
		return
	}
	rt.logger.Warn("api: external database password revealed", slog.String("name", d.Name))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, externalDatabasePasswordResponse{Password: pw})
}

// handleListExternalDatabaseCandidates handles
// GET /api/v1/external-databases/candidates?node_id=. It reads container
// metadata only.
func (rt *Router) handleListExternalDatabaseCandidates(w http.ResponseWriter, r *http.Request) {
	runtime, ok := rt.externalNodeRuntime(w, r.URL.Query().Get("node_id"))
	if !ok {
		return
	}
	cands, err := extdb.ListCandidates(r.Context(), runtime)
	if err != nil {
		rt.internalError(w, "api: list adoptable containers failed", err)
		return
	}
	if cands == nil {
		cands = []extdb.Candidate{}
	}
	writeJSON(w, http.StatusOK, cands)
}

func (rt *Router) externalNodeRuntime(w http.ResponseWriter, nodeID string) (docker.Runtime, bool) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "docker access is not configured on this control plane")
		return nil, false
	}
	runtime, err := rt.execRuntime(nodeID)
	if err != nil {
		rt.logger.Warn("api: external database: resolve node runtime failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "the node is not currently reachable")
		return nil, false
	}
	return runtime, true
}

// handleAdoptExternalDatabase handles POST /api/v1/external-databases/adopt.
// It records a connection to a container that is already running. The
// container is only listed, never stopped, restarted, recreated or changed.
func (rt *Router) handleAdoptExternalDatabase(w http.ResponseWriter, r *http.Request) {
	req, ok := rt.decodeExternalDatabaseRequest(w, r)
	if !ok {
		return
	}
	if req.Container == "" {
		writeError(w, http.StatusBadRequest, "container is required")
		return
	}
	runtime, ok := rt.externalNodeRuntime(w, req.NodeID)
	if !ok {
		return
	}
	cands, err := extdb.ListCandidates(r.Context(), runtime)
	if err != nil {
		rt.internalError(w, "api: adopt external database: list containers failed", err)
		return
	}
	var cand *extdb.Candidate
	for i := range cands {
		if cands[i].Container == req.Container || cands[i].ContainerID == req.Container {
			cand = &cands[i]
			break
		}
	}
	if cand == nil {
		writeError(w, http.StatusNotFound, "no running database container with that name was found on the node")
		return
	}
	req.Engine = cand.Engine
	req.SourceContainer = cand.Container
	if req.Host == "" {
		req.Host = cand.SuggestedHost
	}
	if req.Network == "" {
		req.Network = cand.Network
	}
	if req.Port == 0 {
		req.Port = cand.Port
	}
	if req.Username == "" {
		req.Username = cand.SuggestedUser
	}
	if req.Name == "" {
		req.Name = adoptName(cand.Container)
	}
	rt.logger.Info("api: adopting database container, container is not modified", slog.String("container", cand.Container), slog.String("name", req.Name), slog.String("node_id", req.NodeID))
	rt.createExternalDatabase(w, r, req)
}

func adoptName(container string) string {
	var b strings.Builder
	for _, ch := range strings.ToLower(container) {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9', ch == '-':
			b.WriteRune(ch)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}

func (rt *Router) execRuntimeFor(nodeID string) (docker.Runtime, error) {
	if rt.execRuntime == nil {
		return nil, errors.New("docker access is not configured")
	}
	return rt.execRuntime(nodeID)
}
