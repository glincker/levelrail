package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultArchiveInterval = time.Hour
	minArchiveInterval     = 5 * time.Minute
	maxArchiveInterval     = 24 * time.Hour
	maxArchiveRetention    = 3650
	archiveListPageSize    = 100
)

type logArchivePolicyResource struct {
	ID              string `json:"id"`
	AppName         string `json:"app_name"`
	TargetID        string `json:"target_id"`
	Enabled         bool   `json:"enabled"`
	Interval        string `json:"interval"`
	IntervalSeconds int64  `json:"interval_seconds"`
	RetentionDays   int    `json:"retention_days"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	LastSuccessAt   string `json:"last_success_at,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	CreatedAt       string `json:"created_at"`
}

func toLogArchivePolicyResource(p store.LogArchivePolicy) logArchivePolicyResource {
	return logArchivePolicyResource{
		ID: p.ID, AppName: p.AppName, TargetID: p.TargetID, Enabled: p.Enabled,
		Interval: (time.Duration(p.IntervalSeconds) * time.Second).String(), IntervalSeconds: p.IntervalSeconds,
		RetentionDays: p.RetentionDays, LastRunAt: p.LastRunAt, LastSuccessAt: p.LastSuccessAt, LastError: p.LastError, CreatedAt: p.CreatedAt,
	}
}

type logArchiveRunResource struct {
	ID         string `json:"id"`
	AppName    string `json:"app_name"`
	TargetID   string `json:"target_id"`
	Kind       string `json:"kind"`
	From       string `json:"from"`
	To         string `json:"to"`
	Status     string `json:"status"`
	Objects    int    `json:"objects"`
	Lines      int64  `json:"lines"`
	Bytes      int64  `json:"bytes"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func toLogArchiveRunResource(r store.LogArchiveRun) logArchiveRunResource {
	return logArchiveRunResource{
		ID: r.ID, AppName: r.AppName, TargetID: r.TargetID, Kind: r.Kind,
		From: time.Unix(0, r.FromNs).UTC().Format(time.RFC3339), To: time.Unix(0, r.ToNs).UTC().Format(time.RFC3339),
		Status: r.Status, Objects: r.Objects, Lines: r.Lines, Bytes: r.Bytes, Error: r.Error, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

type logArchivePolicyRequest struct {
	AppName       string `json:"app_name"`
	TargetID      string `json:"target_id"`
	Interval      string `json:"interval,omitempty"`
	RetentionDays int    `json:"retention_days"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

func (rt *Router) appExistsOrEmpty(w http.ResponseWriter, r *http.Request, app string) bool {
	if app == "" {
		return true
	}
	_, found, err := rt.lookupAppResource(r.Context(), app)
	if err != nil {
		rt.logger.Error("api: log archive: load app failed", slog.String("error", err.Error()), slog.String("name", app))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	if !found {
		writeError(w, http.StatusNotFound, "app not found")
		return false
	}
	return true
}

// handleListLogArchivePolicies handles GET /api/v1/log-archive/policies.
func (rt *Router) handleListLogArchivePolicies(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	policies, err := d.Archive.ListLogArchivePolicies(r.Context())
	if err != nil {
		rt.logger.Error("api: list log archive policies failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]logArchivePolicyResource, 0, len(policies))
	for _, p := range policies {
		out = append(out, toLogArchivePolicyResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetLogArchivePolicy handles PUT /api/v1/log-archive/policy: create or
// replace the policy for an app, or the global one when app_name is empty.
func (rt *Router) handleSetLogArchivePolicy(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	var req logArchivePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	interval := defaultArchiveInterval
	if req.Interval != "" {
		var err error
		if interval, err = time.ParseDuration(req.Interval); err != nil {
			writeError(w, http.StatusBadRequest, "interval must be a duration such as 15m or 1h")
			return
		}
	}
	if interval < minArchiveInterval || interval > maxArchiveInterval {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("interval must be between %s and %s", minArchiveInterval, maxArchiveInterval))
		return
	}
	if req.RetentionDays < 0 || req.RetentionDays > maxArchiveRetention {
		writeError(w, http.StatusBadRequest, "retention_days must be between 0 (keep forever) and 3650")
		return
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if _, ok := rt.loadStorageTarget(w, r, req.TargetID); !ok {
		return
	}
	if !rt.appExistsOrEmpty(w, r, req.AppName) {
		return
	}

	now := time.Now().UTC()
	id, err := randomLogArchiveID("lap_")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	enabled := req.Enabled == nil || *req.Enabled
	p := store.LogArchivePolicy{ID: id, AppName: req.AppName, TargetID: req.TargetID, Enabled: enabled, IntervalSeconds: int64(interval / time.Second), RetentionDays: req.RetentionDays, WatermarkNs: now.UnixNano(), CreatedAt: now.Format(time.RFC3339)}
	if err := d.Archive.UpsertLogArchivePolicy(r.Context(), p); err != nil {
		rt.logger.Error("api: set log archive policy failed", slog.String("error", err.Error()), slog.String("app", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	saved, err := d.Archive.GetLogArchivePolicy(r.Context(), req.AppName)
	if err != nil {
		rt.logger.Error("api: reload log archive policy failed", slog.String("error", err.Error()), slog.String("app", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toLogArchivePolicyResource(saved))
}

// handleDeleteLogArchivePolicy handles DELETE /api/v1/log-archive/policy?app=.
func (rt *Router) handleDeleteLogArchivePolicy(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	err := d.Archive.DeleteLogArchivePolicy(r.Context(), r.URL.Query().Get("app"))
	if errors.Is(err, store.ErrLogArchivePolicyNotFound) {
		writeError(w, http.StatusNotFound, "log archive policy not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete log archive policy failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type logArchiveDumpRequest struct {
	AppName  string `json:"app_name"`
	TargetID string `json:"target_id"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// handleLogArchiveDump handles POST /api/v1/log-archive/dump. It answers 202
// with the running run; poll GET /api/v1/log-archive/runs for the outcome.
func (rt *Router) handleLogArchiveDump(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	var req logArchiveDumpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	from, errFrom := time.Parse(time.RFC3339, req.From)
	to, errTo := time.Parse(time.RFC3339, req.To)
	if errFrom != nil || errTo != nil {
		writeError(w, http.StatusBadRequest, "from and to must be RFC3339 timestamps")
		return
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if !rt.appExistsOrEmpty(w, r, req.AppName) {
		return
	}
	run, err := d.Archiver.StartDump(r.Context(), objectstore.DumpRequest{TargetID: req.TargetID, AppName: req.AppName, From: from, To: to})
	if errors.Is(err, objectstore.ErrInvalidDump) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "storage destination not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: log archive dump failed to start", slog.String("error", err.Error()), slog.String("app", req.AppName))
		writeError(w, http.StatusInternalServerError, "could not start the dump")
		return
	}
	writeJSON(w, http.StatusAccepted, toLogArchiveRunResource(run))
}

// handleListLogArchiveRuns handles GET /api/v1/log-archive/runs?app=&all=true.
func (rt *Router) handleListLogArchiveRuns(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 200 {
		limit = n
	}
	all := r.URL.Query().Get("all") == "true"
	runs, err := d.Archive.ListLogArchiveRuns(r.Context(), r.URL.Query().Get("app"), all, limit)
	if err != nil {
		rt.logger.Error("api: list log archive runs failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]logArchiveRunResource, 0, len(runs))
	for _, run := range runs {
		out = append(out, toLogArchiveRunResource(run))
	}
	writeJSON(w, http.StatusOK, out)
}

type logArchiveObjectsResponse struct {
	Objects []objectstore.Object `json:"objects"`
	Next    string               `json:"next,omitempty"`
}

// handleListLogArchiveObjects handles GET /api/v1/log-archive/objects?target_id=&app=&cursor=.
func (rt *Router) handleListLogArchiveObjects(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	c, ok := rt.archiveClient(w, r, d, q.Get("target_id"))
	if !ok {
		return
	}
	prefix := d.ArchiveRoot + "/"
	if app := q.Get("app"); app != "" {
		prefix = objectstore.AppPrefix(d.ArchiveRoot, app)
	}
	page, err := c.List(r.Context(), prefix, q.Get("cursor"), archiveListPageSize)
	if err != nil {
		reason, msg := objectstore.Classify(err)
		rt.logger.Warn("api: list log archive objects failed", slog.String("reason", reason), slog.String("target_id", q.Get("target_id")))
		writeError(w, http.StatusBadGateway, msg)
		return
	}
	writeJSON(w, http.StatusOK, logArchiveObjectsResponse{Objects: page.Objects, Next: page.Next})
}

// handleDownloadLogArchiveObject handles GET /api/v1/log-archive/objects/download?target_id=&key=.
func (rt *Router) handleDownloadLogArchiveObject(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	key := q.Get("key")
	if !objectstore.IsArchiveKey(d.ArchiveRoot, key) {
		writeError(w, http.StatusBadRequest, "key is not a log archive object")
		return
	}
	c, ok := rt.archiveClient(w, r, d, q.Get("target_id"))
	if !ok {
		return
	}
	body, size, err := c.Get(r.Context(), key)
	if err != nil {
		reason, msg := objectstore.Classify(err)
		rt.logger.Warn("api: download log archive object failed", slog.String("reason", reason), slog.String("target_id", q.Get("target_id")))
		writeError(w, http.StatusBadGateway, msg)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(key)))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if _, err := io.Copy(w, body); err != nil {
		rt.logger.Warn("api: stream log archive object interrupted", slog.String("error", err.Error()))
	}
}

func (rt *Router) archiveClient(w http.ResponseWriter, r *http.Request, d *StorageDeps, targetID string) (*objectstore.Client, bool) {
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return nil, false
	}
	if _, ok := rt.loadStorageTarget(w, r, targetID); !ok {
		return nil, false
	}
	c, _, err := d.Clients.Client(r.Context(), targetID)
	if err != nil {
		rt.logger.Error("api: log archive: resolve destination failed", slog.String("error", err.Error()), slog.String("target_id", targetID))
		writeError(w, http.StatusBadGateway, "could not load the destination credentials")
		return nil, false
	}
	return c, true
}
