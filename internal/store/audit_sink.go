package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Audit sink kinds.
const (
	AuditSinkSyslog  = "syslog"
	AuditSinkWebhook = "webhook"
	AuditSinkS3      = "s3"
)

// Audit sink delivery states.
const (
	AuditSinkDeliveryDelivered = "delivered"
	AuditSinkDeliveryFailed    = "failed"
)

// ErrAuditSinkNotFound is returned when no sink has the given id.
var ErrAuditSinkNotFound = errors.New("store: audit sink not found")

// ErrAuditSinkNameTaken is returned when a sink name is already in use.
var ErrAuditSinkNameTaken = errors.New("store: audit sink name already in use")

// AuditSink is one streaming destination for the audit log. Config is the
// non-secret JSON settings; credentials live in internal/secrets.
type AuditSink struct {
	ID                  string
	Name                string
	Kind                string
	Enabled             bool
	Config              string
	CursorSeq           int64
	ConsecutiveFailures int
	NextAttemptAt       string
	LastError           string
	LastDeliveredAt     string
	CreatedAt           string
	UpdatedAt           string
}

// AuditSinkDelivery is one attempt to ship a batch of entries.
type AuditSinkDelivery struct {
	ID        string
	SinkID    string
	FromSeq   int64
	ToSeq     int64
	Entries   int
	State     string
	Attempt   int
	LastError string
	CreatedAt string
}

// AuditSinkSecretsKey is the internal/secrets serviceName a sink's
// credentials are stored under.
func AuditSinkSecretsKey(id string) string { return "audit-sink/" + id }

func newAuditSinkID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate %s id: %w", prefix, err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewAuditSinkID mints a sink id.
func NewAuditSinkID() (string, error) { return newAuditSinkID("asnk") }

// NewAuditSinkDeliveryID mints a delivery id.
func NewAuditSinkDeliveryID() (string, error) { return newAuditSinkID("asdl") }

const auditSinkColumns = `id, name, kind, enabled, config, cursor_seq, consecutive_failures, next_attempt_at, last_error, last_delivered_at, created_at, updated_at`

func scanAuditSink(s interface{ Scan(dest ...any) error }) (AuditSink, error) {
	var k AuditSink
	var enabled int
	if err := s.Scan(&k.ID, &k.Name, &k.Kind, &enabled, &k.Config, &k.CursorSeq, &k.ConsecutiveFailures, &k.NextAttemptAt, &k.LastError, &k.LastDeliveredAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
		return AuditSink{}, err
	}
	k.Enabled = enabled != 0
	return k, nil
}

func wrapAuditSinkWrite(err error, id string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed: audit_sinks.name") {
		return ErrAuditSinkNameTaken
	}
	return fmt.Errorf("store: write audit sink %q: %w", id, err)
}

func auditBoolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SaveAuditSink inserts a sink.
func (db *DB) SaveAuditSink(ctx context.Context, k AuditSink) error {
	_, err := db.ExecContext(ctx, `INSERT INTO audit_sinks (`+auditSinkColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Name, k.Kind, auditBoolInt(k.Enabled), k.Config, k.CursorSeq, k.ConsecutiveFailures, k.NextAttemptAt, k.LastError, k.LastDeliveredAt, k.CreatedAt, k.UpdatedAt)
	return wrapAuditSinkWrite(err, k.ID)
}

// UpdateAuditSink replaces the operator-editable fields of a sink.
func (db *DB) UpdateAuditSink(ctx context.Context, k AuditSink) error {
	res, err := db.ExecContext(ctx, `UPDATE audit_sinks SET name = ?, enabled = ?, config = ?, updated_at = ? WHERE id = ?`,
		k.Name, auditBoolInt(k.Enabled), k.Config, k.UpdatedAt, k.ID)
	if err != nil {
		return wrapAuditSinkWrite(err, k.ID)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAuditSinkNotFound
	}
	return nil
}

// RecordAuditSinkProgress stores the worker-owned delivery state of a sink.
func (db *DB) RecordAuditSinkProgress(ctx context.Context, k AuditSink) error {
	_, err := db.ExecContext(ctx, `UPDATE audit_sinks SET cursor_seq = ?, consecutive_failures = ?, next_attempt_at = ?, last_error = ?, last_delivered_at = ? WHERE id = ?`,
		k.CursorSeq, k.ConsecutiveFailures, k.NextAttemptAt, k.LastError, k.LastDeliveredAt, k.ID)
	if err != nil {
		return fmt.Errorf("store: record audit sink progress %q: %w", k.ID, err)
	}
	return nil
}

// GetAuditSink returns one sink.
func (db *DB) GetAuditSink(ctx context.Context, id string) (AuditSink, error) {
	k, err := scanAuditSink(db.QueryRowContext(ctx, `SELECT `+auditSinkColumns+` FROM audit_sinks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return AuditSink{}, ErrAuditSinkNotFound
	}
	if err != nil {
		return AuditSink{}, fmt.Errorf("store: get audit sink %q: %w", id, err)
	}
	return k, nil
}

// ListAuditSinks returns every sink, oldest first.
func (db *DB) ListAuditSinks(ctx context.Context) ([]AuditSink, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+auditSinkColumns+` FROM audit_sinks ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list audit sinks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AuditSink
	for rows.Next() {
		k, err := scanAuditSink(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan audit sink: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteAuditSink removes a sink and, by cascade, its delivery history.
func (db *DB) DeleteAuditSink(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM audit_sinks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete audit sink %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAuditSinkNotFound
	}
	return nil
}

// SaveAuditSinkDelivery appends one delivery attempt to the history.
func (db *DB) SaveAuditSinkDelivery(ctx context.Context, d AuditSinkDelivery) error {
	_, err := db.ExecContext(ctx, `INSERT INTO audit_sink_deliveries (id, sink_id, from_seq, to_seq, entries, state, attempt, last_error, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.SinkID, d.FromSeq, d.ToSeq, d.Entries, d.State, d.Attempt, d.LastError, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save audit sink delivery %q: %w", d.ID, err)
	}
	return nil
}

// ListAuditSinkDeliveries returns a sink's newest delivery attempts.
func (db *DB) ListAuditSinkDeliveries(ctx context.Context, sinkID string, limit int) ([]AuditSinkDelivery, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, sink_id, from_seq, to_seq, entries, state, attempt, last_error, created_at FROM audit_sink_deliveries WHERE sink_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, sinkID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list audit sink deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AuditSinkDelivery
	for rows.Next() {
		var d AuditSinkDelivery
		if err := rows.Scan(&d.ID, &d.SinkID, &d.FromSeq, &d.ToSeq, &d.Entries, &d.State, &d.Attempt, &d.LastError, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan audit sink delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteAuditSinkDeliveriesOlderThan prunes delivery history.
func (db *DB) DeleteAuditSinkDeliveriesOlderThan(ctx context.Context, cutoff string) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM audit_sink_deliveries WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: prune audit sink deliveries: %w", err)
	}
	return res.RowsAffected()
}
