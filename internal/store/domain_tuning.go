package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DomainTuning is one row of domain_tuning (migrations/3231). A zero field
// keeps the ingress default for that knob.
type DomainTuning struct {
	Domain                       string
	Preset                       string
	Compression                  bool
	Streaming                    bool
	WebSocket                    bool
	DialTimeoutSeconds           int
	ResponseHeaderTimeoutSeconds int
	StreamTimeoutSeconds         int
	StreamCloseDelaySeconds      int
	UpdatedAt                    time.Time
}

const domainTuningColumns = `domain, preset, compression, streaming, websocket, dial_timeout_seconds,
	response_header_timeout_seconds, stream_timeout_seconds, stream_close_delay_seconds, updated_at`

func scanDomainTuning(row rowScanner) (DomainTuning, error) {
	var t DomainTuning
	var updated string
	if err := row.Scan(&t.Domain, &t.Preset, &t.Compression, &t.Streaming, &t.WebSocket, &t.DialTimeoutSeconds,
		&t.ResponseHeaderTimeoutSeconds, &t.StreamTimeoutSeconds, &t.StreamCloseDelaySeconds, &updated); err != nil {
		return DomainTuning{}, err
	}
	if ts, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		t.UpdatedAt = ts
	}
	return t, nil
}

// GetDomainTuning returns domain's tuning. found is false when none is set.
func (db *DB) GetDomainTuning(ctx context.Context, domain string) (DomainTuning, bool, error) {
	t, err := scanDomainTuning(db.QueryRowContext(ctx, `SELECT `+domainTuningColumns+` FROM domain_tuning WHERE domain = ?`, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return DomainTuning{}, false, nil
	}
	if err != nil {
		return DomainTuning{}, false, fmt.Errorf("store: get domain tuning: %w", err)
	}
	return t, true, nil
}

// SetDomainTuning upserts t. Callers validate the fields first.
func (db *DB) SetDomainTuning(ctx context.Context, t DomainTuning) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_tuning (`+domainTuningColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET
			preset = excluded.preset, compression = excluded.compression, streaming = excluded.streaming,
			websocket = excluded.websocket, dial_timeout_seconds = excluded.dial_timeout_seconds,
			response_header_timeout_seconds = excluded.response_header_timeout_seconds,
			stream_timeout_seconds = excluded.stream_timeout_seconds,
			stream_close_delay_seconds = excluded.stream_close_delay_seconds, updated_at = excluded.updated_at
	`, t.Domain, t.Preset, t.Compression, t.Streaming, t.WebSocket, t.DialTimeoutSeconds, t.ResponseHeaderTimeoutSeconds,
		t.StreamTimeoutSeconds, t.StreamCloseDelaySeconds, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set domain tuning: %w", err)
	}
	return nil
}

// DeleteDomainTuning removes domain's tuning. Idempotent.
func (db *DB) DeleteDomainTuning(ctx context.Context, domain string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM domain_tuning WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("store: delete domain tuning: %w", err)
	}
	return nil
}

// ListDomainTuning returns every row ordered by domain.
func (db *DB) ListDomainTuning(ctx context.Context) ([]DomainTuning, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+domainTuningColumns+` FROM domain_tuning ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain tuning: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DomainTuning
	for rows.Next() {
		t, err := scanDomainTuning(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan domain tuning: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain tuning: %w", err)
	}
	return out, nil
}
