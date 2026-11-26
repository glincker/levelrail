package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dynamoViewerTarget resolves a managed DynamoDB local container to exec curl in.
// The engine has no credentials, so nothing secret is read or sent.
func (rt *Router) dynamoViewerTarget(w http.ResponseWriter, r *http.Request) (dbviewer.Target, *store.DesiredDatabase, bool) {
	name := r.PathValue("name")
	if !experimental.Enabled(experimental.DynamoDB) {
		writeError(w, http.StatusNotFound, "database not found")
		return dbviewer.Target{}, nil, false
	}
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return dbviewer.Target{}, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: dynamo viewer: load database failed", err, slog.String("name", name))
		return dbviewer.Target{}, nil, false
	}
	if desired.Engine != store.EngineDynamoDB {
		writeError(w, http.StatusBadRequest, "dynamodb explorer is not supported for engine "+desired.Engine)
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
		Dialect:     dbviewer.DialectDynamoDB,
		Limits:      rt.databaseViewerLimits(r.Context()),
	}, desired, true
}

// dynamoLimits reads the live page limits on every request.
func (rt *Router) dynamoLimits(r *http.Request) dbviewer.DynamoLimits {
	ctx := r.Context()
	return dbviewer.DynamoLimits{
		PageItems:    rt.runtimeSettings.Int(ctx, runtimesettings.KeyDynamoPageItems),
		PageItemsMax: rt.runtimeSettings.Int(ctx, runtimesettings.KeyDynamoPageItemsMax),
		DescribeMax:  rt.runtimeSettings.Int(ctx, runtimesettings.KeyDynamoDescribeMax),
	}
}

// runDynamo guards, runs and logs one typed request. History names the shape only, never key values.
func (rt *Router) runDynamo(w http.ResponseWriter, r *http.Request, target dbviewer.Target, desired *store.DesiredDatabase, req dbviewer.DynamoRequest) (dbviewer.DynamoResult, dbviewer.DynamoPrepared, bool) {
	q, err := dbviewer.CheckDynamo(req, target.Limits, rt.dynamoLimits(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return dbviewer.DynamoResult{}, q, false
	}
	principal := "anonymous"
	if pt, pid, _, perr := rt.callerPrincipal(r); perr == nil {
		principal = "p:" + pt + ":" + pid
	}
	limit := max(1, rt.runtimeSettings.DatabaseViewerLimits(r.Context()).MaxConcurrent)
	wait := time.Duration(max(0, rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDBViewerQueueWaitMs))) * time.Millisecond
	release, ok := mongoReads.acquire(r.Context(), "db:"+desired.Name, principal, limit, wait)
	if !ok {
		if r.Context().Err() != nil {
			return dbviewer.DynamoResult{}, q, false
		}
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "too many reads in flight, retry shortly")
		return dbviewer.DynamoResult{}, q, false
	}
	defer release()

	res, err := target.RunDynamo(r.Context(), q)
	label := "dynamodb " + string(q.Op)
	if req.Table != "" {
		label += " " + req.Table
	}
	attrs := []any{slog.String("database", desired.Name), slog.String("op", string(q.Op)), slog.String("fingerprint", q.Fingerprint)}
	if err != nil {
		rt.logger.Info("api: dynamo viewer read failed", attrs...)
		rt.recordQuery(r, desired.Name, "dynamodb", label, q.Fingerprint, false, mongoHistoryError(err), 0, 0)
		rt.writeViewerError(w, "api: dynamo viewer read failed", desired.Name, err)
		return dbviewer.DynamoResult{}, q, false
	}
	rt.logger.Info("api: dynamo viewer read", append(attrs, slog.Int("items", len(res.Items)), slog.Int64("duration_ms", res.DurationMs))...)
	rt.recordQuery(r, desired.Name, "dynamodb", label, q.Fingerprint, true, "", res.DurationMs, len(res.Items))
	return res, q, true
}

type dynamoTablesResponse struct {
	Tables    []json.RawMessage `json:"tables"`
	Names     []string          `json:"names"`
	Truncated bool              `json:"truncated"`
	// Note is an i18n key: counts and sizes are estimates the engine refreshes lazily.
	Note string `json:"note"`
}

// handleGetDynamoTables handles GET /api/v1/databases/{name}/dynamo/tables: names plus a describe of the first few.
func (rt *Router) handleGetDynamoTables(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.dynamoViewerTarget(w, r)
	if !ok {
		return
	}
	list, _, ok := rt.runDynamo(w, r, target, desired, dbviewer.DynamoRequest{Op: dbviewer.DynamoListTables, StartTable: r.URL.Query().Get("after")})
	if !ok {
		return
	}
	resp := dynamoTablesResponse{Tables: []json.RawMessage{}, Names: list.Tables, Truncated: list.LastTable != "", Note: "estimate"}
	if resp.Names == nil {
		resp.Names = []string{}
	}
	describeMax := rt.dynamoLimits(r).DescribeMax
	for i, tn := range resp.Names {
		if i >= describeMax {
			resp.Truncated = true
			break
		}
		d, _, ok := rt.runDynamo(w, r, target, desired, dbviewer.DynamoRequest{Op: dbviewer.DynamoDescribeTable, Table: tn})
		if !ok {
			return
		}
		ttl, _, ok := rt.runDynamo(w, r, target, desired, dbviewer.DynamoRequest{Op: dbviewer.DynamoDescribeTTL, Table: tn})
		if !ok {
			return
		}
		resp.Tables = append(resp.Tables, mergeDynamoTable(d.Table, ttl.TTL))
	}
	writeJSON(w, http.StatusOK, resp)
}

// mergeDynamoTable folds the TTL description into the table description under one key.
func mergeDynamoTable(table, ttl json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(table, &m) != nil {
		return table
	}
	if len(ttl) > 0 {
		m["TimeToLive"] = ttl
	}
	out, err := json.Marshal(m)
	if err != nil {
		return table
	}
	return out
}

type dynamoReadRequest struct {
	Table    string                   `json:"table"`
	Index    string                   `json:"index"`
	KeyName  string                   `json:"key_name"`
	KeyValue json.RawMessage          `json:"key_value"`
	Sort     *dbviewer.DynamoSortCond `json:"sort"`
	Limit    int                      `json:"limit"`
	StartKey json.RawMessage          `json:"exclusive_start_key"`
	Confirm  bool                     `json:"confirm"`
}

type dynamoItemsResponse struct {
	Items            []json.RawMessage `json:"items"`
	Count            int               `json:"count"`
	ScannedCount     int               `json:"scanned_count"`
	LastKey          json.RawMessage   `json:"last_evaluated_key,omitempty"`
	ConsumedCapacity json.RawMessage   `json:"consumed_capacity,omitempty"`
	Truncated        bool              `json:"truncated"`
	Expensive        bool              `json:"expensive"`
	Fingerprint      string            `json:"fingerprint"`
	DurationMs       int64             `json:"duration_ms"`
}

func (rt *Router) dynamoRead(w http.ResponseWriter, r *http.Request, op dbviewer.DynamoOp) {
	target, desired, ok := rt.dynamoViewerTarget(w, r)
	if !ok {
		return
	}
	body, ok := decodeJSON[dynamoReadRequest](w, r, int64(4*target.Limits.SQLBytes+mongoBodyMargin))
	if !ok {
		return
	}
	res, q, ok := rt.runDynamo(w, r, target, desired, dbviewer.DynamoRequest{
		Op: op, Table: body.Table, Index: body.Index, KeyName: body.KeyName, KeyValue: body.KeyValue,
		Sort: body.Sort, Limit: body.Limit, StartKey: body.StartKey, Confirm: body.Confirm,
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dynamoItemsResponse{
		Items: res.Items, Count: res.Count, ScannedCount: res.ScannedCount, LastKey: res.LastKey,
		ConsumedCapacity: res.ConsumedCapacity, Truncated: res.Truncated, Expensive: op == dbviewer.DynamoScan,
		Fingerprint: q.Fingerprint, DurationMs: res.DurationMs,
	})
}

// handleDynamoQuery handles POST /api/v1/databases/{name}/dynamo/query: a key condition read on a table or index.
func (rt *Router) handleDynamoQuery(w http.ResponseWriter, r *http.Request) {
	rt.dynamoRead(w, r, dbviewer.DynamoQuery)
}

// handleDynamoScan handles POST /api/v1/databases/{name}/dynamo/scan: one page of a full scan, only with confirm true.
func (rt *Router) handleDynamoScan(w http.ResponseWriter, r *http.Request) {
	rt.dynamoRead(w, r, dbviewer.DynamoScan)
}
