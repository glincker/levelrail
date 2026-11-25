package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

// mongoBodyMargin leaves room for the JSON envelope around the four
// validated fields, each capped at the statement limit.
const mongoBodyMargin = 4096

// mongoGate bounds in-flight Mongo reads per database and per principal so
// a few slow scans cannot starve the exec path.
type mongoGate struct {
	mu   sync.Mutex
	busy map[string]int
}

var mongoReads = &mongoGate{busy: map[string]int{}}

// acquire takes a slot on both keys or neither; release undoes it.
func (g *mongoGate) acquire(db, principal string, limit int) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy[db] >= limit || g.busy[principal] >= limit {
		return nil, false
	}
	g.busy[db]++
	g.busy[principal]++
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		for _, k := range []string{db, principal} {
			if g.busy[k]--; g.busy[k] <= 0 {
				delete(g.busy, k)
			}
		}
	}, true
}

// mongoViewerTarget resolves a managed MongoDB container to exec mongosh in.
// Credentials stay in the container, as for the SQL viewer.
func (rt *Router) mongoViewerTarget(w http.ResponseWriter, r *http.Request) (dbviewer.Target, *store.DesiredDatabase, bool) {
	name := r.PathValue("name")
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return dbviewer.Target{}, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: mongo viewer: load database failed", err, slog.String("name", name))
		return dbviewer.Target{}, nil, false
	}
	if desired.Engine != store.EngineMongoDB {
		writeError(w, http.StatusBadRequest, "mongo viewer is not supported for engine "+desired.Engine)
		return dbviewer.Target{}, desired, false
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return dbviewer.Target{}, desired, false
	}
	nodeRuntime, state, ok := rt.resolveDatabaseExecContainer(w, r, name, desired.NodeID)
	if !ok {
		return dbviewer.Target{}, desired, false
	}
	return dbviewer.Target{
		Exec:        nodeRuntime,
		ContainerID: state.ID,
		Dialect:     dbviewer.DialectMongo,
		Limits:      rt.databaseViewerLimits(r.Context()),
	}, desired, true
}

// runMongo guards, runs and logs one typed request. Reads are logged with a
// value-free fingerprint, never the filter itself.
func (rt *Router) runMongo(w http.ResponseWriter, r *http.Request, target dbviewer.Target, desired *store.DesiredDatabase, req dbviewer.MongoRequest) (dbviewer.MongoResult, dbviewer.MongoQuery, bool) {
	q, err := dbviewer.CheckMongo(req, target.Limits)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return dbviewer.MongoResult{}, q, false
	}
	principal := "anonymous"
	if pt, pid, _, perr := rt.callerPrincipal(r); perr == nil {
		principal = "p:" + pt + ":" + pid
	}
	limit := max(1, rt.runtimeSettings.DatabaseViewerLimits(r.Context()).MaxConcurrent)
	release, ok := mongoReads.acquire("db:"+desired.Name, principal, limit)
	if !ok {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "too many document reads in flight, retry shortly")
		return dbviewer.MongoResult{}, q, false
	}
	defer release()

	res, err := target.RunMongo(r.Context(), q)
	// History names the target and shape only; filter values may hold personal data.
	label := "mongo " + string(q.Op)
	if req.Database != "" {
		label += " " + req.Database
	}
	if req.Collection != "" {
		label += "." + req.Collection
	}
	attrs := []any{
		slog.String("database", desired.Name), slog.String("op", string(q.Op)),
		slog.String("target", strings.TrimPrefix(label, "mongo ")),
		slog.String("fingerprint", q.Fingerprint),
	}
	if err != nil {
		rt.logger.Info("api: mongo viewer read failed", attrs...)
		rt.recordQuery(r, desired.Name, "mongo", label, q.Fingerprint, false, mongoHistoryError(err), 0, 0)
		rt.writeViewerError(w, "api: mongo viewer read failed", desired.Name, err)
		return dbviewer.MongoResult{}, q, false
	}
	rt.logger.Info("api: mongo viewer read", append(attrs, slog.Int("docs", len(res.Documents)), slog.Int64("duration_ms", res.DurationMs))...)
	rt.recordQuery(r, desired.Name, "mongo", label, q.Fingerprint, true, "", res.DurationMs, len(res.Documents))
	return res, q, true
}

// mongoHistoryError keeps server error text out of history: it can echo filter
// values. The full message still goes back in the HTTP response.
func mongoHistoryError(err error) string {
	var qe *dbviewer.QueryError
	if errors.As(err, &qe) {
		return "query rejected by server"
	}
	return queryErrorText(err)
}

type mongoCollectionsResponse struct {
	Databases   []json.RawMessage `json:"databases"`
	Collections []json.RawMessage `json:"collections"`
	Truncated   bool              `json:"truncated"`
	Fingerprint string            `json:"fingerprint"`
}

// handleGetMongoCollections handles GET /api/v1/databases/{name}/mongo/collections.
func (rt *Router) handleGetMongoCollections(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.mongoViewerTarget(w, r)
	if !ok {
		return
	}
	res, q, ok := rt.runMongo(w, r, target, desired, dbviewer.MongoRequest{
		Op: dbviewer.MongoCollections, Database: r.URL.Query().Get("database"),
	})
	if !ok {
		return
	}
	resp := mongoCollectionsResponse{
		Databases: []json.RawMessage{}, Collections: []json.RawMessage{},
		Truncated: res.Truncated, Fingerprint: q.Fingerprint,
	}
	for _, d := range res.Documents {
		var kind struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(d, &kind)
		if kind.Kind == "db" {
			resp.Databases = append(resp.Databases, d)
		} else {
			resp.Collections = append(resp.Collections, d)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type mongoFindRequest struct {
	Database   string          `json:"database"`
	Collection string          `json:"collection"`
	Filter     json.RawMessage `json:"filter"`
	Sort       json.RawMessage `json:"sort"`
	Projection json.RawMessage `json:"projection"`
	Limit      int             `json:"limit"`
	// After is the _id of the last document of the previous page.
	After json.RawMessage `json:"after"`
}

type mongoFindResponse struct {
	Documents   []json.RawMessage `json:"documents"`
	Truncated   bool              `json:"truncated"`
	NextAfter   json.RawMessage   `json:"next_after,omitempty"`
	Fingerprint string            `json:"fingerprint"`
	DurationMs  int64             `json:"duration_ms"`
}

// handleMongoFind handles POST /api/v1/databases/{name}/mongo/find: one
// read-only find, paged by _id.
func (rt *Router) handleMongoFind(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.mongoViewerTarget(w, r)
	if !ok {
		return
	}
	body, ok := decodeJSON[mongoFindRequest](w, r, int64(4*target.Limits.SQLBytes+mongoBodyMargin))
	if !ok {
		return
	}
	res, q, ok := rt.runMongo(w, r, target, desired, dbviewer.MongoRequest{
		Op: dbviewer.MongoFind, Database: body.Database, Collection: body.Collection,
		Filter: body.Filter, Sort: body.Sort, Projection: body.Projection,
		Limit: body.Limit, After: body.After,
	})
	if !ok {
		return
	}
	resp := mongoFindResponse{Documents: res.Documents, Truncated: res.Truncated, Fingerprint: q.Fingerprint, DurationMs: res.DurationMs}
	if res.Truncated {
		resp.NextAfter = res.NextAfter
	}
	writeJSON(w, http.StatusOK, resp)
}
