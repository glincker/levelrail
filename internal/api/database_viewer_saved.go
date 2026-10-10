package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultQueryHistoryList = 50
	maxSavedQueryNameLen    = 120
)

type queryHistoryResource struct {
	ID          string    `json:"id"`
	Mode        string    `json:"mode"`
	SQL         string    `json:"sql"`
	Fingerprint string    `json:"fingerprint"`
	OK          bool      `json:"ok"`
	Error       string    `json:"error,omitempty"`
	DurationMs  int64     `json:"duration_ms"`
	RowCount    int       `json:"row_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type savedQueryResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SQL       string    `json:"sql"`
	CreatedAt time.Time `json:"created_at"`
}

type savedQueryRequest struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

func (rt *Router) queryOwner(w http.ResponseWriter, r *http.Request) (pt, pid string, ok bool) {
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return "", "", false
	}
	return pt, pid, true
}

// handleListDatabaseQueryHistory handles GET /api/v1/databases/{name}/query-history.
func (rt *Router) handleListDatabaseQueryHistory(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := rt.queryOwner(w, r)
	if !ok {
		return
	}
	limit := defaultQueryHistoryList
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, dbviewer.HistoryKeepFromEnv(nil))
	}
	name := r.PathValue("name")
	rows, err := rt.dbQueries.ListDatabaseQueryHistory(r.Context(), name, pt, pid, limit)
	if err != nil {
		rt.internalError(w, "api: list database query history failed", err, slog.String("name", name))
		return
	}
	out := make([]queryHistoryResource, len(rows))
	for i, h := range rows {
		out[i] = queryHistoryResource{ID: h.ID, Mode: h.Mode, SQL: h.SQL, Fingerprint: h.Fingerprint, OK: h.OK, Error: h.Error, DurationMs: h.DurationMs, RowCount: h.RowCount, CreatedAt: h.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleClearDatabaseQueryHistory handles DELETE /api/v1/databases/{name}/query-history.
func (rt *Router) handleClearDatabaseQueryHistory(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := rt.queryOwner(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if err := rt.dbQueries.ClearDatabaseQueryHistory(r.Context(), name, pt, pid); err != nil {
		rt.internalError(w, "api: clear database query history failed", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListDatabaseSavedQueries handles GET /api/v1/databases/{name}/saved-queries.
func (rt *Router) handleListDatabaseSavedQueries(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := rt.queryOwner(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	rows, err := rt.dbQueries.ListDatabaseSavedQueries(r.Context(), name, pt, pid)
	if err != nil {
		rt.internalError(w, "api: list database saved queries failed", err, slog.String("name", name))
		return
	}
	out := make([]savedQueryResource, len(rows))
	for i, q := range rows {
		out[i] = savedQueryResource{ID: q.ID, Name: q.Name, SQL: q.SQL, CreatedAt: q.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSaveDatabaseQuery handles POST /api/v1/databases/{name}/saved-queries.
func (rt *Router) handleSaveDatabaseQuery(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := rt.queryOwner(w, r)
	if !ok {
		return
	}
	limits := dbviewer.LimitsFromEnv(nil)
	var req savedQueryRequest
	r.Body = http.MaxBytesReader(w, r.Body, int64(limits.SQLBytes+queryBodyMargin))
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	switch {
	case req.Name == "" || len(req.Name) > maxSavedQueryNameLen:
		writeError(w, http.StatusBadRequest, "name must be 1 to "+strconv.Itoa(maxSavedQueryNameLen)+" characters")
		return
	case strings.TrimSpace(req.SQL) == "" || len(req.SQL) > limits.SQLBytes:
		writeError(w, http.StatusBadRequest, "sql is required and must fit the statement size limit")
		return
	}
	name := r.PathValue("name")
	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) && !rt.isExternalDatabase(r, name) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.internalError(w, "api: save database query: load database failed", err, slog.String("name", name))
		return
	}
	q, err := rt.dbQueries.SaveDatabaseQuery(r.Context(), store.DatabaseSavedQuery{
		DatabaseName: name, PrincipalType: pt, PrincipalID: pid, Name: req.Name, SQL: req.SQL,
	})
	if errors.Is(err, store.ErrSavedQueryExists) {
		writeError(w, http.StatusConflict, "a saved query with that name already exists")
		return
	}
	if err != nil {
		rt.internalError(w, "api: save database query failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusCreated, savedQueryResource{ID: q.ID, Name: q.Name, SQL: q.SQL, CreatedAt: q.CreatedAt})
}

// handleDeleteDatabaseSavedQuery handles DELETE
// /api/v1/databases/{name}/saved-queries/{id}.
func (rt *Router) handleDeleteDatabaseSavedQuery(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := rt.queryOwner(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	err := rt.dbQueries.DeleteDatabaseSavedQuery(r.Context(), name, pt, pid, r.PathValue("id"))
	if errors.Is(err, store.ErrSavedQueryNotFound) {
		writeError(w, http.StatusNotFound, "saved query not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete database saved query failed", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
