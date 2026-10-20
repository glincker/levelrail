package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	queryModeRead    = "read"
	queryModeWrite   = "write"
	queryModeExplain = "explain"
	queryBodyMargin  = 4096
)

type databaseQueryRequest struct {
	SQL     string `json:"sql"`
	Analyze bool   `json:"analyze,omitempty"`
	// Confirm must equal the database name on the write route.
	Confirm string `json:"confirm,omitempty"`
}

type databaseQueryResponse struct {
	dbviewer.Result
	Mode        string `json:"mode"`
	Kind        string `json:"kind"`
	Fingerprint string `json:"fingerprint"`
}

// DatabaseQueryStore is the store surface the console's history and saved
// query routes need.
type DatabaseQueryStore interface {
	AddDatabaseQueryHistory(ctx context.Context, h store.DatabaseQueryHistory, keep int) (store.DatabaseQueryHistory, error)
	ListDatabaseQueryHistory(ctx context.Context, database, principalType, principalID string, limit int) ([]store.DatabaseQueryHistory, error)
	ClearDatabaseQueryHistory(ctx context.Context, database, principalType, principalID string) error
	SaveDatabaseQuery(ctx context.Context, q store.DatabaseSavedQuery) (store.DatabaseSavedQuery, error)
	ListDatabaseSavedQueries(ctx context.Context, database, principalType, principalID string) ([]store.DatabaseSavedQuery, error)
	DeleteDatabaseSavedQuery(ctx context.Context, database, principalType, principalID, id string) error
}

func (rt *Router) decodeQueryRequest(w http.ResponseWriter, r *http.Request, limits dbviewer.Limits) (databaseQueryRequest, bool) {
	var req databaseQueryRequest
	r.Body = http.MaxBytesReader(w, r.Body, int64(limits.SQLBytes+queryBodyMargin))
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	if len(req.SQL) > limits.SQLBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "statement is too large")
		return req, false
	}
	return req, true
}

// handleDatabaseQuery handles POST /api/v1/databases/{name}/query: one
// read-only statement.
func (rt *Router) handleDatabaseQuery(w http.ResponseWriter, r *http.Request) {
	rt.runDatabaseStatement(w, r, queryModeRead)
}

// handleDatabaseQueryWrite handles POST /api/v1/databases/{name}/query/write.
// Registered at AbilityRoot; the caller must also echo the database name.
func (rt *Router) handleDatabaseQueryWrite(w http.ResponseWriter, r *http.Request) {
	rt.runDatabaseStatement(w, r, queryModeWrite)
}

// handleDatabaseExplain handles POST /api/v1/databases/{name}/explain.
func (rt *Router) handleDatabaseExplain(w http.ResponseWriter, r *http.Request) {
	rt.runDatabaseStatement(w, r, queryModeExplain)
}

func (rt *Router) runDatabaseStatement(w http.ResponseWriter, r *http.Request, mode string) {
	target, desired, ok := rt.viewerTarget(w, r, false)
	if !ok {
		return
	}
	req, ok := rt.decodeQueryRequest(w, r, target.Limits)
	if !ok {
		return
	}
	write := mode == queryModeWrite
	if write && req.Confirm != desired.Name {
		writeError(w, http.StatusBadRequest, "confirm must equal the database name to run a write statement")
		return
	}

	st, err := dbviewer.Check(target.Dialect, req.SQL, write)
	if err != nil {
		rt.recordQuery(r, desired.Name, mode, req.SQL, "", false, err.Error(), 0, 0)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	start := time.Now()
	var res dbviewer.Result
	if mode == queryModeExplain {
		res, err = target.Explain(r.Context(), st, req.Analyze)
	} else {
		res, err = target.Run(r.Context(), st, write)
	}
	elapsed := time.Since(start).Milliseconds()

	attrs := []any{
		slog.String("database", desired.Name), slog.String("mode", mode),
		slog.String("fingerprint", st.Fingerprint), slog.Int64("duration_ms", elapsed),
	}
	if err != nil {
		rt.logger.Info("api: database console statement failed", attrs...)
		rt.recordQuery(r, desired.Name, mode, req.SQL, st.Fingerprint, false, queryErrorText(err), elapsed, 0)
		if write {
			rt.recordWriteAudit(r, desired.Name, st.Fingerprint, queryErrorStatus(err))
		}
		rt.writeViewerError(w, "api: database console statement failed", desired.Name, err)
		return
	}
	rt.logger.Info("api: database console statement", append(attrs, slog.Int("rows", res.RowCount))...)
	rt.recordQuery(r, desired.Name, mode, req.SQL, st.Fingerprint, true, "", elapsed, res.RowCount)
	if write {
		rt.recordWriteAudit(r, desired.Name, st.Fingerprint, http.StatusOK)
	}
	writeJSON(w, http.StatusOK, databaseQueryResponse{Result: res, Mode: mode, Kind: st.Kind, Fingerprint: st.Fingerprint})
}

func queryErrorText(err error) string {
	var qe *dbviewer.QueryError
	if errors.As(err, &qe) {
		return qe.Message
	}
	if errors.Is(err, dbviewer.ErrTimeout) {
		return err.Error()
	}
	return "internal error"
}

func queryErrorStatus(err error) int {
	var qe *dbviewer.QueryError
	switch {
	case errors.As(err, &qe):
		return http.StatusBadRequest
	case errors.Is(err, dbviewer.ErrTimeout):
		return http.StatusRequestTimeout
	}
	return http.StatusInternalServerError
}

// recordQuery stores one history row for the caller. Best-effort, like
// recordAudit: a failure is logged and never fails the request.
func (rt *Router) recordQuery(r *http.Request, database, mode, sql, fingerprint string, ok bool, errMsg string, durationMs int64, rows int) {
	if rt.dbQueries == nil {
		return
	}
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return
	}
	_, err = rt.dbQueries.AddDatabaseQueryHistory(r.Context(), store.DatabaseQueryHistory{
		DatabaseName: database, PrincipalType: pt, PrincipalID: pid, Mode: mode, SQL: sql,
		Fingerprint: fingerprint, OK: ok, Error: errMsg, DurationMs: durationMs, RowCount: rows,
	}, dbviewer.HistoryKeepFromEnv(nil))
	if err != nil {
		rt.logger.Warn("api: record database query history failed", slog.String("error", err.Error()), slog.String("database", database))
	}
}

// recordWriteAudit writes an audit row naming the statement fingerprint,
// since the route-level audit row only carries the path.
func (rt *Router) recordWriteAudit(r *http.Request, database, fingerprint string, status int) {
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: database write audit id failed", slog.String("error", err.Error()))
		return
	}
	actorType := "session"
	if pt == store.PrincipalTypeToken {
		actorType = "token"
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: pid,
		ActorName: rt.auditActorName(r.Context(), actorType, pid, pid),
		Ability:   AbilityRoot, Method: http.MethodPost,
		Path:       "/api/v1/databases/" + database + "/query/write#" + fingerprint,
		StatusCode: status, RemoteAddr: clientIP(r),
		CreatedAt:  store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: database write audit save failed", slog.String("error", err.Error()), slog.String("database", database))
	}
}
