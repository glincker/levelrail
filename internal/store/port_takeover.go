package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrPortTakeoverNotFound is returned for an unknown takeover id.
var ErrPortTakeoverNotFound = errors.New("store: port takeover not found")

// PortTakeover is one stored takeover run. PreviousJSON, StepsJSON and
// VerifyJSON are owned by internal/takeover.
type PortTakeover struct {
	ID            string
	State         string
	Actor         string
	PreviousJSON  string
	TargetHTTP    string
	TargetHTTPS   string
	StepsJSON     string
	VerifyJSON    string
	Error         string
	Boots         int
	WindowSeconds int
	DeadlineAt    time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	FinishedAt    time.Time
}

const portTakeoverColumns = `id, state, actor, previous_json, target_http, target_https, steps_json, verify_json, error, boots,
	window_seconds, deadline_at, created_at, updated_at, finished_at`

func scanPortTakeover(sc interface{ Scan(...any) error }) (PortTakeover, error) {
	var r PortTakeover
	var deadline, created, updated, finished string
	if err := sc.Scan(&r.ID, &r.State, &r.Actor, &r.PreviousJSON, &r.TargetHTTP, &r.TargetHTTPS, &r.StepsJSON, &r.VerifyJSON, &r.Error,
		&r.Boots, &r.WindowSeconds, &deadline, &created, &updated, &finished); err != nil {
		return PortTakeover{}, err
	}
	r.DeadlineAt, _ = time.Parse(time.RFC3339Nano, deadline)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	r.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
	return r, nil
}

// CreatePortTakeover inserts a run.
func (db *DB) CreatePortTakeover(ctx context.Context, r PortTakeover) error {
	_, err := db.ExecContext(ctx, `INSERT INTO port_takeovers (`+portTakeoverColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.State, r.Actor, r.PreviousJSON, r.TargetHTTP, r.TargetHTTPS, r.StepsJSON, r.VerifyJSON, r.Error, r.Boots,
		r.WindowSeconds, fmtCutoverTime(r.DeadlineAt), fmtCutoverTime(r.CreatedAt), fmtCutoverTime(r.UpdatedAt), fmtCutoverTime(r.FinishedAt))
	if err != nil {
		return fmt.Errorf("store: create port takeover %q: %w", r.ID, err)
	}
	return nil
}

// SavePortTakeover replaces a run's mutable fields.
func (db *DB) SavePortTakeover(ctx context.Context, r PortTakeover) error {
	res, err := db.ExecContext(ctx, `UPDATE port_takeovers SET state = ?, previous_json = ?, steps_json = ?, verify_json = ?, error = ?, boots = ?,
		deadline_at = ?, updated_at = ?, finished_at = ? WHERE id = ?`,
		r.State, r.PreviousJSON, r.StepsJSON, r.VerifyJSON, r.Error, r.Boots, fmtCutoverTime(r.DeadlineAt), fmtCutoverTime(r.UpdatedAt), fmtCutoverTime(r.FinishedAt), r.ID)
	if err != nil {
		return fmt.Errorf("store: save port takeover %q: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPortTakeoverNotFound
	}
	return nil
}

// GetPortTakeover returns one run.
func (db *DB) GetPortTakeover(ctx context.Context, id string) (PortTakeover, error) {
	r, err := scanPortTakeover(db.QueryRowContext(ctx, `SELECT `+portTakeoverColumns+` FROM port_takeovers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return PortTakeover{}, ErrPortTakeoverNotFound
	}
	if err != nil {
		return PortTakeover{}, fmt.Errorf("store: get port takeover %q: %w", id, err)
	}
	return r, nil
}

func (db *DB) queryPortTakeovers(ctx context.Context, query string, args ...any) ([]PortTakeover, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list port takeovers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PortTakeover
	for rows.Next() {
		r, err := scanPortTakeover(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan port takeover: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListPortTakeovers returns runs newest first.
func (db *DB) ListPortTakeovers(ctx context.Context, limit int) ([]PortTakeover, error) {
	return db.queryPortTakeovers(ctx, `SELECT `+portTakeoverColumns+` FROM port_takeovers ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
}

// ListPortTakeoversByState returns runs in any of states, oldest first.
func (db *DB) ListPortTakeoversByState(ctx context.Context, states ...string) ([]PortTakeover, error) {
	if len(states) == 0 {
		return nil, nil
	}
	args := make([]any, len(states))
	for i, s := range states {
		args[i] = s
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(states)), ",")
	return db.queryPortTakeovers(ctx, `SELECT `+portTakeoverColumns+` FROM port_takeovers WHERE state IN (`+marks+`) ORDER BY created_at`, args...)
}
