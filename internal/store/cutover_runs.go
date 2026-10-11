package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCutoverRunNotFound is returned for an unknown cutover run id.
var ErrCutoverRunNotFound = errors.New("store: cutover run not found")

// ErrCutoverRunActive means the app already has a run in flight.
var ErrCutoverRunActive = errors.New("store: a cutover run is already in progress for this app")

// CutoverRun is one stored cutover run. DomainsJSON, StepsJSON and PlanJSON
// are owned by internal/cutover.
type CutoverRun struct {
	ID          string
	SessionID   string
	SourceID    string
	AppName     string
	Mode        string
	State       string
	Method      string
	DNSWrite    bool
	WasRouted   bool
	AcceptWarn  bool
	Awaiting    string
	DomainsJSON string
	StepsJSON   string
	PlanJSON    string
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FinishedAt  time.Time
}

const cutoverRunColumns = `id, session_id, source_id, app_name, mode, state, method, dns_write, was_routed, accept_warn, awaiting,
	domains_json, steps_json, plan_json, error, created_at, updated_at, finished_at`

func scanCutoverRun(sc interface{ Scan(...any) error }) (CutoverRun, error) {
	var r CutoverRun
	var dns, routed, accept int
	var created, updated, finished string
	if err := sc.Scan(&r.ID, &r.SessionID, &r.SourceID, &r.AppName, &r.Mode, &r.State, &r.Method, &dns, &routed, &accept, &r.Awaiting,
		&r.DomainsJSON, &r.StepsJSON, &r.PlanJSON, &r.Error, &created, &updated, &finished); err != nil {
		return CutoverRun{}, err
	}
	r.DNSWrite, r.WasRouted, r.AcceptWarn = dns != 0, routed != 0, accept != 0
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	r.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
	return r, nil
}

func fmtCutoverTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func cutoverFlag(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateCutoverRun inserts a run; ErrCutoverRunActive when the app has one in flight.
func (db *DB) CreateCutoverRun(ctx context.Context, r CutoverRun) error {
	_, err := db.ExecContext(ctx, `INSERT INTO cutover_runs (`+cutoverRunColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.SourceID, r.AppName, r.Mode, r.State, r.Method, cutoverFlag(r.DNSWrite), cutoverFlag(r.WasRouted), cutoverFlag(r.AcceptWarn), r.Awaiting,
		r.DomainsJSON, r.StepsJSON, r.PlanJSON, r.Error, fmtCutoverTime(r.CreatedAt), fmtCutoverTime(r.UpdatedAt), fmtCutoverTime(r.FinishedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrCutoverRunActive
		}
		return fmt.Errorf("store: create cutover run %q: %w", r.ID, err)
	}
	return nil
}

// SaveCutoverRun replaces a run's mutable fields.
func (db *DB) SaveCutoverRun(ctx context.Context, r CutoverRun) error {
	res, err := db.ExecContext(ctx, `UPDATE cutover_runs SET state = ?, method = ?, awaiting = ?, domains_json = ?, steps_json = ?, plan_json = ?,
		error = ?, was_routed = ?, updated_at = ?, finished_at = ? WHERE id = ?`,
		r.State, r.Method, r.Awaiting, r.DomainsJSON, r.StepsJSON, r.PlanJSON, r.Error, cutoverFlag(r.WasRouted), fmtCutoverTime(r.UpdatedAt), fmtCutoverTime(r.FinishedAt), r.ID)
	if err != nil {
		return fmt.Errorf("store: save cutover run %q: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCutoverRunNotFound
	}
	return nil
}

// GetCutoverRun returns one run.
func (db *DB) GetCutoverRun(ctx context.Context, id string) (CutoverRun, error) {
	r, err := scanCutoverRun(db.QueryRowContext(ctx, `SELECT `+cutoverRunColumns+` FROM cutover_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return CutoverRun{}, ErrCutoverRunNotFound
	}
	if err != nil {
		return CutoverRun{}, fmt.Errorf("store: get cutover run %q: %w", id, err)
	}
	return r, nil
}

func (db *DB) queryCutoverRuns(ctx context.Context, query string, args ...any) ([]CutoverRun, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list cutover runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CutoverRun
	for rows.Next() {
		r, err := scanCutoverRun(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan cutover run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListCutoverRuns returns an app's runs, newest first.
func (db *DB) ListCutoverRuns(ctx context.Context, app string, limit int) ([]CutoverRun, error) {
	return db.queryCutoverRuns(ctx, `SELECT `+cutoverRunColumns+` FROM cutover_runs WHERE app_name = ? ORDER BY created_at DESC, id DESC LIMIT ?`, app, limit)
}

// ListCutoverRunsByState returns every run in one of states, oldest first.
func (db *DB) ListCutoverRunsByState(ctx context.Context, states ...string) ([]CutoverRun, error) {
	if len(states) == 0 {
		return nil, nil
	}
	args := make([]any, len(states))
	for i, s := range states {
		args[i] = s
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(states)), ",")
	return db.queryCutoverRuns(ctx, `SELECT `+cutoverRunColumns+` FROM cutover_runs WHERE state IN (`+marks+`) ORDER BY created_at`, args...)
}
