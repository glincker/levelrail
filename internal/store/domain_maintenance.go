package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuditActionDomainMaintenanceEnded is the system audit action written when
// a domain's maintenance ends on its own at the linked window's end.
const AuditActionDomainMaintenanceEnded = "domain_maintenance.ended"

// DomainMaintenance is one domain's maintenance mode settings.
type DomainMaintenance struct {
	Domain            string
	Title             string
	Message           string
	RetryAfterSeconds int
	// BypassCIDRs are normalized CIDRs whose clients still reach the app.
	BypassCIDRs []string
	// BypassHealthCheck lets the app's readiness path through.
	BypassHealthCheck bool
	// WindowID links a scheduled maintenance window; its end ends this.
	WindowID string
	// EndsAt is the linked window's end, zero when open ended.
	EndsAt time.Time
	// StartedAt is when maintenance began, zero for rows from before it was recorded.
	StartedAt time.Time
	// AutoOffAt is the safety cut-off for open ended maintenance, zero when none.
	AutoOffAt time.Time
	// Ongoing is true when the linked window is an open ended placeholder.
	Ongoing bool
	// windowGone is true when WindowID names a window that no longer exists.
	windowGone bool
}

// ActiveAt reports whether m still applies at now. A row linked to a window
// that ended or was cancelled no longer does.
func (m DomainMaintenance) ActiveAt(now time.Time) bool {
	if !m.AutoOffAt.IsZero() && !now.Before(m.AutoOffAt) {
		return false
	}
	if m.WindowID == "" {
		return true
	}
	return !m.windowGone && (m.Ongoing || now.Before(m.EndsAt))
}

const domainMaintenanceSelect = `
	SELECT m.domain, m.title, m.message, m.retry_after_seconds, m.bypass_cidrs, m.bypass_health_check,
	       m.window_id, w.id, COALESCE(w.ends_at, ''), m.started_at, m.auto_off_at, COALESCE(w.ongoing, 0)
	FROM domain_maintenance m LEFT JOIN maintenance_windows w ON w.id = m.window_id AND m.window_id != ''`

func scanDomainMaintenance(row interface{ Scan(dest ...any) error }) (DomainMaintenance, error) {
	var (
		m                DomainMaintenance
		cidrs, endsAt    string
		started, autoOff string
		health, ongoing  int
		linkedWindowID   sql.NullString
	)
	if err := row.Scan(&m.Domain, &m.Title, &m.Message, &m.RetryAfterSeconds, &cidrs, &health,
		&m.WindowID, &linkedWindowID, &endsAt, &started, &autoOff, &ongoing); err != nil {
		return DomainMaintenance{}, err
	}
	m.BypassHealthCheck = health != 0
	m.StartedAt, _ = parseStatusTime(started)
	m.AutoOffAt, _ = parseStatusTime(autoOff)
	if cidrs != "" {
		m.BypassCIDRs = strings.Split(cidrs, ",")
	}
	if m.WindowID != "" {
		m.windowGone = !linkedWindowID.Valid
		m.Ongoing = ongoing != 0
		if !m.Ongoing {
			m.EndsAt, _ = parseStatusTime(endsAt)
		}
	}
	return m, nil
}

// GetDomainMaintenanceSettings returns domain's maintenance settings when it
// is in maintenance at now.
func (db *DB) GetDomainMaintenanceSettings(ctx context.Context, domain string, now time.Time) (DomainMaintenance, bool, error) {
	m, err := scanDomainMaintenance(db.QueryRowContext(ctx, domainMaintenanceSelect+` WHERE m.domain = ?`, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return DomainMaintenance{}, false, nil
	}
	if err != nil {
		return DomainMaintenance{}, false, fmt.Errorf("store: get domain maintenance: %w", err)
	}
	if !m.ActiveAt(now) {
		return DomainMaintenance{}, false, nil
	}
	return m, true, nil
}

// SaveDomainMaintenance enables maintenance mode for m.Domain with m's
// settings, replacing any earlier settings.
func (db *DB) SaveDomainMaintenance(ctx context.Context, m DomainMaintenance) error {
	return saveDomainMaintenance(ctx, db, m)
}

func saveDomainMaintenance(ctx context.Context, x interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, m DomainMaintenance) error {
	_, err := x.ExecContext(ctx, `
		INSERT INTO domain_maintenance (domain, title, message, retry_after_seconds, bypass_cidrs, bypass_health_check, window_id, started_at, auto_off_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET title = excluded.title, message = excluded.message,
			retry_after_seconds = excluded.retry_after_seconds, bypass_cidrs = excluded.bypass_cidrs,
			bypass_health_check = excluded.bypass_health_check, window_id = excluded.window_id,
			started_at = excluded.started_at, auto_off_at = excluded.auto_off_at
	`, m.Domain, m.Title, m.Message, m.RetryAfterSeconds, strings.Join(m.BypassCIDRs, ","), boolInt(m.BypassHealthCheck), m.WindowID,
		formatTimeOrEmpty(m.StartedAt), formatTimeOrEmpty(m.AutoOffAt))
	if err != nil {
		return fmt.Errorf("store: save domain maintenance: %w", err)
	}
	return nil
}

// SaveDomainMaintenanceAll saves every row in one transaction, all or none.
func (db *DB) SaveDomainMaintenanceAll(ctx context.Context, rows []DomainMaintenance) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin save domain maintenance: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range rows {
		if err := saveDomainMaintenance(ctx, tx, m); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit save domain maintenance: %w", err)
	}
	return nil
}

// listDomainMaintenance returns every stored row, ordered by domain.
func (db *DB) listDomainMaintenance(ctx context.Context) ([]DomainMaintenance, error) {
	rows, err := db.QueryContext(ctx, domainMaintenanceSelect+` ORDER BY m.domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain maintenance: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DomainMaintenance
	for rows.Next() {
		m, err := scanDomainMaintenance(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan domain maintenance row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain maintenance rows: %w", err)
	}
	return out, nil
}

// ListActiveDomainMaintenance returns the settings of every domain in
// maintenance at now, for the ingress to re-derive every pass.
func (db *DB) ListActiveDomainMaintenance(ctx context.Context, now time.Time) ([]DomainMaintenance, error) {
	all, err := db.listDomainMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, m := range all {
		if m.ActiveAt(now) {
			out = append(out, m)
		}
	}
	return out, nil
}

// ListExpiredDomainMaintenance returns rows whose linked window has ended or
// been cancelled by now.
func (db *DB) ListExpiredDomainMaintenance(ctx context.Context, now time.Time) ([]DomainMaintenance, error) {
	all, err := db.listDomainMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	var out []DomainMaintenance
	for _, m := range all {
		if !m.ActiveAt(now) {
			out = append(out, m)
		}
	}
	return out, nil
}

// GetDomainMaintenance reports whether domain is in maintenance right now.
func (db *DB) GetDomainMaintenance(ctx context.Context, domain string) (bool, error) {
	_, ok, err := db.GetDomainMaintenanceSettings(ctx, domain, time.Now())
	return ok, err
}

// SetDomainMaintenance enables maintenance mode for domain with the default
// page, keeping any settings already saved. Idempotent.
func (db *DB) SetDomainMaintenance(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO domain_maintenance (domain, started_at) VALUES (?, ?) ON CONFLICT (domain) DO NOTHING`,
		domain, formatTimeOrEmpty(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set domain maintenance: %w", err)
	}
	return nil
}

// DeleteDomainMaintenance disables maintenance mode for domain, if
// enabled. Idempotent: clearing a domain with none configured is not an
// error.
func (db *DB) DeleteDomainMaintenance(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_maintenance WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain maintenance: %w", err)
	}
	return nil
}

// ListDomainMaintenance returns every domain in maintenance right now,
// ordered by domain.
func (db *DB) ListDomainMaintenance(ctx context.Context) ([]string, error) {
	active, err := db.ListActiveDomainMaintenance(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(active))
	for _, m := range active {
		out = append(out, m.Domain)
	}
	return out, nil
}

func formatTimeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
