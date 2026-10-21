package alerting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Inbox defaults: warning and critical only, so a new principal sees no info chatter.
const (
	InboxDefaultMinSeverity = SeverityWarning
	InboxDefaultDigestHour  = 8
	inboxMaxLimit           = 200
	inboxDefaultLimit       = 50
	inboxMaxMarkIDs         = 500
)

// ErrInboxInvalid marks a preferences or request value the caller can fix.
var ErrInboxInvalid = errors.New("invalid inbox input")

// InboxPrefs are one principal's routing rules and digest settings.
type InboxPrefs struct {
	Principal       string
	MinSeverity     string
	MutedApps       []string
	DigestEnabled   bool
	DigestHour      int
	DigestChannelID string
	LastDigestAt    time.Time
}

// InboxItem is one history entry as seen by a principal.
type InboxItem struct {
	HistoryEntry
	Read bool
}

// InboxFilter narrows ListInbox.
type InboxFilter struct {
	UnreadOnly bool
	Archived   bool
	Before     *time.Time
	Limit      int
}

func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityInfo:
		return 1
	default:
		return 2
	}
}

// DefaultInboxPrefs is what a principal with no saved row gets.
func DefaultInboxPrefs(principal string) InboxPrefs {
	return InboxPrefs{Principal: principal, MinSeverity: InboxDefaultMinSeverity, DigestHour: InboxDefaultDigestHour}
}

// Validate rejects unknown severities, hours outside 0-23 and an enabled digest with no channel.
func (p InboxPrefs) Validate() error {
	if !validSeverity(p.MinSeverity) {
		return fmt.Errorf("%w: min_severity must be info, warning or critical", ErrInboxInvalid)
	}
	if p.DigestHour < 0 || p.DigestHour > 23 {
		return fmt.Errorf("%w: digest_hour must be 0 to 23", ErrInboxInvalid)
	}
	if p.DigestEnabled && p.DigestChannelID == "" {
		return fmt.Errorf("%w: digest_channel_id is required when the digest is enabled", ErrInboxInvalid)
	}
	return nil
}

// Visible reports whether routing rules let e into the principal's inbox.
func (p InboxPrefs) Visible(e HistoryEntry) bool {
	if severityRank(e.Severity) < severityRank(p.MinSeverity) {
		return false
	}
	for _, a := range p.MutedApps {
		if e.App != "" && a == e.App {
			return false
		}
	}
	return true
}

const inboxBase = `
	FROM alert_history h
	LEFT JOIN inbox_read_state s ON s.principal = ?
	LEFT JOIN inbox_read r ON r.principal = ? AND r.history_id = h.id
	WHERE h.outcome IN ('sent','quiet_hours','failed') AND h.event IN ('fired','resolved','flapping','group')
	AND EXISTS (SELECT 1 FROM inbox_archive a WHERE a.principal = ? AND a.item_id = h.id) = ?`

const inboxReadExpr = `(r.history_id IS NOT NULL OR (s.read_before IS NOT NULL AND h.at <= s.read_before))`

// GetInboxPrefs returns saved preferences or the defaults.
func (db *DB) GetInboxPrefs(ctx context.Context, principal string) (InboxPrefs, error) {
	p := DefaultInboxPrefs(principal)
	var muted, last string
	var digest int
	err := db.QueryRowContext(ctx, `SELECT min_severity, muted_apps, digest_enabled, digest_hour, digest_channel_id, last_digest_at FROM inbox_prefs WHERE principal = ?`, principal).
		Scan(&p.MinSeverity, &muted, &digest, &p.DigestHour, &p.DigestChannelID, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("alerting: get inbox prefs: %w", err)
	}
	p.DigestEnabled = digest != 0
	if muted != "" {
		p.MutedApps = strings.Split(muted, "\n")
	}
	if last != "" {
		p.LastDigestAt, _ = time.Parse(time.RFC3339Nano, last)
	}
	return p, nil
}

// SaveInboxPrefs validates and upserts preferences. LastDigestAt is not touched.
func (db *DB) SaveInboxPrefs(ctx context.Context, p InboxPrefs, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.DigestChannelID != "" {
		if _, err := db.GetNotificationChannel(ctx, p.DigestChannelID); err != nil {
			return fmt.Errorf("%w: digest channel not found", ErrInboxInvalid)
		}
	}
	digest := 0
	if p.DigestEnabled {
		digest = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO inbox_prefs (principal, min_severity, muted_apps, digest_enabled, digest_hour, digest_channel_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(principal) DO UPDATE SET min_severity = excluded.min_severity, muted_apps = excluded.muted_apps,
			digest_enabled = excluded.digest_enabled, digest_hour = excluded.digest_hour,
			digest_channel_id = excluded.digest_channel_id, updated_at = excluded.updated_at`,
		p.Principal, p.MinSeverity, strings.Join(p.MutedApps, "\n"), digest, p.DigestHour, p.DigestChannelID, fmtTime(now))
	if err != nil {
		return fmt.Errorf("alerting: save inbox prefs: %w", err)
	}
	return nil
}

// ListInbox returns the principal's items newest first, honoring its routing rules.
func (db *DB) ListInbox(ctx context.Context, principal string, f InboxFilter) ([]InboxItem, error) {
	prefs, err := db.GetInboxPrefs(ctx, principal)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > inboxMaxLimit {
		limit = inboxDefaultLimit
	}
	before := f.Before
	var out []InboxItem
	for len(out) < limit {
		q := `SELECT h.id, h.at, h.rule_id, h.rule_name, h.rule_kind, h.resource_id, h.app, h.node, h.severity, h.event, h.outcome, h.detail, h.silence_id, h.channel_id, h.error, h.cert_json, ` + inboxReadExpr + inboxBase
		archivedArg := 0
		if f.Archived {
			archivedArg = 1
		}
		args := []any{principal, principal, principal, archivedArg}
		if before != nil {
			q += ` AND h.at < ?`
			args = append(args, fmtTime(*before))
		}
		if f.UnreadOnly {
			q += ` AND NOT ` + inboxReadExpr
		}
		q += ` ORDER BY h.at DESC, h.id DESC LIMIT ?`
		args = append(args, limit*2)
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("alerting: list inbox: %w", err)
		}
		n := 0
		var last time.Time
		for rows.Next() {
			var (
				it       InboxItem
				at       string
				certJSON string
				read     int
			)
			if err := rows.Scan(&it.ID, &at, &it.RuleID, &it.RuleName, &it.RuleKind, &it.ResourceID, &it.App, &it.Node, &it.Severity,
				&it.Event, &it.Outcome, &it.Detail, &it.SilenceID, &it.ChannelID, &it.Error, &certJSON, &read); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("alerting: scan inbox row: %w", err)
			}
			if it.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("alerting: parse inbox time: %w", err)
			}
			it.Cert = decodeCertPayload(certJSON)
			it.Read = read != 0
			n++
			last = it.At
			if len(out) < limit && prefs.Visible(it.HistoryEntry) {
				out = append(out, it)
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, fmt.Errorf("alerting: iterate inbox rows: %w", err)
		}
		if n < limit*2 {
			break
		}
		before = &last
	}
	return out, nil
}

// InboxUnreadCount counts visible unread items, capped at the newest 200.
func (db *DB) InboxUnreadCount(ctx context.Context, principal string) (int, error) {
	items, err := db.ListInbox(ctx, principal, InboxFilter{UnreadOnly: true, Limit: inboxMaxLimit})
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

// MarkInboxRead marks the given history ids read for principal.
func (db *DB) MarkInboxRead(ctx context.Context, principal string, ids []string) error {
	if len(ids) == 0 || len(ids) > inboxMaxMarkIDs {
		return fmt.Errorf("%w: ids must hold 1 to %d entries", ErrInboxInvalid, inboxMaxMarkIDs)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("alerting: mark inbox read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inbox_read (principal, history_id) VALUES (?, ?)`, principal, id); err != nil {
			return fmt.Errorf("alerting: mark inbox read: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("alerting: mark inbox read: %w", err)
	}
	return nil
}

// MarkAllInboxRead moves the principal's read cursor to now and drops the per-item rows it covers.
func (db *DB) MarkAllInboxRead(ctx context.Context, principal string, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("alerting: mark all inbox read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO inbox_read_state (principal, read_before) VALUES (?, ?)
		ON CONFLICT(principal) DO UPDATE SET read_before = excluded.read_before`, principal, fmtTime(now)); err != nil {
		return fmt.Errorf("alerting: mark all inbox read: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM inbox_read WHERE principal = ?`, principal); err != nil {
		return fmt.Errorf("alerting: mark all inbox read: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("alerting: mark all inbox read: %w", err)
	}
	return nil
}

// InboxState is the explicit read and archived ids a principal holds, for items the server does not list itself.
type InboxState struct {
	ReadIDs     []string
	ArchivedIDs []string
}

// GetInboxState returns the principal's per-item read and archive rows.
func (db *DB) GetInboxState(ctx context.Context, principal string) (InboxState, error) {
	st := InboxState{ReadIDs: []string{}, ArchivedIDs: []string{}}
	for _, q := range []struct {
		sql string
		dst *[]string
	}{
		{`SELECT history_id FROM inbox_read WHERE principal = ? ORDER BY history_id LIMIT 2000`, &st.ReadIDs},
		{`SELECT item_id FROM inbox_archive WHERE principal = ? ORDER BY item_id LIMIT 2000`, &st.ArchivedIDs},
	} {
		rows, err := db.QueryContext(ctx, q.sql, principal)
		if err != nil {
			return st, fmt.Errorf("alerting: get inbox state: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return st, fmt.Errorf("alerting: scan inbox state: %w", err)
			}
			*q.dst = append(*q.dst, id)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return st, fmt.Errorf("alerting: iterate inbox state: %w", err)
		}
	}
	return st, nil
}

// ArchiveInbox archives the given item ids for principal, or restores them when archived is false.
func (db *DB) ArchiveInbox(ctx context.Context, principal string, ids []string, archived bool) error {
	if len(ids) == 0 || len(ids) > inboxMaxMarkIDs {
		return fmt.Errorf("%w: ids must hold 1 to %d entries", ErrInboxInvalid, inboxMaxMarkIDs)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("alerting: archive inbox: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := `DELETE FROM inbox_archive WHERE principal = ? AND item_id = ?`
	if archived {
		q = `INSERT OR IGNORE INTO inbox_archive (principal, item_id) VALUES (?, ?)`
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, q, principal, id); err != nil {
			return fmt.Errorf("alerting: archive inbox: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("alerting: archive inbox: %w", err)
	}
	return nil
}

// DigestDue lists principals whose digest is on, whose hour (UTC) has arrived and who got none in the last 20 hours.
func (db *DB) DigestDue(ctx context.Context, now time.Time) ([]InboxPrefs, error) {
	rows, err := db.QueryContext(ctx, `SELECT principal FROM inbox_prefs WHERE digest_enabled = 1 AND digest_hour <= ?`, now.UTC().Hour())
	if err != nil {
		return nil, fmt.Errorf("alerting: list digest due: %w", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("alerting: scan digest due: %w", err)
		}
		names = append(names, n)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, fmt.Errorf("alerting: iterate digest due: %w", err)
	}
	var out []InboxPrefs
	for _, n := range names {
		p, err := db.GetInboxPrefs(ctx, n)
		if err != nil {
			return nil, err
		}
		if p.LastDigestAt.IsZero() || now.Sub(p.LastDigestAt) >= 20*time.Hour {
			out = append(out, p)
		}
	}
	return out, nil
}

// MarkDigestSent records that a digest went out at now.
func (db *DB) MarkDigestSent(ctx context.Context, principal string, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE inbox_prefs SET last_digest_at = ? WHERE principal = ?`, fmtTime(now), principal); err != nil {
		return fmt.Errorf("alerting: mark digest sent: %w", err)
	}
	return nil
}
