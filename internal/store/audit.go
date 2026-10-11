package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// AuditEntry is one recorded request that passed internal/api's
// requireAbility at more than AbilityRead (migrations/0043): who did
// what, closing the gap docs/comparison.md's own "no audit log exists
// anywhere in the codebase" line calls out. CreatedAt is a pre-formatted
// RFC3339Nano string, not a time.Time, the same "caller controls the
// wire format" convention BackupHistory's own StartedAt/FinishedAt
// fields already establish in this package.
type AuditEntry struct {
	ID         string
	ActorType  string // "session", "token", or "system" (internal/ingress's SQLiteStorage, no request behind it)
	ActorID    string
	ActorName  string
	Ability    string
	Method     string
	Path       string
	StatusCode int
	RemoteAddr string
	CreatedAt  string
	ClientKind string // "cli", "dashboard", "mcp", "api" (migrations/0077, derived from User-Agent), or "system"
	// AgentName is the agent label of the token that made the request, empty
	// for a session or an unlabeled token. AgentClient is the self-reported
	// MCP client name and version.
	AgentName   string
	AgentClient string
	// Action is a stable event name such as AuditActionDeviceLoginApproved,
	// empty for plain request rows. Filters use it, never the path.
	Action string
}

// auditTimeLayout formats a CreatedAt value with a fixed 9-digit
// fractional second (0s, not 9s, in the layout). time.RFC3339Nano
// strips trailing zeros, which would make two entries created at
// different times produce different-length strings; ListAuditEntries'
// before-cursor filter is a plain string comparison
// (`created_at < ?`), which is only correct when every stored value has
// identical width.
const auditTimeLayout = "2006-01-02T15:04:05.000000000Z"

// FormatAuditTime renders t as this package's audit_log.created_at
// format. Exported so internal/api's requireAbility, the only writer of
// AuditEntry.CreatedAt, produces values in the exact format
// ListAuditEntries' own cursor comparison assumes.
func FormatAuditTime(t time.Time) string {
	return t.UTC().Format(auditTimeLayout)
}

// auditEntryIDPrefix mirrors NewDeployAttemptID's own "short, greppable
// tag on an otherwise-opaque random ID" convention.
const auditEntryIDPrefix = "aud_"

// NewAuditEntryID generates an opaque, URL-safe audit entry identifier,
// minted the same way NewDeployAttemptID mints its own (fixed-length
// crypto/rand bytes, base64 URL encoding, a short prefix).
func NewAuditEntryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate audit entry id: %w", err)
	}
	return auditEntryIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveAuditEntry inserts a new audit log row. Insert-only, like
// SaveDeployAttempt: an audit log has no update or delete path through
// the app, an entry ID is minted fresh by every caller, and a duplicate
// ID would only ever indicate a caller bug, left to fail on the primary
// key constraint rather than silently overwriting history.
func (db *DB) SaveAuditEntry(ctx context.Context, e AuditEntry) error {
	return insertAuditEntry(ctx, db, e)
}

type auditExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertAuditEntry(ctx context.Context, x auditExecer, e AuditEntry) error {
	_, err := x.ExecContext(ctx, `
		INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind, agent_name, agent_client, action)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.ActorType, e.ActorID, e.ActorName, e.Ability, e.Method, e.Path, e.StatusCode, e.RemoteAddr, e.CreatedAt, e.ClientKind, e.AgentName, e.AgentClient, e.Action)
	if err != nil {
		return fmt.Errorf("store: save audit entry %q: %w", e.ID, err)
	}
	return nil
}

// AuditEntryFilter narrows ListAuditEntries to a specific resource's own
// audit trail, e.g. an app's config-changing requests (Path set to that
// app's exact path, Method to "PUT"). A zero-value filter applies no
// narrowing, the same behavior ListAuditEntries always had.
type AuditEntryFilter struct {
	Path       string
	Method     string
	ClientKind string
	// Search is a case-insensitive substring matched across actor_name,
	// ability, method, path and remote_addr.
	Search string
	// FailedOnly restricts results to status_code >= 400.
	FailedOnly bool
	// AgentName restricts results to entries made by this agent label.
	AgentName string
	// Action matches an event name exactly, or every event of a family
	// when given without a dot ("device_login" matches "device_login.expired").
	Action string
	// PathLike keeps rows whose path matches any of these LIKE patterns
	// (escape literals with AuditLikeLiteral, '%' is the wildcard).
	PathLike []string
	// ActionPrefixes keeps rows whose action starts with any of these.
	ActionPrefixes []string
}

// AuditLikeLiteral escapes s for use inside an AuditEntryFilter.PathLike pattern.
func AuditLikeLiteral(s string) string { return auditLikeEscaper.Replace(s) }

// auditLikeEscaper escapes LIKE wildcards so Search is a literal substring.
var auditLikeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ListAuditEntries returns up to limit audit log rows, newest first.
// before, when non-nil, restricts the result to entries strictly older
// than that timestamp, so a caller pages backward through a large table
// by re-issuing this call with the last returned row's CreatedAt: cursor
// pagination, not offset pagination, the same reasoning this function's
// own doc comment in the task spec calls for (an OFFSET query degrades
// linearly as the table grows; this one doesn't). filter narrows by
// exact path and/or method match.
func (db *DB) ListAuditEntries(ctx context.Context, limit int, before *time.Time, filter AuditEntryFilter) ([]AuditEntry, error) {
	query := `
		SELECT id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind, agent_name, agent_client, action
		FROM audit_log
	`
	var (
		conditions []string
		args       []any
	)
	if before != nil {
		conditions = append(conditions, "created_at < ?")
		args = append(args, FormatAuditTime(*before))
	}
	if filter.Path != "" {
		conditions = append(conditions, "path = ?")
		args = append(args, filter.Path)
	}
	if filter.Method != "" {
		conditions = append(conditions, "method = ?")
		args = append(args, filter.Method)
	}
	if filter.ClientKind != "" {
		conditions = append(conditions, "client_kind = ?")
		args = append(args, filter.ClientKind)
	}
	if filter.Search != "" {
		pattern := "%" + auditLikeEscaper.Replace(filter.Search) + "%"
		conditions = append(conditions, `(actor_name LIKE ? ESCAPE '\' OR ability LIKE ? ESCAPE '\' OR method LIKE ? ESCAPE '\' OR path LIKE ? ESCAPE '\' OR remote_addr LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	if filter.FailedOnly {
		conditions = append(conditions, "status_code >= 400")
	}
	if filter.AgentName != "" {
		conditions = append(conditions, "agent_name = ?")
		args = append(args, filter.AgentName)
	}
	if filter.Action != "" {
		conditions = append(conditions, `(action = ? OR action LIKE ? ESCAPE '\')`)
		args = append(args, filter.Action, auditLikeEscaper.Replace(filter.Action)+".%")
	}
	conditions, args = appendAnyLike(conditions, args, "path", filter.PathLike, "")
	conditions, args = appendAnyLike(conditions, args, "action", escapeAll(filter.ActionPrefixes), "%")
	if len(conditions) > 0 {
		query += "WHERE " + strings.Join(conditions, " AND ") + "\n"
	}
	query += "ORDER BY created_at DESC\nLIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list audit entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Ability, &e.Method, &e.Path, &e.StatusCode, &e.RemoteAddr, &e.CreatedAt, &e.ClientKind, &e.AgentName, &e.AgentClient, &e.Action); err != nil {
			return nil, fmt.Errorf("store: scan audit entry row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate audit entry rows: %w", err)
	}
	return out, nil
}

// appendAnyLike adds one "(col LIKE p1 OR col LIKE p2 ...)" condition.
func appendAnyLike(conditions []string, args []any, col string, patterns []string, suffix string) ([]string, []any) {
	var ors []string
	for _, p := range patterns {
		if p == "" {
			continue
		}
		ors = append(ors, col+` LIKE ? ESCAPE '\'`)
		args = append(args, p+suffix)
	}
	if len(ors) > 0 {
		conditions = append(conditions, "("+strings.Join(ors, " OR ")+")")
	}
	return conditions, args
}

func escapeAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, auditLikeEscaper.Replace(s))
	}
	return out
}

// DeleteAuditEntriesOlderThan removes every audit_log row created strictly
// before cutoff, returning the number of rows removed. The retention sweep
// (internal/api's RunAuditLogSweeper) and the manual purge endpoint both call
// this with a cutoff derived from the operator-configured retention window,
// rather than each rolling its own delete query.
func (db *DB) DeleteAuditEntriesOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM audit_log WHERE created_at < ?`, FormatAuditTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("store: delete audit entries older than %s: %w", cutoff, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: count deleted audit entries: %w", err)
	}
	return n, nil
}
