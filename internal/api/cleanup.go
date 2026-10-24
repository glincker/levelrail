package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cleanup"
	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	cleanupRunsDefaultLimit = 50
	cleanupRunsMaxLimit     = 500
	envCleanupExportMax     = "APP_CLEANUP_EXPORT_MAX"
	defaultCleanupExportMax = 10000
	// CleanupPreviewKey is the stable key of the informational first-run review item.
	CleanupPreviewKey = "cleanup.preview_mode"
)

// SetCleanup enables the /api/v1/cleanup routes.
func (rt *Router) SetCleanup(c *cleanup.Center) { rt.cleanup = c }

// EmitEvent queues a platform event for the event webhooks; it is a no-op when none are configured.
func (rt *Router) EmitEvent(eventType string, data map[string]any) {
	if rt.eventWebhooks == nil {
		return
	}
	rt.eventWebhooks.Emit(eventhook.Event{Type: eventType, Data: data})
}

func (rt *Router) cleanupOr501(w http.ResponseWriter) bool {
	if rt.cleanup == nil {
		writeError(w, http.StatusNotImplemented, "cleanup is not available on this control plane")
		return false
	}
	return true
}

type cleanupRunRequest struct {
	Days   *int `json:"days"`
	DryRun bool `json:"dry_run"`
}

type cleanupModeRequest struct {
	Mode string `json:"mode"`
}

type cleanupRunResponse struct {
	Rows          int64  `json:"rows"`
	Bytes         int64  `json:"bytes"`
	MoreRemaining bool   `json:"more_remaining"`
	Error         string `json:"error"`
}

type cleanupRunsResponse struct {
	Runs       []cleanup.RunRecord `json:"runs"`
	NextCursor string              `json:"next_cursor"`
}

func (rt *Router) writeCleanupError(w http.ResponseWriter, what string, err error) {
	var fe *cleanup.FloorError
	switch {
	case errors.As(err, &fe):
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{Error: fe.Error(), Code: errCodeInvalidValue, Field: fe.Key})
	case errors.Is(err, cleanup.ErrUnknownTarget):
		writeError(w, http.StatusNotFound, "unknown cleanup target")
	case errors.Is(err, cleanup.ErrBusy), errors.Is(err, cleanup.ErrReadOnly):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, cleanup.ErrNoRetention), errors.Is(err, cleanup.ErrInvalidDays):
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{Error: err.Error(), Code: errCodeInvalidValue, Field: "days"})
	default:
		rt.internalError(w, what, err)
	}
}

// handleGetCleanup handles GET /api/v1/cleanup (AbilityRead).
func (rt *Router) handleGetCleanup(w http.ResponseWriter, r *http.Request) {
	if !rt.cleanupOr501(w) {
		return
	}
	ov, err := rt.cleanup.List(r.Context())
	if err != nil {
		rt.internalError(w, "api: list cleanup targets", err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// handlePreviewCleanup handles POST /api/v1/cleanup/{id}/preview (AbilityRoot).
func (rt *Router) handlePreviewCleanup(w http.ResponseWriter, r *http.Request) {
	if !rt.cleanupOr501(w) {
		return
	}
	var req cleanupRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	imp, err := rt.cleanup.Preview(r.Context(), r.PathValue("id"), req.Days)
	if err != nil {
		rt.writeCleanupError(w, "api: preview cleanup", err)
		return
	}
	writeJSON(w, http.StatusOK, cleanupRunResponse{Rows: imp.Rows, Bytes: imp.Bytes})
}

func cleanupTriggerFor(r *http.Request) cleanup.Trigger {
	switch clientKindFromUserAgent(r.Header.Get("User-Agent")) {
	case ClientKindCLI:
		return cleanup.TriggerCLI
	case ClientKindDashboard:
		return cleanup.TriggerManual
	default:
		return cleanup.TriggerAPI
	}
}

// handleRunCleanup handles POST /api/v1/cleanup/{id}/run (AbilityRoot). A failure of the run itself is
// reported in the body's error field; the run and its failure are recorded in history either way.
func (rt *Router) handleRunCleanup(w http.ResponseWriter, r *http.Request) {
	if !rt.cleanupOr501(w) {
		return
	}
	var req cleanupRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	who := rt.accessActor(r)
	_, res, err := rt.cleanup.Run(r.Context(), r.PathValue("id"), cleanup.Request{Days: req.Days, DryRun: req.DryRun, Opts: cleanup.RunOptions{
		Trigger: cleanupTriggerFor(r), Actor: rt.eventActor(r), ActorType: who.kind, ActorID: who.id, RemoteAddr: clientIP(r),
	}})
	if err != nil {
		rt.writeCleanupError(w, "api: run cleanup", err)
		return
	}
	out := cleanupRunResponse{Rows: res.Rows, Bytes: res.Bytes, MoreRemaining: res.More}
	if res.Err != nil {
		out.Error = res.Err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetCleanupMode handles PUT /api/v1/cleanup/{id}/mode (AbilityRoot). The mode is a runtime setting,
// so the change writes the usual settings_change audit row.
func (rt *Router) handleSetCleanupMode(w http.ResponseWriter, r *http.Request) {
	if !rt.cleanupOr501(w) || !rt.runtimeSettingsOr501(w) {
		return
	}
	id := r.PathValue("id")
	var req cleanupModeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cur, err := rt.cleanup.Get(r.Context(), id)
	if err != nil {
		rt.writeCleanupError(w, "api: get cleanup target", err)
		return
	}
	if cur.ReadThrough {
		rt.writeCleanupError(w, "api: set cleanup mode", cleanup.ErrReadOnly)
		return
	}
	raw, _ := json.Marshal(req.Mode)
	key := cleanup.ModeKey(id)
	before, err := rt.runtimeSettings.Get(r.Context(), key)
	if err != nil {
		rt.writeSettingError(w, "api: set cleanup mode", err)
		return
	}
	after, err := rt.runtimeSettings.Set(r.Context(), key, raw, rt.eventActor(r))
	if err != nil {
		var verr *runtimesettings.ValidationError
		if errors.As(err, &verr) {
			writeJSON(w, http.StatusBadRequest, runtimeSettingError{Error: verr.Message, Code: errCodeInvalidValue, Field: "mode"})
			return
		}
		rt.writeSettingError(w, "api: set cleanup mode", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	view, err := rt.cleanup.Get(r.Context(), id)
	if err != nil {
		rt.writeCleanupError(w, "api: get cleanup target", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func parseCleanupTime(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, true
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func cleanupRunsFilter(r *http.Request) (cleanup.RunsFilter, string) {
	q := r.URL.Query()
	f := cleanup.RunsFilter{Target: q.Get("target"), Status: q.Get("status"), Limit: cleanupRunsDefaultLimit}
	var ok bool
	if f.Since, ok = parseCleanupTime(q.Get("since")); !ok {
		return f, "since must be an RFC 3339 time or a date"
	}
	if f.Until, ok = parseCleanupTime(q.Get("until")); !ok {
		return f, "until must be an RFC 3339 time or a date"
	}
	if f.Status != "" && f.Status != "ok" && f.Status != "failed" {
		return f, "status must be ok or failed"
	}
	if v := q.Get("dry_run"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, "dry_run must be true or false"
		}
		f.DryRun = &b
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, "limit must be a positive number"
		}
		f.Limit = min(n, cleanupRunsMaxLimit)
	}
	if v := q.Get("cursor"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			return f, "cursor is not valid"
		}
		f.Cursor = n
	}
	return f, ""
}

// handleListCleanupRuns handles GET /api/v1/cleanup/runs (AbilityRead), as JSON pages or a CSV export.
func (rt *Router) handleListCleanupRuns(w http.ResponseWriter, r *http.Request) {
	if !rt.cleanupOr501(w) {
		return
	}
	f, msg := cleanupRunsFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format != "" && format != "json" && format != "csv" {
		writeError(w, http.StatusBadRequest, "format must be json or csv")
		return
	}
	if format == "csv" {
		f.Cursor = 0
		if r.URL.Query().Get("limit") == "" {
			f.Limit = envIntDefault(envCleanupExportMax, defaultCleanupExportMax)
		}
	}
	page, err := rt.cleanup.Runs(r.Context(), f)
	if err != nil {
		rt.internalError(w, "api: list cleanup runs", err)
		return
	}
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="cleanup-runs.csv"`)
		w.WriteHeader(http.StatusOK)
		if err := cleanup.WriteRunsCSV(w, page.Runs); err != nil {
			rt.logger.Warn("api: write cleanup runs csv failed")
		}
		return
	}
	out := cleanupRunsResponse{Runs: page.Runs}
	if page.NextCursor > 0 {
		out.NextCursor = strconv.FormatInt(page.NextCursor, 10)
	}
	writeJSON(w, http.StatusOK, out)
}

// cleanupPreviewReviewItem is the informational first-run item counting the data types still in preview mode.
func (rt *Router) cleanupPreviewReviewItem(ctx context.Context) (runtimesettings.Setting, bool) {
	if rt.cleanup == nil {
		return runtimesettings.Setting{}, false
	}
	n := rt.cleanup.PreviewCount(ctx)
	if n == 0 {
		return runtimesettings.Setting{}, false
	}
	return runtimesettings.Setting{
		Key: CleanupPreviewKey, Category: string(runtimesettings.CategoryRetention), Kind: string(runtimesettings.KindInt),
		Title:       "Cleanup is in preview mode",
		Description: "These data types are only measured, nothing is deleted until you enable them in Settings > Cleanup.",
		Default:     json.RawMessage("0"), Value: json.RawMessage(strconv.Itoa(n)), Source: runtimesettings.SourceDefault,
		Apply: string(runtimesettings.ApplyLive), Risk: string(runtimesettings.RiskNormal), Onboarding: true,
	}, true
}
