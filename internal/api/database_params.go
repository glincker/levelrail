package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbparams"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit actions for the engine parameters editor. Values are logged with
// names because every allowlisted parameter is a non-secret tuning number.
const (
	AuditActionDatabaseParamsStage   = "database.params.stage"
	AuditActionDatabaseParamsApply   = "database.params.apply"
	AuditActionDatabaseParamsDiscard = "database.params.discard"
)

type databaseParamsStore interface {
	GetDatabaseEngineParams(ctx context.Context, name string) (store.DatabaseEngineParams, error)
	StageDatabaseEngineParams(ctx context.Context, name string, params map[string]string, by string, expected *string) error
	DiscardDatabaseEngineParams(ctx context.Context, name string, expected *string) error
	ApplyDatabaseEngineParams(ctx context.Context, name, expected string) (bool, error)
}

type databaseParamView struct {
	dbparams.Spec
	Current string  `json:"current"`
	Pending *string `json:"pending,omitempty"`
}

type databaseParamsResponse struct {
	Database        string              `json:"database"`
	Engine          string              `json:"engine"`
	Supported       bool                `json:"supported"`
	Params          []databaseParamView `json:"params"`
	HasPending      bool                `json:"has_pending"`
	RestartRequired bool                `json:"restart_required"`
	StagedBy        string              `json:"staged_by,omitempty"`
	StagedAt        string              `json:"staged_at,omitempty"`
	Previous        map[string]string   `json:"previous"`
	MaxConnections  int                 `json:"max_connections_ceiling"`
	// RolledBack is set when the reconciler restored the previous set after
	// the last applied one failed to start; the next apply clears it.
	RolledBack *databaseParamsRollback `json:"rolled_back,omitempty"`
}

type databaseParamsRollback struct {
	At     string `json:"at"`
	Reason string `json:"reason"`
}

type stageDatabaseParamsRequest struct {
	Set   map[string]string `json:"set"`
	Unset []string          `json:"unset"`
	// Rollback stages the set that was applied before the current one.
	Rollback bool `json:"rollback"`
}

type applyDatabaseParamsRequest struct {
	Confirm bool `json:"confirm"`
}

func (rt *Router) paramsStore(w http.ResponseWriter) (databaseParamsStore, bool) {
	s, ok := rt.databases.(databaseParamsStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "database parameters are not available on this server")
	}
	return s, ok
}

// paramsLimits caps memory at 75 percent of the database's own limit, or the admin setting without one.
func (rt *Router) paramsLimits(ctx context.Context, d *store.DesiredDatabase) dbparams.Limits {
	var memLimit int64
	if d.Resources != nil {
		memLimit = d.Resources.MemoryBytes
	}
	return dbparams.Limits{
		MaxConnections: rt.runtimeSettings.DatabaseParamsMaxConnections(ctx),
		MaxMemoryBytes: dbparams.MemoryCeiling(memLimit, rt.runtimeSettings.DatabaseParamsMaxMemoryMB(ctx)),
	}
}

// writeParamsConflict maps a lost compare and set to 409.
func (rt *Router) writeParamsConflict(w http.ResponseWriter, err error, msg, name string) {
	if errors.Is(err, store.ErrEngineParamsConflict) {
		writeError(w, http.StatusConflict, "the staged parameters changed meanwhile: reload and try again")
		return
	}
	rt.internalError(w, msg, err, slog.String("name", name))
}

// loadParamsDatabase returns the database and its parameter sets, 404 when missing.
func (rt *Router) loadParamsDatabase(w http.ResponseWriter, r *http.Request, s databaseParamsStore) (*store.DesiredDatabase, store.DatabaseEngineParams, bool) {
	name := r.PathValue("name")
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, store.DatabaseEngineParams{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load database for params failed", err, slog.String("name", name))
		return nil, store.DatabaseEngineParams{}, false
	}
	p, err := s.GetDatabaseEngineParams(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: load database params failed", err, slog.String("name", name))
		return nil, store.DatabaseEngineParams{}, false
	}
	return d, p, true
}

func (rt *Router) databaseParamsView(ctx context.Context, d *store.DesiredDatabase, p store.DatabaseEngineParams) databaseParamsResponse {
	specs := dbparams.Specs(d.Engine)
	lim := rt.paramsLimits(ctx, d)
	resp := databaseParamsResponse{
		Database: d.Name, Engine: d.Engine, Supported: specs != nil,
		Params: make([]databaseParamView, 0, len(specs)), HasPending: p.Pending != nil,
		RestartRequired: p.Pending != nil, StagedBy: p.StagedBy, StagedAt: p.StagedAt,
		Previous: p.Previous, MaxConnections: lim.MaxConnections,
	}
	if p.RolledBackAt != "" {
		resp.RolledBack = &databaseParamsRollback{At: p.RolledBackAt, Reason: p.RolledBackReason}
	}
	for _, s := range specs {
		// A shown default above the admin ceiling would not validate if submitted.
		if def, err := strconv.Atoi(s.Default); s.ConnectionCap && err == nil && lim.MaxConnections > 0 && def > lim.MaxConnections {
			s.Default = strconv.Itoa(lim.MaxConnections)
		}
		v := databaseParamView{Spec: s, Current: p.Applied[s.Name]}
		if p.Pending != nil {
			next := p.Pending[s.Name]
			if next != v.Current {
				v.Pending = &next
			}
		}
		resp.Params = append(resp.Params, v)
	}
	return resp
}

// handleGetDatabaseParams handles GET /api/v1/databases/{name}/params:
// default, current and staged value of every allowlisted parameter.
func (rt *Router) handleGetDatabaseParams(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.paramsStore(w)
	if !ok {
		return
	}
	d, p, ok := rt.loadParamsDatabase(w, r, s)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.databaseParamsView(r.Context(), d, p))
}

// handleStageDatabaseParams handles PUT /api/v1/databases/{name}/params. It
// only stages: the running container is untouched until apply or the next start.
func (rt *Router) handleStageDatabaseParams(w http.ResponseWriter, r *http.Request) {
	var req stageDatabaseParamsRequest
	if !decodeReq(w, r, &req) {
		return
	}
	s, ok := rt.paramsStore(w)
	if !ok {
		return
	}
	d, p, ok := rt.loadParamsDatabase(w, r, s)
	if !ok {
		return
	}
	if _, supported := dbparams.FamilyFor(d.Engine); !supported {
		writeError(w, http.StatusBadRequest, dbparams.ErrUnsupportedEngine.Error())
		return
	}
	next, err := nextParamSet(d.Engine, p, req, rt.paramsLimits(r.Context(), d))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a := rt.accessActor(r)
	if err := s.StageDatabaseEngineParams(r.Context(), d.Name, next, a.name, p.PendingRaw); err != nil {
		rt.writeParamsConflict(w, err, "api: stage database params failed", d.Name)
		return
	}
	rt.auditDatabaseParams(r, AuditActionDatabaseParamsStage, AbilityWrite, http.MethodPut, d.Name, next)
	rt.writeDatabaseParams(w, r, s, d)
}

// nextParamSet merges the request over the staged set, or the applied one
// when nothing is staged, and validates the whole result.
func nextParamSet(engine string, p store.DatabaseEngineParams, req stageDatabaseParamsRequest, lim dbparams.Limits) (map[string]string, error) {
	base := p.Applied
	if req.Rollback {
		base = p.Previous
	} else if p.Pending != nil {
		base = p.Pending
	}
	next := make(map[string]string, len(base)+len(req.Set))
	for k, v := range base {
		next[k] = v
	}
	for _, k := range req.Unset {
		if !knownParam(engine, k) {
			_, err := dbparams.ValidateOne(engine, k, "x", lim)
			return nil, err
		}
		delete(next, k)
	}
	for k, v := range req.Set {
		next[k] = v
	}
	return dbparams.Validate(engine, next, lim)
}

func knownParam(engine, name string) bool {
	for _, s := range dbparams.Specs(engine) {
		if s.Name == name {
			return true
		}
	}
	return false
}

// handleDiscardDatabaseParams handles DELETE /api/v1/databases/{name}/params/pending.
func (rt *Router) handleDiscardDatabaseParams(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.paramsStore(w)
	if !ok {
		return
	}
	d, p, ok := rt.loadParamsDatabase(w, r, s)
	if !ok {
		return
	}
	if p.PendingRaw == nil {
		rt.writeDatabaseParams(w, r, s, d)
		return
	}
	if err := s.DiscardDatabaseEngineParams(r.Context(), d.Name, p.PendingRaw); err != nil {
		rt.writeParamsConflict(w, err, "api: discard database params failed", d.Name)
		return
	}
	rt.auditDatabaseParams(r, AuditActionDatabaseParamsDiscard, AbilityWrite, http.MethodDelete, d.Name, nil)
	rt.writeDatabaseParams(w, r, s, d)
}

// handleApplyDatabaseParams handles POST /api/v1/databases/{name}/params/apply:
// it promotes the staged set, and the reconciler then replaces the container
// through its normal sequential replace and recreate guard.
func (rt *Router) handleApplyDatabaseParams(w http.ResponseWriter, r *http.Request) {
	var req applyDatabaseParamsRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "applying restarts the database: send confirm true to proceed")
		return
	}
	s, ok := rt.paramsStore(w)
	if !ok {
		return
	}
	d, p, ok := rt.loadParamsDatabase(w, r, s)
	if !ok {
		return
	}
	if p.PendingRaw == nil {
		writeError(w, http.StatusConflict, "no staged parameter change to apply")
		return
	}
	if bad := rt.stagedParamsIncompatible(r.Context(), d, p.Pending); len(bad) > 0 {
		writeError(w, http.StatusConflict, "staged parameters "+strings.Join(bad, ", ")+" are not valid on version "+d.Version+": stage the set again")
		return
	}
	applied, err := s.ApplyDatabaseEngineParams(r.Context(), d.Name, *p.PendingRaw)
	if err != nil {
		rt.internalError(w, "api: apply database params failed", err, slog.String("name", d.Name))
		return
	}
	if !applied {
		writeError(w, http.StatusConflict, "the staged parameters changed meanwhile: reload and try again")
		return
	}
	rt.auditDatabaseParams(r, AuditActionDatabaseParamsApply, AbilityWriteSensitive, http.MethodPost, d.Name, p.Pending)
	rt.writeDatabaseParams(w, r, s, d)
}

// applyStagedDatabaseParams promotes a staged set once a start of a stopped
// database succeeded; a failure leaves it staged and is only logged.
func (rt *Router) applyStagedDatabaseParams(r *http.Request, name string) {
	s, ok := rt.databases.(databaseParamsStore)
	if !ok {
		return
	}
	p, err := s.GetDatabaseEngineParams(r.Context(), name)
	if err != nil || p.PendingRaw == nil {
		return
	}
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		return
	}
	if bad := rt.stagedParamsIncompatible(r.Context(), d, p.Pending); len(bad) > 0 {
		rt.logger.Warn("api: staged database params not applied on start: invalid on this version", slog.String("name", name), slog.String("params", strings.Join(bad, ",")))
		return
	}
	applied, err := s.ApplyDatabaseEngineParams(r.Context(), name, *p.PendingRaw)
	if err != nil {
		rt.logger.Warn("api: apply staged database params on start failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	if applied {
		rt.auditDatabaseParams(r, AuditActionDatabaseParamsApply, AbilityWriteSensitive, http.MethodPost, name, p.Pending)
	}
}

func (rt *Router) writeDatabaseParams(w http.ResponseWriter, r *http.Request, s databaseParamsStore, d *store.DesiredDatabase) {
	p, err := s.GetDatabaseEngineParams(r.Context(), d.Name)
	if err != nil {
		rt.internalError(w, "api: reload database params failed", err, slog.String("name", d.Name))
		return
	}
	writeJSON(w, http.StatusOK, rt.databaseParamsView(r.Context(), d, p))
}

func (rt *Router) auditDatabaseParams(r *http.Request, action, ability, method, name string, params map[string]string) {
	if rt.auditLog == nil {
		return
	}
	entry, ok := rt.paramsAuditEntry(r, action, ability, method, name, params)
	if !ok {
		return
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: database params audit save failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}

// paramsAuditEntry builds the audit row, for callers that write it in the
// same transaction as the change.
func (rt *Router) paramsAuditEntry(r *http.Request, action, ability, method, name string, params map[string]string) (store.AuditEntry, bool) {
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: database params audit id failed", slog.String("error", err.Error()))
		return store.AuditEntry{}, false
	}
	names := make([]string, 0, len(params))
	for k := range params {
		names = append(names, k)
	}
	sort.Strings(names)
	detail, _ := json.Marshal(map[string]any{"database": name, "params": params, "names": names})
	return store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: ability, Method: method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     action, Detail: string(detail),
	}, true
}
