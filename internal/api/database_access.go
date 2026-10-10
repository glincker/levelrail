package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DatabaseAccessStore is the persistence the database access, network and
// temporary credential handlers need. *store.DB satisfies it.
type DatabaseAccessStore interface {
	RecordDatabaseAccessUser(ctx context.Context, u store.DatabaseAccessUser) error
	ListDatabaseAccessUsers(ctx context.Context, database, kind string) ([]store.DatabaseAccessUser, error)
	GetDatabaseAccessUser(ctx context.Context, database, role string) (*store.DatabaseAccessUser, error)
	GetDatabaseAccessUserByID(ctx context.Context, id string) (*store.DatabaseAccessUser, error)
	MarkDatabaseAccessUserRevoked(ctx context.Context, database, role string, now time.Time) error
	GetDatabaseAccessSettings(ctx context.Context, database string) (store.DatabaseAccessSettings, error)
	SaveDatabaseAccessSettings(ctx context.Context, s store.DatabaseAccessSettings) error
	ListDatabaseNetworkRuleNotes(ctx context.Context, database string) (map[string]string, error)
	ReplaceDatabaseNetworkRuleNotes(ctx context.Context, database string, notes map[string]string) error
	EnvironmentOfDatabase(ctx context.Context, databaseName string) (*store.EnvironmentRef, error)
	EnvironmentOfApp(ctx context.Context, appName string) (*store.EnvironmentRef, error)
}

// WithDatabaseAccess enables database users, temporary credentials and the
// network controls. ttl bounds temporary credential lifetimes.
func WithDatabaseAccess(s DatabaseAccessStore, ttl dbaccess.TTLLimits) Option {
	return func(rt *Router) {
		rt.dbAccess = s
		rt.dbAccessTTL = ttl
	}
}

const (
	maxUserConnLimit      = 10000
	defaultUserConnLimit  = 20
	userExpiryMaxDays     = 3650
	errDatabaseAccessOff  = "database access controls are not configured on this control plane"
	errUsersPostgresOnly  = "database users are managed for PostgreSQL databases only"
	errBadBody            = "invalid request body"
	pgSSLModeRequire      = "require"
	pgSSLModePrefer       = "prefer"
	databaseKindRoleUser  = "user"
	databaseKindRoleTemp  = "temp"
	databaseKindRoleOwner = "platform"
	databaseKindRoleOther = "system"
)

// databaseUserResource is one role as listed. It never carries a secret.
type databaseUserResource struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Preset          string `json:"preset,omitempty"`
	CanLogin        bool   `json:"can_login"`
	Superuser       bool   `json:"superuser"`
	CreateDB        bool   `json:"create_db"`
	CreateRole      bool   `json:"create_role"`
	ConnectionLimit int    `json:"connection_limit"`
	ValidUntil      string `json:"valid_until,omitempty"`
	Expired         bool   `json:"expired"`
	Connections     int    `json:"connections"`
	Protected       bool   `json:"protected"`
	Managed         bool   `json:"managed"`
	CreatedBy       string `json:"created_by,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// databaseCredentialResource is returned exactly once, on create, rotate and
// temporary issue. Nothing here is stored.
type databaseCredentialResource struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Database     string `json:"database"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	SSLMode      string `json:"sslmode"`
	InternalURL  string `json:"internal_url"`
	ExternalURL  string `json:"external_url,omitempty"`
	ExternalNote string `json:"external_note,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

type databaseUserCreateRequest struct {
	Name            string `json:"name"`
	Preset          string `json:"preset"`
	ConnectionLimit int    `json:"connection_limit"`
	ExpiresAt       string `json:"expires_at"`
}

type databaseUserCreateResponse struct {
	User       databaseUserResource       `json:"user"`
	Credential databaseCredentialResource `json:"credential"`
}

// dbAccessTarget resolves a Postgres database's running container into a
// role manager, writing the error response itself on failure.
func (rt *Router) dbAccessTarget(w http.ResponseWriter, r *http.Request) (dbaccess.Postgres, *store.DesiredDatabase, bool) {
	var none dbaccess.Postgres
	if rt.dbAccess == nil {
		writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
		return none, nil, false
	}
	desired, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return none, nil, false
	}
	if desired.Engine != store.EnginePostgres {
		writeError(w, http.StatusBadRequest, errUsersPostgresOnly)
		return none, desired, false
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return none, desired, false
	}
	nodeRuntime, state, ok := rt.resolveDatabaseExecContainer(w, r, desired.Name, desired.NodeID)
	if !ok {
		return none, desired, false
	}
	return dbaccess.Postgres{Exec: nodeRuntime, ContainerID: state.ID, Admin: desired.Name, Database: desired.Name}, desired, true
}

func (rt *Router) dbAccessDatabase(w http.ResponseWriter, r *http.Request) (*store.DesiredDatabase, bool) {
	name := r.PathValue("name")
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: database access: load database failed", err, slog.String("name", name))
		return nil, false
	}
	return desired, true
}

// writeDatabaseAccessError maps a dbaccess failure to a response.
func (rt *Router) writeDatabaseAccessError(w http.ResponseWriter, context, name string, err error) {
	var se *dbaccess.StatementError
	switch {
	case errors.Is(err, dbaccess.ErrRoleName), errors.Is(err, dbaccess.ErrPreset), errors.Is(err, dbaccess.ErrTTL):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, dbaccess.ErrRoleExists):
		writeError(w, http.StatusConflict, "a role with that name already exists")
	case errors.Is(err, dbaccess.ErrRoleNotFound):
		writeError(w, http.StatusNotFound, "role not found")
	case errors.Is(err, dbaccess.ErrProtected):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &se):
		writeError(w, http.StatusBadRequest, se.Message)
	default:
		rt.internalError(w, context, err, slog.String("name", name))
	}
}

// databaseCredential assembles the connection details shown once. The
// external URL is only offered for a published port, and says which host it
// assumes.
func (rt *Router) databaseCredential(ctx context.Context, r *http.Request, d *store.DesiredDatabase, role, password string, expires time.Time) databaseCredentialResource {
	tls := rt.databaseTLSEnabled(ctx, *d)
	mode := pgSSLModePrefer
	if tls {
		mode = pgSSLModeRequire
	}
	port, _ := database.ContainerPort(d.Engine)
	host := database.ContainerName(d.Name)
	cred := databaseCredentialResource{
		Username: role, Password: password, Database: d.Name, Host: host, Port: port, SSLMode: mode,
		InternalURL: postgresURL(role, password, host, port, d.Name, mode),
	}
	if !expires.IsZero() {
		cred.ExpiresAt = expires.UTC().Format(time.RFC3339)
	}
	if d.PubliclyAccessible && d.PublicPort != 0 {
		pubHost := requestHostname(r)
		cred.ExternalURL = postgresURL(role, password, pubHost, d.PublicPort, d.Name, mode)
		cred.ExternalNote = "Uses this control plane's address. The port is bound to " + bindLabel(d.PublicBindAddress) + "."
	}
	return cred
}

func bindLabel(bind string) string {
	switch bind {
	case "", "private":
		return "loopback only, so reach it through an SSH tunnel"
	case "public":
		return "every interface"
	default:
		return bind
	}
}

func postgresURL(user, password, host string, port int, db, sslmode string) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + db,
		RawQuery: url.Values{"sslmode": {sslmode}}.Encode(),
	}
	return u.String()
}

func requestHostname(r *http.Request) string {
	h := r.Host
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return strings.Trim(h, "[]")
}

// toDatabaseUserResource merges an engine role with its platform record.
func toDatabaseUserResource(role dbaccess.Role, admin string, rec *store.DatabaseAccessUser, now time.Time) databaseUserResource {
	res := databaseUserResource{
		Name: role.Name, CanLogin: role.CanLogin, Superuser: role.Superuser, CreateDB: role.CreateDB, CreateRole: role.CreateRole,
		ConnectionLimit: role.ConnLimit, ValidUntil: role.ValidUntil, Connections: role.Connections,
		Protected: dbaccess.Protected(role, admin),
	}
	switch {
	case role.Name == admin:
		res.Kind = databaseKindRoleOwner
	case rec != nil && rec.Kind == store.DatabaseAccessKindTemp, strings.HasPrefix(role.Name, dbaccess.TempRolePrefix):
		res.Kind = databaseKindRoleTemp
	case rec != nil:
		res.Kind = databaseKindRoleUser
	default:
		res.Kind = databaseKindRoleOther
	}
	if rec != nil {
		res.Managed, res.Preset, res.CreatedBy, res.CreatedAt = true, rec.Preset, rec.CreatedBy, rec.CreatedAt
	}
	if role.ValidUntil != "" {
		if t, err := time.Parse(time.RFC3339, role.ValidUntil); err == nil && !t.After(now) {
			res.Expired = true
		}
	}
	return res
}

// handleListDatabaseUsers handles GET /api/v1/databases/{name}/users.
func (rt *Router) handleListDatabaseUsers(w http.ResponseWriter, r *http.Request) {
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	roles, err := pg.ListRoles(r.Context())
	if err != nil {
		rt.writeDatabaseAccessError(w, "api: list database users failed", desired.Name, err)
		return
	}
	recs, err := rt.dbAccess.ListDatabaseAccessUsers(r.Context(), desired.Name, "")
	if err != nil {
		rt.internalError(w, "api: list database users: load records failed", err, slog.String("name", desired.Name))
		return
	}
	byRole := make(map[string]*store.DatabaseAccessUser, len(recs))
	for i := range recs {
		byRole[recs[i].Role] = &recs[i]
	}
	now := time.Now()
	out := make([]databaseUserResource, 0, len(roles))
	for _, role := range roles {
		out = append(out, toDatabaseUserResource(role, desired.Name, byRole[role.Name], now))
	}
	writeJSON(w, http.StatusOK, out)
}

func parseUserExpiry(raw string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New("expires_at must be an RFC3339 timestamp")
	}
	if !t.After(now) {
		return time.Time{}, errors.New("expires_at must be in the future")
	}
	if t.After(now.Add(userExpiryMaxDays * 24 * time.Hour)) {
		return time.Time{}, errors.New("expires_at is too far in the future")
	}
	return t.UTC().Truncate(time.Second), nil
}

// handleCreateDatabaseUser handles POST /api/v1/databases/{name}/users.
func (rt *Router) handleCreateDatabaseUser(w http.ResponseWriter, r *http.Request) {
	var req databaseUserCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	preset := dbaccess.Preset(req.Preset)
	if req.Preset == "" {
		preset = dbaccess.PresetReadOnly
	}
	if !dbaccess.ValidPreset(preset) {
		writeError(w, http.StatusBadRequest, "preset must be read_only, read_write or owner")
		return
	}
	if req.ConnectionLimit < 0 || req.ConnectionLimit > maxUserConnLimit {
		writeError(w, http.StatusBadRequest, "connection_limit must be between 0 and "+strconv.Itoa(maxUserConnLimit))
		return
	}
	limit := req.ConnectionLimit
	if limit == 0 {
		limit = defaultUserConnLimit
	}
	now := time.Now()
	expires, err := parseUserExpiry(req.ExpiresAt, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := dbaccess.ValidateRoleName(req.Name, r.PathValue("name")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	cred, err := pg.Create(r.Context(), dbaccess.CreateParamsIn{Role: req.Name, Preset: preset, ConnLimit: limit, ValidUntil: expires})
	if err != nil {
		rt.writeDatabaseAccessError(w, "api: create database user failed", desired.Name, err)
		return
	}
	actor := rt.accessActor(r)
	rec := store.DatabaseAccessUser{
		Database: desired.Name, Role: req.Name, Kind: store.DatabaseAccessKindUser, Preset: string(preset),
		CreatedBy: actor.name, CreatedAt: now.UTC().Format(store.DatabaseAccessTimeLayout),
	}
	if !expires.IsZero() {
		rec.ExpiresAt = expires.Format(store.DatabaseAccessTimeLayout)
	}
	if rec.ID, err = store.NewDatabaseAccessID(); err == nil {
		err = rt.dbAccess.RecordDatabaseAccessUser(r.Context(), rec)
	}
	if err != nil {
		_ = pg.Drop(r.Context(), req.Name)
		rt.internalError(w, "api: create database user: record failed, role rolled back", err, slog.String("name", desired.Name), slog.String("role", req.Name))
		return
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionUserCreate, desired.Name, req.Name, http.StatusCreated)
	rt.logger.Info("api: database user created", slog.String("database", desired.Name), slog.String("role", req.Name), slog.String("preset", string(preset)))
	role := dbaccess.Role{Name: req.Name, CanLogin: true, ConnLimit: limit}
	if !expires.IsZero() {
		role.ValidUntil = expires.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusCreated, databaseUserCreateResponse{
		User:       toDatabaseUserResource(role, desired.Name, &rec, now),
		Credential: rt.databaseCredential(r.Context(), r, desired, req.Name, cred.Password, expires),
	})
}

// handleRotateDatabaseUser handles POST /api/v1/databases/{name}/users/{role}/rotate.
func (rt *Router) handleRotateDatabaseUser(w http.ResponseWriter, r *http.Request) {
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	role := r.PathValue("role")
	cred, err := pg.Rotate(r.Context(), role)
	if err != nil {
		rt.writeDatabaseAccessError(w, "api: rotate database user failed", desired.Name, err)
		return
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionUserRotate, desired.Name, role, http.StatusOK)
	rt.logger.Info("api: database user password rotated", slog.String("database", desired.Name), slog.String("role", role))
	writeJSON(w, http.StatusOK, rt.databaseCredential(r.Context(), r, desired, role, cred.Password, time.Time{}))
}

func (rt *Router) setDatabaseUserLogin(w http.ResponseWriter, r *http.Request, login bool) {
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	role := r.PathValue("role")
	if err := pg.SetLogin(r.Context(), role, login); err != nil {
		rt.writeDatabaseAccessError(w, "api: set database user login failed", desired.Name, err)
		return
	}
	action := dbaccess.ActionUserDisable
	if login {
		action = dbaccess.ActionUserEnable
	}
	rt.auditDatabaseAccess(r, action, desired.Name, role, http.StatusNoContent)
	rt.logger.Info("api: database user login changed", slog.String("database", desired.Name), slog.String("role", role), slog.Bool("login", login))
	w.WriteHeader(http.StatusNoContent)
}

// handleDisableDatabaseUser handles POST .../users/{role}/disable.
func (rt *Router) handleDisableDatabaseUser(w http.ResponseWriter, r *http.Request) {
	rt.setDatabaseUserLogin(w, r, false)
}

// handleEnableDatabaseUser handles POST .../users/{role}/enable.
func (rt *Router) handleEnableDatabaseUser(w http.ResponseWriter, r *http.Request) {
	rt.setDatabaseUserLogin(w, r, true)
}

// handleDeleteDatabaseUser handles DELETE /api/v1/databases/{name}/users/{role}.
func (rt *Router) handleDeleteDatabaseUser(w http.ResponseWriter, r *http.Request) {
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	role := r.PathValue("role")
	if err := pg.Drop(r.Context(), role); err != nil {
		rt.writeDatabaseAccessError(w, "api: delete database user failed", desired.Name, err)
		return
	}
	if err := rt.dbAccess.MarkDatabaseAccessUserRevoked(r.Context(), desired.Name, role, time.Now()); err != nil {
		rt.logger.Warn("api: delete database user: record update failed", slog.String("database", desired.Name), slog.String("role", role), slog.String("error", err.Error()))
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionUserDelete, desired.Name, role, http.StatusNoContent)
	rt.logger.Info("api: database user deleted", slog.String("database", desired.Name), slog.String("role", role))
	w.WriteHeader(http.StatusNoContent)
}
