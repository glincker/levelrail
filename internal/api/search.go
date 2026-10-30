package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/search"
)

// searchService builds the index service on first use; it stays nil when the
// store has no SQL handle, and the routes answer 501.
func (rt *Router) searchService() *search.Service {
	rt.searchOnce.Do(func() {
		db, ok := rt.apps.(search.DB)
		if !ok {
			return
		}
		rt.searchSvc = search.New(searchReadDB(db), rt.logger, rt.searchProviders()...)
	})
	return rt.searchSvc
}

// searchReadDB sends the index's SELECTs to the read pool when the store has
// one; writes and transactions stay on the writer connection.
func searchReadDB(db search.DB) search.DB {
	rd, ok := db.(searchReader)
	if !ok {
		return db
	}
	return searchReadDBConn{DB: db, rd: rd}
}

type searchReader interface {
	ReadQueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ReadQueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type searchReadDBConn struct {
	search.DB
	rd searchReader
}

func (c searchReadDBConn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.rd.ReadQueryContext(ctx, query, args...)
}

func (c searchReadDBConn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.rd.ReadQueryRowContext(ctx, query, args...)
}

func (rt *Router) markSearchDirty() {
	if svc := rt.searchService(); svc != nil {
		svc.MarkDirty()
	}
}

func (rt *Router) searchOrFail(w http.ResponseWriter) *search.Service {
	svc := rt.searchService()
	if svc == nil {
		writeError(w, http.StatusNotImplemented, "search is not available on this store")
	}
	return svc
}

func (rt *Router) searchFailure(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, search.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, search.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		rt.internalError(w, msg, err)
	}
}

// handleSearch handles GET /api/v1/search?q=&kind=&limit=&cursor=.
func (rt *Router) handleSearch(w http.ResponseWriter, r *http.Request) {
	svc := rt.searchOrFail(w)
	if svc == nil {
		return
	}
	caller, err := rt.newSearchCaller(r)
	if err != nil {
		rt.internalError(w, "api: search: resolve caller failed", err)
		return
	}
	qv := r.URL.Query()
	req := search.Request{Q: qv.Get("q"), Cursor: qv.Get("cursor")}
	if k := qv.Get("kind"); k != "" {
		kind, ok := search.ParseKind(k)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown kind "+strconv.Quote(k))
			return
		}
		req.Kind = kind
	}
	if l := qv.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		req.Limit = n
	}
	resp, err := svc.Search(r.Context(), caller, req)
	if err != nil {
		rt.searchFailure(w, "api: search failed", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSearchSuggest handles GET /api/v1/search/suggest?q=.
func (rt *Router) handleSearchSuggest(w http.ResponseWriter, r *http.Request) {
	svc := rt.searchOrFail(w)
	if svc == nil {
		return
	}
	caller, err := rt.newSearchCaller(r)
	if err != nil {
		rt.internalError(w, "api: search suggest: resolve caller failed", err)
		return
	}
	out, err := svc.Suggest(r.Context(), caller, r.URL.Query().Get("q"))
	if err != nil {
		rt.searchFailure(w, "api: search suggest failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": out})
}

type searchSavedRequest struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

type searchUsageRequest struct {
	Ref string `json:"ref"`
}

func (rt *Router) searchPrincipal(w http.ResponseWriter, r *http.Request) (string, bool) {
	t, id, _, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: search: resolve caller failed", err)
		return "", false
	}
	return t + ":" + id, true
}

// handleListSavedSearches handles GET /api/v1/search/saved: the caller's own.
func (rt *Router) handleListSavedSearches(w http.ResponseWriter, r *http.Request) {
	svc := rt.searchOrFail(w)
	if svc == nil {
		return
	}
	principal, ok := rt.searchPrincipal(w, r)
	if !ok {
		return
	}
	out, err := svc.ListSaved(r.Context(), principal)
	if err != nil {
		rt.searchFailure(w, "api: list saved searches failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": out})
}

// handleSaveSearch handles POST /api/v1/search/saved.
func (rt *Router) handleSaveSearch(w http.ResponseWriter, r *http.Request) {
	svc := rt.searchOrFail(w)
	if svc == nil {
		return
	}
	principal, ok := rt.searchPrincipal(w, r)
	if !ok {
		return
	}
	var body searchSavedRequest
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	out, err := svc.SaveSearch(r.Context(), principal, body.Name, body.Query)
	if err != nil {
		rt.searchFailure(w, "api: save search failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleDeleteSavedSearch handles DELETE /api/v1/search/saved/{id}.
func (rt *Router) handleDeleteSavedSearch(w http.ResponseWriter, r *http.Request) {
	svc := rt.searchOrFail(w)
	if svc == nil {
		return
	}
	principal, ok := rt.searchPrincipal(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteSaved(r.Context(), principal, r.PathValue("id")); err != nil {
		rt.searchFailure(w, "api: delete saved search failed", err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// handleSearchUsage handles POST /api/v1/search/usage: a hit the caller opened.
func (rt *Router) handleSearchUsage(w http.ResponseWriter, r *http.Request) {
	svc := rt.searchOrFail(w)
	if svc == nil {
		return
	}
	caller, err := rt.newSearchCaller(r)
	if err != nil {
		rt.internalError(w, "api: search usage: resolve caller failed", err)
		return
	}
	var body searchUsageRequest
	if err := decodeJSONBody(r, &body); err != nil || body.Ref == "" {
		writeError(w, http.StatusBadRequest, "ref is required")
		return
	}
	if err := svc.RecordUse(r.Context(), caller, body.Ref); err != nil {
		rt.searchFailure(w, "api: record search usage failed", err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
