package api

import (
	"context"
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// statusRecorder wraps http.ResponseWriter to capture the status code a
// handler eventually writes, so requireAbility's audit hook (auth.go)
// can record it after next returns. Both WriteHeader and Write are
// overridden: a handler that never calls WriteHeader explicitly (just
// Write) still implicitly sends 200, and that path has to be captured
// too, not just the explicit one. Flush delegates to the underlying
// writer when it supports http.Flusher, so a route later added at an
// audited ability tier can still stream SSE through this wrapper
// without losing that capability.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.wroteHeader {
		return
	}
	s.status = status
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// auditWorthy reports whether required is sensitive enough to record in
// the audit log: everything except AbilityRead. An audit log answers
// "who changed what," not a full read-access log, so a plain read stays
// unlogged; AbilityReadSensitive is included deliberately, since reading
// a private credential is not an ordinary read.
func auditWorthy(required string) bool {
	return required != AbilityRead
}

// auditActorName resolves the real display name behind an audited
// request's actor: tokenName (already known to every requireAbility
// caller, no extra lookup) for a token actor, or the session's own user
// record for a session actor. A lookup failure degrades to actorID
// rather than blocking the write: this runs after next(w, r) has
// already returned, so there's no request left to protect by failing
// loudly, the same best-effort contract recordAudit's own doc comment
// describes.
func (rt *Router) auditActorName(ctx context.Context, actorType, actorID, tokenName string) string {
	if actorType != "session" {
		return tokenName
	}
	user, err := rt.auth.GetUserByID(ctx, actorID)
	if err != nil {
		rt.logger.Warn("api: resolve audit actor name failed", slog.String("error", err.Error()), slog.String("user_id", actorID))
		return actorID
	}
	return user.DisplayName
}

// recordAudit writes one audit_log row for a completed request.
// Best-effort: log a warning on failure, never block or fail the
// request, the same contract TouchAPITokenLastUsed already established
// a few lines above every caller of this function in auth.go. By the
// time this runs, next(w, r) has already returned, so there is no
// request left to fail even if this one does.
func (rt *Router) recordAudit(ctx context.Context, r *http.Request, required, actorType, actorID, actorName string, status int) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: generate audit entry id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID:         id,
		ActorType:  actorType,
		ActorID:    actorID,
		ActorName:  actorName,
		Ability:    required,
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: status,
		RemoteAddr: clientIP(r),
		CreatedAt:  store.FormatAuditTime(time.Now()),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save audit entry failed", slog.String("error", err.Error()), slog.String("path", r.URL.Path))
	}
}

// auditLogEntryResource is GET /api/v1/audit-log's wire shape, mirroring
// store.AuditEntry field-for-field.
type auditLogEntryResource struct {
	ID         string `json:"id"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	ActorName  string `json:"actor_name"`
	Ability    string `json:"ability"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	RemoteAddr string `json:"remote_addr"`
	CreatedAt  string `json:"created_at"`
}

func toAuditLogEntryResource(e store.AuditEntry) auditLogEntryResource {
	return auditLogEntryResource{
		ID:         e.ID,
		ActorType:  e.ActorType,
		ActorID:    e.ActorID,
		ActorName:  e.ActorName,
		Ability:    e.Ability,
		Method:     e.Method,
		Path:       e.Path,
		StatusCode: e.StatusCode,
		RemoteAddr: e.RemoteAddr,
		CreatedAt:  e.CreatedAt,
	}
}

const (
	defaultAuditLogLimit = 50
	maxAuditLogLimit     = 200
)

// parseAuditLogQuery reads handleListAuditLog's shared ?limit/?before/
// ?path/?method params, writing the request's own 400 response and
// returning ok=false on the first invalid one so the handler can bail
// out without duplicating that response shape per format.
func parseAuditLogQuery(w http.ResponseWriter, r *http.Request) (limit int, before *time.Time, filter store.AuditEntryFilter, ok bool) {
	limit = defaultAuditLogLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return 0, nil, store.AuditEntryFilter{}, false
		}
		limit = n
	}
	if limit > maxAuditLogLimit {
		limit = maxAuditLogLimit
	}

	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC3339 timestamp")
			return 0, nil, store.AuditEntryFilter{}, false
		}
		before = &t
	}

	filter = store.AuditEntryFilter{
		Path:   r.URL.Query().Get("path"),
		Method: r.URL.Query().Get("method"),
	}
	return limit, before, filter, true
}

// handleListAuditLog handles GET /api/v1/audit-log: every recorded
// write/deploy/root-tier request, newest first, cursor-paginated by
// ?before (an RFC3339 timestamp) so a large table never needs offset
// pagination. ?limit defaults to defaultAuditLogLimit, capped at
// maxAuditLogLimit. ?path and ?method narrow to one resource's own
// trail (e.g. an app's config-change history), both exact match.
// ?format=csv returns the same rows as a CSV download instead of JSON,
// for compliance/record-keeping export; the row shape is identical to
// auditLogEntryResource, so it can never leak a field the JSON view
// doesn't already expose.
func (rt *Router) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	limit, before, filter, ok := parseAuditLogQuery(w, r)
	if !ok {
		return
	}

	entries, err := rt.auditLog.ListAuditEntries(r.Context(), limit, before, filter)
	if err != nil {
		rt.internalError(w, "api: list audit log failed", err)
		return
	}

	if r.URL.Query().Get("format") == "csv" {
		rt.writeAuditLogCSV(w, entries)
		return
	}

	out := make([]auditLogEntryResource, len(entries))
	for i, e := range entries {
		out[i] = toAuditLogEntryResource(e)
	}
	writeJSON(w, http.StatusOK, out)
}

// auditLogCSVHeader is the CSV export's column order, field-for-field
// with auditLogEntryResource's own JSON keys.
var auditLogCSVHeader = []string{
	"id", "actor_type", "actor_id", "actor_name", "ability",
	"method", "path", "status_code", "remote_addr", "created_at",
}

// writeAuditLogCSV writes entries as a CSV file attachment. limit
// already caps the row count (maxAuditLogLimit), so this writes directly
// to w through csv.Writer rather than buffering the whole body into a
// byte slice first; the status line is committed before any row is
// written, so a mid-write failure can only be logged, not turned into an
// error response.
func (rt *Router) writeAuditLogCSV(w http.ResponseWriter, entries []store.AuditEntry) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-log.csv"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	_ = cw.Write(auditLogCSVHeader)
	for _, e := range entries {
		_ = cw.Write([]string{
			e.ID, e.ActorType, e.ActorID, e.ActorName, e.Ability,
			e.Method, e.Path, strconv.Itoa(e.StatusCode), e.RemoteAddr, e.CreatedAt,
		})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		rt.logger.Warn("api: write audit log csv failed", slog.String("error", err.Error()))
	}
}
