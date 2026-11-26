package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaudit"
	"github.com/GLINCKER/levelrail/internal/dbauthz"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DatabaseStatementAuditStore is the store surface the audit read route needs.
type DatabaseStatementAuditStore interface {
	ListDatabaseStatementAudits(ctx context.Context, f store.DatabaseStatementAuditFilter) ([]store.DatabaseStatementAudit, string, error)
}

// WithDatabaseAudit attaches the async audit writer; nil keeps recording off.
func (rt *Router) WithDatabaseAudit(w *dbaudit.Writer) *Router {
	rt.dbAudit = w
	return rt
}

type auditNoteKey struct{}

// auditNote lets a handler add what only it knows. It carries a fingerprint and
// counts, never raw values; Text is stored only when the live setting allows.
type auditNote struct {
	Fingerprint string
	Text        string
	Rows        int64
	Denied      bool
	// Database overrides the path name when a handler resolved the real root.
	Database string
}

// auditDatabaseNameRe mirrors the database name rule, so odd path values never reach a lookup.
var auditDatabaseNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// noteDenied marks the current explorer request as refused by the gate.
func noteDenied(r *http.Request) {
	if n, ok := r.Context().Value(auditNoteKey{}).(*auditNote); ok {
		n.Denied = true
	}
}

// noteStatement records the statement identity for the current explorer request.
func noteStatement(r *http.Request, fingerprint, text string, rows int) {
	if n, ok := r.Context().Value(auditNoteKey{}).(*auditNote); ok {
		n.Fingerprint, n.Text, n.Rows = fingerprint, text, int64(rows)
	}
}

// auditWriter captures status and size for the audit row.
type auditWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (a *auditWriter) WriteHeader(code int) {
	if a.status == 0 {
		a.status = code
	}
	a.ResponseWriter.WriteHeader(code)
}

func (a *auditWriter) Write(b []byte) (int, error) {
	if a.status == 0 {
		a.status = http.StatusOK
	}
	n, err := a.ResponseWriter.Write(b)
	a.bytes += int64(n)
	return n, err
}

func (a *auditWriter) Flush() {
	if f, ok := a.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (a *auditWriter) Unwrap() http.ResponseWriter { return a.ResponseWriter }

// withDatabaseAudit records one event per explorer request, denied ones included.
// It runs outside the gate so a refusal is audited too.
func (rt *Router) withDatabaseAudit(ability string, action dbauthz.Action, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt.dbAudit == nil {
			next(w, r)
			return
		}
		// Writes and admin actions stop when the trail cannot take them; reads fail open and count a drop.
		if (action == dbauthz.ActionWrite || action == dbauthz.ActionAdmin) && !rt.dbAudit.HasRoom(r.Context()) {
			writeCodedError(w, http.StatusServiceUnavailable, "audit_unavailable", "the audit trail is full, try again shortly")
			return
		}
		note := &auditNote{}
		aw := &auditWriter{ResponseWriter: w}
		start := time.Now()
		next(aw, r.WithContext(context.WithValue(r.Context(), auditNoteKey{}, note)))
		rt.recordExplorerAudit(r, aw, note, ability, action, time.Since(start))
	}
}

func (rt *Router) recordExplorerAudit(r *http.Request, aw *auditWriter, n *auditNote, ability string, action dbauthz.Action, d time.Duration) {
	status := aw.status
	if status == 0 {
		status = http.StatusOK
	}
	ctx := context.WithoutCancel(r.Context())
	ev := dbaudit.Event{
		At: time.Now(), DatabaseName: auditPathName(r, n), Ability: ability, Action: string(action),
		OpKind: auditOpKind(r.Pattern), Fingerprint: n.Fingerprint, StatementText: n.Text, RowsReturned: n.Rows,
		Bytes: aw.bytes, DurationMs: d.Milliseconds(), Outcome: auditOutcome(r, status), ClientIP: clientIP(r), BranchName: auditSafeName(r.PathValue("branch")),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), RequestID: aw.Header().Get(requestIDHeader),
	}
	if n.Denied {
		ev.Outcome = dbaudit.OutcomeDenied
	}
	if ev.Outcome != dbaudit.OutcomeOK {
		ev.ErrorClass = "http_" + strconv.Itoa(status)
	}
	if ev.Fingerprint == "" {
		ev.Fingerprint = routeFingerprint(r)
	}
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		// No principal means a flood of unauthenticated requests could fill the trail; count, do not store.
		rt.dbAudit.CountUnauthenticated()
		return
	}
	ev.PrincipalType, ev.PrincipalID = pt, pid
	actor := "session"
	if pt == store.PrincipalTypeToken {
		actor = "token"
	}
	ev.PrincipalName = rt.auditActorName(ctx, actor, pid, pid)
	if rt.databases != nil && auditDatabaseNameRe.MatchString(ev.DatabaseName) {
		if desired, err := rt.databases.GetDesiredDatabase(ctx, ev.DatabaseName); err == nil && desired != nil {
			ev.Engine, ev.DatabaseID = desired.Engine, desired.InstanceID
		}
	}
	rt.dbAudit.Record(ctx, ev)
}

// auditOutcome maps the response to the audit vocabulary; a gate refusal is "denied".
func auditOutcome(r *http.Request, status int) string {
	switch {
	case status < 400:
		return dbaudit.OutcomeOK
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return dbaudit.OutcomeDenied
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return dbaudit.OutcomeTimeout
	case status == 499 || errors.Is(r.Context().Err(), context.Canceled):
		return dbaudit.OutcomeCancelled
	case status >= 500:
		return dbaudit.OutcomeError
	}
	return dbaudit.OutcomeRejected
}

// auditOpKind names the kind of operation from the route pattern.
func auditOpKind(pattern string) string {
	p := pattern
	switch {
	case strings.HasSuffix(p, "/rows"):
		return "rows"
	case strings.HasSuffix(p, "/structure"), strings.HasSuffix(p, "/schema"), strings.HasSuffix(p, "/mongo/collections"),
		strings.HasSuffix(p, "/explorer/capabilities"), strings.HasSuffix(p, "/engine-stats"):
		return "schema"
	case strings.HasSuffix(p, "/query"), strings.HasSuffix(p, "/query/write"):
		return "query"
	case strings.HasSuffix(p, "/explain"):
		return "explain"
	case strings.HasSuffix(p, "/mongo/find"):
		return "find"
	case strings.HasSuffix(p, "/keys"), strings.HasSuffix(p, "/key"):
		return "keys"
	}
	return "command"
}

// routeFingerprint hashes the route and the parameter names, never the values.
func routeFingerprint(r *http.Request) string {
	keys := make([]string, 0, len(r.URL.Query()))
	for k := range r.URL.Query() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(r.Pattern + "?" + strings.Join(keys, "&")))
	return hex.EncodeToString(sum[:8])
}

type databaseAuditListResponse struct {
	Items      []store.DatabaseStatementAudit `json:"items"`
	NextCursor string                         `json:"next_cursor,omitempty"`
}

// handleListDatabaseStatementAudit handles GET /api/v1/databases/{name}/audit.
// Filters: principal, action, outcome, since, until (RFC3339), limit, cursor.
func (rt *Router) handleListDatabaseStatementAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.DatabaseStatementAuditFilter{
		DatabaseName: r.PathValue("name"), PrincipalID: q.Get("principal"), Action: q.Get("action"),
		Outcome: q.Get("outcome"), Cursor: q.Get("cursor"),
	}
	for key, dst := range map[string]*string{"since": &f.Since, "until": &f.Until} {
		raw := q.Get(key)
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, key+" must be an RFC3339 time")
			return
		}
		*dst = t.UTC().Format(store.StatementAuditTimeLayout)
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		f.Limit = n
	}
	// Only rows of this incarnation of the name are readable, so a reused name starts a clean trail.
	if rt.databases != nil && auditDatabaseNameRe.MatchString(f.DatabaseName) {
		if desired, err := rt.databases.GetDesiredDatabase(r.Context(), f.DatabaseName); err == nil && desired != nil {
			f.DatabaseID, f.MatchDatabaseID = desired.InstanceID, true
		}
	}
	rows, next, err := rt.dbAuditStore.ListDatabaseStatementAudits(r.Context(), f)
	if err != nil {
		if errors.Is(err, store.ErrStatementAuditCursor) {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		rt.internalError(w, "api: list database statement audit failed", err)
		return
	}
	if rows == nil {
		rows = []store.DatabaseStatementAudit{}
	}
	writeJSON(w, http.StatusOK, databaseAuditListResponse{Items: rows, NextCursor: next})
}

// auditSafeName keeps only values shaped like a database name, so raw caller input never reaches the trail.
func auditSafeName(v string) string {
	if auditDatabaseNameRe.MatchString(v) {
		return v
	}
	return ""
}

// auditPathName is the database a request touched. Branch routes use the resolved root, or nothing
// when the handler did not get that far, never the raw path value.
func auditPathName(r *http.Request, n *auditNote) string {
	if r.PathValue("branch") != "" {
		return auditSafeName(n.Database)
	}
	return r.PathValue("name")
}
