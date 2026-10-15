package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ExportKind selects which signal an export destination carries and which
// wire protocol it speaks.
type ExportKind string

// The destination kinds internal/dataexport can build a sender for.
const (
	ExportKindLogsHTTP     ExportKind = "logs_http"
	ExportKindLogsSyslog   ExportKind = "logs_syslog"
	ExportKindLogsLoki     ExportKind = "logs_loki"
	ExportKindAuditWebhook ExportKind = "audit_webhook"
	ExportKindAuditS3      ExportKind = "audit_s3"
	ExportKindMetricsOTLP  ExportKind = "metrics_otlp"
	ExportKindLogsOTLP     ExportKind = "logs_otlp"

	// Vendor destinations: the vendor's own ingest API with a key header.
	ExportKindLogsNewRelic    ExportKind = "logs_newrelic"
	ExportKindLogsDatadog     ExportKind = "logs_datadog"
	ExportKindLogsAxiom       ExportKind = "logs_axiom"
	ExportKindLogsBetterStack ExportKind = "logs_betterstack"
)

// ErrExportDestinationNotFound is returned when no destination has the given id.
var ErrExportDestinationNotFound = errors.New("store: export destination not found")

// ErrExportDestinationNameTaken is returned when a name is already in use.
var ErrExportDestinationNameTaken = errors.New("store: export destination name already in use")

// ExportOptions holds the non-secret, kind-specific settings.
type ExportOptions struct {
	// StorageTargetID names the storage destination an audit_s3 export writes to.
	StorageTargetID string `json:"storage_target_id,omitempty"`
	// Prefix is the object key prefix for audit_s3.
	Prefix string `json:"prefix,omitempty"`
	// Labels are static Loki stream labels.
	Labels map[string]string `json:"labels,omitempty"`
	// Region is us (default) or eu for New Relic and Axiom.
	Region string `json:"region,omitempty"`
	// Site is the Datadog site, for example datadoghq.eu.
	Site string `json:"site,omitempty"`
	// Dataset is the Axiom dataset.
	Dataset string `json:"dataset,omitempty"`
	// IntervalSeconds is the OTLP push interval.
	IntervalSeconds int `json:"interval_seconds,omitempty"`
	// Backfill makes an audit export start from the oldest row instead of now.
	Backfill bool `json:"backfill,omitempty"`
	// HeaderNames lists the secret header names so they can be shown without
	// resolving the secret.
	HeaderNames []string `json:"header_names,omitempty"`
	// HasHMACKey reports that a signing key is stored.
	HasHMACKey bool `json:"has_hmac_key,omitempty"`
}

// ExportDestination is one outbound export configuration.
type ExportDestination struct {
	ID        string
	Name      string
	Kind      ExportKind
	Endpoint  string
	Enabled   bool
	Apps      []string
	MinLevel  string
	Options   ExportOptions
	CreatedAt string
	UpdatedAt string
}

// ExportStatus is the persisted delivery state of one destination.
type ExportStatus struct {
	DestinationID string
	LastSuccessAt string
	LastError     string
	LastErrorAt   string
	SentTotal     int64
	DroppedTotal  int64
	FailuresTotal int64
	QueueDepth    int64
	CircuitOpen   bool
	CursorAt      string
	CursorID      string
}

// NewExportDestinationID mints a destination id.
func NewExportDestinationID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate export destination id: %w", err)
	}
	return "exp_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// ExportDestinationSecretsKey is the internal/secrets serviceName a
// destination's secret blob is stored under (envKey "config").
func ExportDestinationSecretsKey(id string) string { return "export-destination/" + id }

const exportDestinationColumns = `id, name, kind, endpoint, enabled, apps, min_level, options, created_at, updated_at`

type exportRowScanner interface{ Scan(dest ...any) error }

func scanExportDestination(s exportRowScanner) (ExportDestination, error) {
	var d ExportDestination
	var enabled int
	var apps, options string
	if err := s.Scan(&d.ID, &d.Name, &d.Kind, &d.Endpoint, &enabled, &apps, &d.MinLevel, &options, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return ExportDestination{}, err
	}
	d.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(apps), &d.Apps); err != nil {
		return ExportDestination{}, fmt.Errorf("decode apps: %w", err)
	}
	if err := json.Unmarshal([]byte(options), &d.Options); err != nil {
		return ExportDestination{}, fmt.Errorf("decode options: %w", err)
	}
	return d, nil
}

// SaveExportDestination inserts a destination.
func (db *DB) SaveExportDestination(ctx context.Context, d ExportDestination) error {
	apps, options, err := encodeExport(d)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO export_destinations (`+exportDestinationColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.Name, string(d.Kind), d.Endpoint, exportBoolInt(d.Enabled), apps, d.MinLevel, options, d.CreatedAt, d.UpdatedAt)
	return wrapExportWrite(err, d.ID)
}

// UpdateExportDestination replaces every mutable field of a destination.
func (db *DB) UpdateExportDestination(ctx context.Context, d ExportDestination) error {
	apps, options, err := encodeExport(d)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE export_destinations SET name = ?, endpoint = ?, enabled = ?, apps = ?, min_level = ?, options = ?, updated_at = ?
		WHERE id = ?
	`, d.Name, d.Endpoint, exportBoolInt(d.Enabled), apps, d.MinLevel, options, d.UpdatedAt, d.ID)
	if err != nil {
		return wrapExportWrite(err, d.ID)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExportDestinationNotFound
	}
	return nil
}

func encodeExport(d ExportDestination) (apps, options string, err error) {
	if d.Apps == nil {
		d.Apps = []string{}
	}
	a, err := json.Marshal(d.Apps)
	if err != nil {
		return "", "", fmt.Errorf("store: encode export apps: %w", err)
	}
	o, err := json.Marshal(d.Options)
	if err != nil {
		return "", "", fmt.Errorf("store: encode export options: %w", err)
	}
	return string(a), string(o), nil
}

func wrapExportWrite(err error, id string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed: export_destinations.name") {
		return ErrExportDestinationNameTaken
	}
	return fmt.Errorf("store: write export destination %q: %w", id, err)
}

func exportBoolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GetExportDestination returns one destination.
func (db *DB) GetExportDestination(ctx context.Context, id string) (ExportDestination, error) {
	d, err := scanExportDestination(db.QueryRowContext(ctx, `SELECT `+exportDestinationColumns+` FROM export_destinations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ExportDestination{}, ErrExportDestinationNotFound
	}
	if err != nil {
		return ExportDestination{}, fmt.Errorf("store: get export destination %q: %w", id, err)
	}
	return d, nil
}

// ListExportDestinations returns every destination ordered by name.
func (db *DB) ListExportDestinations(ctx context.Context) ([]ExportDestination, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+exportDestinationColumns+` FROM export_destinations ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list export destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ExportDestination
	for rows.Next() {
		d, err := scanExportDestination(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan export destination: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate export destinations: %w", err)
	}
	return out, nil
}

// DeleteExportDestination removes a destination and, by cascade, its status.
func (db *DB) DeleteExportDestination(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM export_destinations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete export destination %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExportDestinationNotFound
	}
	return nil
}

// SaveExportStatus upserts the delivery state of a destination.
func (db *DB) SaveExportStatus(ctx context.Context, s ExportStatus) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO export_destination_status
			(destination_id, last_success_at, last_error, last_error_at, sent_total, dropped_total, failures_total, queue_depth, circuit_open, cursor_at, cursor_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(destination_id) DO UPDATE SET
			last_success_at = excluded.last_success_at, last_error = excluded.last_error, last_error_at = excluded.last_error_at,
			sent_total = excluded.sent_total, dropped_total = excluded.dropped_total, failures_total = excluded.failures_total,
			queue_depth = excluded.queue_depth, circuit_open = excluded.circuit_open,
			cursor_at = excluded.cursor_at, cursor_id = excluded.cursor_id, updated_at = excluded.updated_at
	`, s.DestinationID, s.LastSuccessAt, s.LastError, s.LastErrorAt, s.SentTotal, s.DroppedTotal, s.FailuresTotal,
		s.QueueDepth, exportBoolInt(s.CircuitOpen), s.CursorAt, s.CursorID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save export status %q: %w", s.DestinationID, err)
	}
	return nil
}

// GetExportStatus returns a destination's state, or a zero value when none
// has been written yet.
func (db *DB) GetExportStatus(ctx context.Context, id string) (ExportStatus, error) {
	var s ExportStatus
	var open int
	err := db.QueryRowContext(ctx, `
		SELECT destination_id, last_success_at, last_error, last_error_at, sent_total, dropped_total, failures_total, queue_depth, circuit_open, cursor_at, cursor_id
		FROM export_destination_status WHERE destination_id = ?
	`, id).Scan(&s.DestinationID, &s.LastSuccessAt, &s.LastError, &s.LastErrorAt, &s.SentTotal, &s.DroppedTotal, &s.FailuresTotal, &s.QueueDepth, &open, &s.CursorAt, &s.CursorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ExportStatus{DestinationID: id}, nil
	}
	if err != nil {
		return ExportStatus{}, fmt.Errorf("store: get export status %q: %w", id, err)
	}
	s.CircuitOpen = open != 0
	return s, nil
}

// ListAuditEntriesAfter returns up to limit audit rows strictly after the
// (afterAt, afterID) cursor, oldest first, optionally bounded above by to.
// An empty afterAt starts from the oldest row. The (created_at, id) tuple
// order makes the cursor safe across rows sharing a timestamp.
func (db *DB) ListAuditEntriesAfter(ctx context.Context, afterAt, afterID string, to *time.Time, limit int) ([]AuditEntry, error) {
	query := `
		SELECT id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind, agent_name, agent_client, action, on_behalf_of
		FROM audit_log WHERE (created_at > ? OR (created_at = ? AND id > ?))`
	args := []any{afterAt, afterAt, afterID}
	if to != nil {
		query += ` AND created_at < ?`
		args = append(args, FormatAuditTime(*to))
	}
	query += ` ORDER BY created_at ASC, id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list audit entries after cursor: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Ability, &e.Method, &e.Path, &e.StatusCode, &e.RemoteAddr, &e.CreatedAt, &e.ClientKind, &e.AgentName, &e.AgentClient, &e.Action, &e.OnBehalfOf); err != nil {
			return nil, fmt.Errorf("store: scan audit entry row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate audit entry rows: %w", err)
	}
	return out, nil
}
