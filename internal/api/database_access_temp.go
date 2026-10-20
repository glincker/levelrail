package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/store"
)

type databaseTempIssueRequest struct {
	Preset     string `json:"preset"`
	TTLMinutes int    `json:"ttl_minutes"`
}

type databaseTempResource struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Preset    string `json:"preset"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	State     string `json:"state"`
}

type databaseTempIssueResponse struct {
	Temp       databaseTempResource       `json:"temp"`
	Credential databaseCredentialResource `json:"credential"`
	// Clamped is true when the requested lifetime was outside the allowed
	// window and was moved into it.
	Clamped bool                  `json:"clamped"`
	Limits  databaseTTLLimitsBody `json:"limits"`
}

type databaseTTLLimitsBody struct {
	MinMinutes     int `json:"min_minutes"`
	MaxMinutes     int `json:"max_minutes"`
	DefaultMinutes int `json:"default_minutes"`
}

func (rt *Router) ttlLimitsBody() databaseTTLLimitsBody {
	l := rt.dbAccessTTL
	return databaseTTLLimitsBody{
		MinMinutes: int(l.Min / time.Minute), MaxMinutes: int(l.Max / time.Minute), DefaultMinutes: int(l.Default / time.Minute),
	}
}

func toTempResource(u store.DatabaseAccessUser) databaseTempResource {
	return databaseTempResource{
		ID: u.ID, Role: u.Role, Preset: u.Preset, CreatedBy: u.CreatedBy,
		CreatedAt: u.CreatedAt, ExpiresAt: u.ExpiresAt, State: u.State,
	}
}

// handleIssueDatabaseTempCredential handles POST /api/v1/databases/{name}/access/temp.
func (rt *Router) handleIssueDatabaseTempCredential(w http.ResponseWriter, r *http.Request) {
	var req databaseTempIssueRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	preset := dbaccess.Preset(req.Preset)
	if req.Preset == "" {
		preset = dbaccess.PresetReadOnly
	}
	if !dbaccess.TempPresetAllowed(preset) {
		writeError(w, http.StatusBadRequest, "temporary credentials are read_only or read_write")
		return
	}
	if req.TTLMinutes < 0 {
		writeError(w, http.StatusBadRequest, "ttl_minutes must not be negative")
		return
	}
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	ttl, clamped := rt.dbAccessTTL.Clamp(time.Duration(req.TTLMinutes) * time.Minute)
	roleName, err := dbaccess.NewTempRoleName()
	if err != nil {
		rt.internalError(w, "api: issue temp credential: name failed", err)
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(ttl)
	rec := store.DatabaseAccessUser{
		Database: desired.Name, Role: roleName, Kind: store.DatabaseAccessKindTemp, Preset: string(preset),
		CreatedBy: rt.accessActor(r).name, CreatedAt: now.Format(store.DatabaseAccessTimeLayout),
		ExpiresAt: expires.Format(store.DatabaseAccessTimeLayout),
	}
	if rec.ID, err = store.NewDatabaseAccessID(); err != nil {
		rt.internalError(w, "api: issue temp credential: id failed", err)
		return
	}
	// Record before creating the role: a crash between the two leaves a
	// row the sweeper drops idempotently, never a live role nobody tracks.
	rec.State = store.DatabaseAccessStateActive
	if err := rt.dbAccess.RecordDatabaseAccessUser(r.Context(), rec); err != nil {
		rt.internalError(w, "api: issue temp credential: record failed", err, slog.String("name", desired.Name))
		return
	}
	cred, err := pg.Create(r.Context(), dbaccess.CreateParamsIn{
		Role: roleName, Preset: preset, ConnLimit: dbaccess.TempConnLimit, ValidUntil: expires, Temporary: true,
	})
	if err != nil {
		_ = rt.dbAccess.MarkDatabaseAccessUserRevoked(r.Context(), desired.Name, roleName, time.Now())
		rt.writeDatabaseAccessError(w, "api: issue temp credential failed", desired.Name, err)
		return
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionTempIssue, desired.Name, roleName, http.StatusCreated)
	rt.logger.Info("api: temporary database credential issued",
		slog.String("database", desired.Name), slog.String("role", roleName), slog.String("preset", string(preset)), slog.Time("expires_at", expires))
	writeJSON(w, http.StatusCreated, databaseTempIssueResponse{
		Temp:       toTempResource(rec),
		Credential: rt.databaseCredential(r.Context(), r, desired, roleName, cred.Password, expires),
		Clamped:    clamped,
		Limits:     rt.ttlLimitsBody(),
	})
}

type databaseTempListResponse struct {
	Items  []databaseTempResource `json:"items"`
	Limits databaseTTLLimitsBody  `json:"limits"`
}

// handleListDatabaseTempCredentials handles GET /api/v1/databases/{name}/access/temp.
func (rt *Router) handleListDatabaseTempCredentials(w http.ResponseWriter, r *http.Request) {
	if rt.dbAccess == nil {
		writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
		return
	}
	desired, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	recs, err := rt.dbAccess.ListDatabaseAccessUsers(r.Context(), desired.Name, store.DatabaseAccessKindTemp)
	if err != nil {
		rt.internalError(w, "api: list temp credentials failed", err, slog.String("name", desired.Name))
		return
	}
	out := databaseTempListResponse{Items: make([]databaseTempResource, 0, len(recs)), Limits: rt.ttlLimitsBody()}
	for _, u := range recs {
		out.Items = append(out.Items, toTempResource(u))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeDatabaseTempCredential handles DELETE /api/v1/databases/{name}/access/temp/{id}.
func (rt *Router) handleRevokeDatabaseTempCredential(w http.ResponseWriter, r *http.Request) {
	pg, desired, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	rec, err := rt.dbAccess.GetDatabaseAccessUserByID(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.internalError(w, "api: revoke temp credential: lookup failed", err, slog.String("name", desired.Name))
		return
	}
	if rec == nil || rec.Database != desired.Name || rec.Kind != store.DatabaseAccessKindTemp {
		writeError(w, http.StatusNotFound, "temporary credential not found")
		return
	}
	if err := pg.DropTemp(r.Context(), rec.Role); err != nil {
		rt.writeDatabaseAccessError(w, "api: revoke temp credential failed", desired.Name, err)
		return
	}
	if err := rt.dbAccess.MarkDatabaseAccessUserRevoked(r.Context(), desired.Name, rec.Role, time.Now()); err != nil {
		rt.logger.Warn("api: revoke temp credential: record update failed", slog.String("database", desired.Name), slog.String("error", err.Error()))
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionTempRevoke, desired.Name, rec.Role, http.StatusNoContent)
	rt.logger.Info("api: temporary database credential revoked", slog.String("database", desired.Name), slog.String("role", rec.Role))
	w.WriteHeader(http.StatusNoContent)
}

// RevokeTemp implements dbaccess.Revoker for the expiry sweeper. A database
// that no longer exists has nothing left to revoke.
func (rt *Router) RevokeTemp(ctx context.Context, name, role string) error {
	desired, err := rt.databases.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load database %q: %w", name, err)
	}
	if rt.execRuntime == nil {
		return errors.New("exec is not configured")
	}
	nodeRuntime, err := rt.execRuntime(desired.NodeID)
	if err != nil {
		return fmt.Errorf("resolve runtime for database %q: %w", name, err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	state, err := nodeRuntime.InspectByName(inspectCtx, databaseContainerName(name))
	if err != nil {
		return fmt.Errorf("inspect database %q: %w", name, err)
	}
	if state == nil || !state.Running {
		return fmt.Errorf("database %q is not running, role %q stays unrevoked until it is (its VALID UNTIL still blocks logins)", name, role)
	}
	pg := dbaccess.Postgres{Exec: nodeRuntime, ContainerID: state.ID, Admin: desired.Name, Database: desired.Name}
	return pg.DropTemp(ctx, role)
}
