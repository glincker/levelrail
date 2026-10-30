package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuditActionTrafficExportSecretsChanged marks a write to a traffic export sink credential; its Detail names slots, never values.
const AuditActionTrafficExportSecretsChanged = "traffic_export.secrets_changed"

// TrafficExportSecretsKey is the internal/secrets service name holding one sink's credentials.
func TrafficExportSecretsKey(id string) string { return "traffic-export/" + id }

// TrafficExportSaltKey is the secrets service and key of the visitor id salt.
const (
	TrafficExportSaltService = "traffic-export"
	TrafficExportSaltKey     = "visitor_salt"
)

// Sink kinds a traffic export sink can have.
const (
	TrafficSinkPostHog = "posthog"
	TrafficSinkOTLP    = "otlp"
	TrafficSinkWebhook = "webhook"
)

// ErrTrafficExportSinkNotFound is returned when no sink matches.
var ErrTrafficExportSinkNotFound = errors.New("store: traffic export sink not found")

// ErrTrafficExportSinkInvalid wraps validation failures such as a duplicate name.
var ErrTrafficExportSinkInvalid = errors.New("store: invalid traffic export sink")

// TrafficExportSink is one stored sink. No credential fields: those live in internal/secrets.
type TrafficExportSink struct {
	ID         string
	Name       string
	Kind       string
	Enabled    bool
	Host       string
	TargetHost string
	Fields     []string
	IPTruncate bool
	Sample     float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const trafficExportSinkColumns = `id, name, kind, enabled, host, target_host, fields, ip_truncate, sample, created_at, updated_at`

type trafficExportScanner interface{ Scan(dest ...any) error }

func scanTrafficExportSink(r trafficExportScanner) (TrafficExportSink, error) {
	var s TrafficExportSink
	var enabled, trunc int
	var fields, created, updated string
	if err := r.Scan(&s.ID, &s.Name, &s.Kind, &enabled, &s.Host, &s.TargetHost, &fields, &trunc, &s.Sample, &created, &updated); err != nil {
		return TrafficExportSink{}, err
	}
	s.Enabled, s.IPTruncate = enabled != 0, trunc != 0
	if err := json.Unmarshal([]byte(fields), &s.Fields); err != nil {
		return TrafficExportSink{}, fmt.Errorf("decode fields: %w", err)
	}
	s.CreatedAt, s.UpdatedAt = parseProviderTime(created), parseProviderTime(updated)
	return s, nil
}

// ListTrafficExportSinks returns every sink ordered by name.
func (db *DB) ListTrafficExportSinks(ctx context.Context) ([]TrafficExportSink, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+trafficExportSinkColumns+` FROM traffic_export_sinks ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list traffic export sinks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TrafficExportSink
	for rows.Next() {
		s, err := scanTrafficExportSink(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan traffic export sink: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list traffic export sinks: %w", err)
	}
	return out, nil
}

// GetTrafficExportSink returns one sink or ErrTrafficExportSinkNotFound.
func (db *DB) GetTrafficExportSink(ctx context.Context, id string) (TrafficExportSink, error) {
	s, err := scanTrafficExportSink(db.QueryRowContext(ctx, `SELECT `+trafficExportSinkColumns+` FROM traffic_export_sinks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficExportSink{}, ErrTrafficExportSinkNotFound
	}
	if err != nil {
		return TrafficExportSink{}, fmt.Errorf("store: get traffic export sink: %w", err)
	}
	return s, nil
}

// UpsertTrafficExportSink inserts or replaces a sink in full.
func (db *DB) UpsertTrafficExportSink(ctx context.Context, s TrafficExportSink) error {
	if s.ID == "" || strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("%w: id and name are required", ErrTrafficExportSinkInvalid)
	}
	fields := s.Fields
	if fields == nil {
		fields = []string{}
	}
	fb, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("store: encode traffic export fields: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	created := now
	if !s.CreatedAt.IsZero() {
		created = formatProviderTime(s.CreatedAt)
	}
	enabled, trunc := 0, 0
	if s.Enabled {
		enabled = 1
	}
	if s.IPTruncate {
		trunc = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO traffic_export_sinks (`+trafficExportSinkColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, kind = excluded.kind, enabled = excluded.enabled,
			host = excluded.host, target_host = excluded.target_host, fields = excluded.fields,
			ip_truncate = excluded.ip_truncate, sample = excluded.sample, updated_at = excluded.updated_at`,
		s.ID, s.Name, s.Kind, enabled, s.Host, s.TargetHost, string(fb), trunc, s.Sample, created, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("%w: a sink named %q already exists", ErrTrafficExportSinkInvalid, s.Name)
		}
		return fmt.Errorf("store: upsert traffic export sink: %w", err)
	}
	return nil
}

// DeleteTrafficExportSink removes a sink or returns ErrTrafficExportSinkNotFound.
func (db *DB) DeleteTrafficExportSink(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM traffic_export_sinks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete traffic export sink: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTrafficExportSinkNotFound
	}
	return nil
}
