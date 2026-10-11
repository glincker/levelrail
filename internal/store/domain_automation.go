package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Audit action names for DNS records Levelrail writes on an operator's behalf.
const (
	AuditActionDNSRecordCreated = "dns_record.created"
	AuditActionDNSRecordUpdated = "dns_record.updated"
	AuditActionDNSRecordDeleted = "dns_record.deleted"
)

// ErrAutomationRunNotFound is returned for an unknown run id.
var ErrAutomationRunNotFound = errors.New("store: domain automation run not found")

// GetDomainAutomationRaw returns the stored policy JSON, "" when never set.
func (db *DB) GetDomainAutomationRaw(ctx context.Context) (string, error) {
	var raw sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT domain_automation FROM ingress_settings WHERE id = 1`).Scan(&raw); err != nil {
		return "", fmt.Errorf("store: get domain automation: %w", err)
	}
	return raw.String, nil
}

// SetDomainAutomationRaw stores the policy JSON.
func (db *DB) SetDomainAutomationRaw(ctx context.Context, raw string) error {
	if _, err := db.ExecContext(ctx, `UPDATE ingress_settings SET domain_automation = ? WHERE id = 1`, raw); err != nil {
		return fmt.Errorf("store: set domain automation: %w", err)
	}
	return nil
}

// DomainAutomationRun is one recorded go-live run. Steps and Undo are JSON
// owned by internal/api.
type DomainAutomationRun struct {
	ID        string
	AppName   string
	Domain    string
	Result    string
	Steps     string
	Undo      string
	CreatedAt time.Time
	UndoneAt  *time.Time
}

// NewDomainAutomationRunID returns a fresh run id.
func NewDomainAutomationRunID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: run id: %w", err)
	}
	return "gl_" + hex.EncodeToString(b), nil
}

// SaveDomainAutomationRun records a run.
func (db *DB) SaveDomainAutomationRun(ctx context.Context, r DomainAutomationRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_automation_runs (id, app_name, domain, result, steps, undo, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.AppName, r.Domain, r.Result, r.Steps, r.Undo, r.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save domain automation run: %w", err)
	}
	return nil
}

func scanAutomationRun(sc interface{ Scan(...any) error }) (DomainAutomationRun, error) {
	var r DomainAutomationRun
	var created string
	var undone sql.NullString
	if err := sc.Scan(&r.ID, &r.AppName, &r.Domain, &r.Result, &r.Steps, &r.Undo, &created, &undone); err != nil {
		return DomainAutomationRun{}, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if undone.Valid {
		if t, err := time.Parse(time.RFC3339Nano, undone.String); err == nil {
			r.UndoneAt = &t
		}
	}
	return r, nil
}

const automationRunColumns = `id, app_name, domain, result, steps, undo, created_at, undone_at`

// GetDomainAutomationRun returns one run.
func (db *DB) GetDomainAutomationRun(ctx context.Context, id string) (DomainAutomationRun, error) {
	r, err := scanAutomationRun(db.QueryRowContext(ctx, `SELECT `+automationRunColumns+` FROM domain_automation_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DomainAutomationRun{}, ErrAutomationRunNotFound
	}
	if err != nil {
		return DomainAutomationRun{}, fmt.Errorf("store: get domain automation run: %w", err)
	}
	return r, nil
}

// ListDomainAutomationRuns returns the newest runs first, limited.
func (db *DB) ListDomainAutomationRuns(ctx context.Context, limit int) ([]DomainAutomationRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+automationRunColumns+` FROM domain_automation_runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list domain automation runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DomainAutomationRun
	for rows.Next() {
		r, err := scanAutomationRun(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan domain automation run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkDomainAutomationRunUndone stamps a run as undone.
func (db *DB) MarkDomainAutomationRunUndone(ctx context.Context, id string, at time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE domain_automation_runs SET undone_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("store: mark domain automation run undone: %w", err)
	}
	return nil
}
