package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Function invocation states. Dead is the dead-letter list.
const (
	FunctionInvocationQueued  = "queued"
	FunctionInvocationRunning = "running"
	FunctionInvocationDone    = "done"
	FunctionInvocationDead    = "dead"
)

// Errors of the function trigger store.
var (
	ErrFunctionWebhookNotFound     = errors.New("store: function webhook not found")
	ErrFunctionInvocationNotFound  = errors.New("store: function invocation not found")
	ErrFunctionInvocationDuplicate = errors.New("store: function invocation already received")
	ErrFunctionQueueFull           = errors.New("store: function queue is full")
	ErrFunctionInvocationNotDead   = errors.New("store: only a dead invocation can be retried")
)

// FunctionWebhook is the signed webhook trigger of one function. The secret lives in the secrets store.
type FunctionWebhook struct {
	Function  string
	Path      string
	CreatedAt time.Time
	RotatedAt *time.Time
}

// FunctionInvocation is one queued call to a function.
type FunctionInvocation struct {
	ID             string
	Function       string
	Source         string
	Method         string
	Path           string
	ContentType    string
	Body           []byte
	DedupeKey      string
	Status         string
	Attempts       int
	MaxAttempts    int
	NextAttemptAt  time.Time
	LastStatusCode int
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func fmtTS(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// SetFunctionWebhook upserts a function's webhook trigger; rotated marks a secret rotation.
func (db *DB) SetFunctionWebhook(ctx context.Context, fn, path string, rotated bool, now time.Time) error {
	rot := sql.NullString{}
	if rotated {
		rot = sql.NullString{String: fmtTS(now), Valid: true}
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO function_webhooks (function, path, created_at, rotated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(function) DO UPDATE SET path = excluded.path,
			rotated_at = COALESCE(excluded.rotated_at, function_webhooks.rotated_at)
	`, fn, path, fmtTS(now), rot)
	if err != nil {
		return fmt.Errorf("store: set function webhook %q: %w", fn, err)
	}
	return nil
}

// GetFunctionWebhook returns a function's webhook or ErrFunctionWebhookNotFound.
func (db *DB) GetFunctionWebhook(ctx context.Context, fn string) (FunctionWebhook, error) {
	w := FunctionWebhook{Function: fn}
	var created string
	var rotated sql.NullString
	err := db.QueryRowContext(ctx, `SELECT path, created_at, rotated_at FROM function_webhooks WHERE function = ?`, fn).Scan(&w.Path, &created, &rotated)
	if errors.Is(err, sql.ErrNoRows) {
		return FunctionWebhook{}, ErrFunctionWebhookNotFound
	}
	if err != nil {
		return FunctionWebhook{}, fmt.Errorf("store: get function webhook %q: %w", fn, err)
	}
	w.CreatedAt = parseTS(created)
	if rotated.Valid {
		t := parseTS(rotated.String)
		w.RotatedAt = &t
	}
	return w, nil
}

// DeleteFunctionWebhook removes a function's webhook; missing is not an error.
func (db *DB) DeleteFunctionWebhook(ctx context.Context, fn string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM function_webhooks WHERE function = ?`, fn); err != nil {
		return fmt.Errorf("store: delete function webhook %q: %w", fn, err)
	}
	return nil
}

// EnqueueFunctionInvocation stores a queued call. maxQueued <= 0 disables the per-function depth cap.
func (db *DB) EnqueueFunctionInvocation(ctx context.Context, inv FunctionInvocation, maxQueued int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: enqueue function invocation %q: %w", inv.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if maxQueued > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM function_invocations WHERE function = ? AND status IN (?, ?)`,
			inv.Function, FunctionInvocationQueued, FunctionInvocationRunning).Scan(&n); err != nil {
			return fmt.Errorf("store: count queued invocations for %q: %w", inv.Function, err)
		}
		if n >= maxQueued {
			return ErrFunctionQueueFull
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO function_invocations (id, function, source, method, path, content_type, body, dedupe_key, status,
			attempts, max_attempts, next_attempt_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)
	`, inv.ID, inv.Function, inv.Source, inv.Method, inv.Path, inv.ContentType, inv.Body, inv.DedupeKey,
		FunctionInvocationQueued, inv.MaxAttempts, fmtTS(inv.NextAttemptAt), fmtTS(inv.CreatedAt), fmtTS(inv.CreatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrFunctionInvocationDuplicate
		}
		return fmt.Errorf("store: enqueue function invocation %q: %w", inv.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: enqueue function invocation %q: %w", inv.ID, err)
	}
	return nil
}

const functionInvocationCols = `id, function, source, method, path, content_type, body, dedupe_key, status, attempts,
	max_attempts, next_attempt_at, last_status_code, last_error, created_at, updated_at`

type invocationScanner interface{ Scan(dest ...any) error }

func scanFunctionInvocation(r invocationScanner) (FunctionInvocation, error) {
	var inv FunctionInvocation
	var next, created, updated string
	err := r.Scan(&inv.ID, &inv.Function, &inv.Source, &inv.Method, &inv.Path, &inv.ContentType, &inv.Body, &inv.DedupeKey,
		&inv.Status, &inv.Attempts, &inv.MaxAttempts, &next, &inv.LastStatusCode, &inv.LastError, &created, &updated)
	inv.NextAttemptAt, inv.CreatedAt, inv.UpdatedAt = parseTS(next), parseTS(created), parseTS(updated)
	return inv, err
}

// ClaimFunctionInvocation marks the oldest due queued call running and counts the attempt.
func (db *DB) ClaimFunctionInvocation(ctx context.Context, now time.Time) (FunctionInvocation, bool, error) {
	row := db.QueryRowContext(ctx, `
		UPDATE function_invocations SET status = ?, attempts = attempts + 1, updated_at = ?
		WHERE id = (SELECT id FROM function_invocations WHERE status = ? AND next_attempt_at <= ? ORDER BY next_attempt_at LIMIT 1)
		RETURNING `+functionInvocationCols, FunctionInvocationRunning, fmtTS(now), FunctionInvocationQueued, fmtTS(now))
	inv, err := scanFunctionInvocation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return FunctionInvocation{}, false, nil
	}
	if err != nil {
		return FunctionInvocation{}, false, fmt.Errorf("store: claim function invocation: %w", err)
	}
	return inv, true, nil
}

// SettleFunctionInvocation records an attempt outcome: done, dead, or queued again at next.
func (db *DB) SettleFunctionInvocation(ctx context.Context, id, status string, next time.Time, code int, errMsg string, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE function_invocations SET status = ?, next_attempt_at = ?, last_status_code = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`, status, fmtTS(next), code, errMsg, fmtTS(now), id)
	if err != nil {
		return fmt.Errorf("store: settle function invocation %q: %w", id, err)
	}
	return nil
}

// RequeueRunningFunctionInvocations returns calls a crashed worker left running to the queue.
func (db *DB) RequeueRunningFunctionInvocations(ctx context.Context, now time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `UPDATE function_invocations SET status = ?, updated_at = ? WHERE status = ?`,
		FunctionInvocationQueued, fmtTS(now), FunctionInvocationRunning)
	if err != nil {
		return 0, fmt.Errorf("store: requeue running function invocations: %w", err)
	}
	return res.RowsAffected()
}

// GetFunctionInvocation returns one call or ErrFunctionInvocationNotFound.
func (db *DB) GetFunctionInvocation(ctx context.Context, id string) (FunctionInvocation, error) {
	inv, err := scanFunctionInvocation(db.QueryRowContext(ctx, `SELECT `+functionInvocationCols+` FROM function_invocations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return FunctionInvocation{}, ErrFunctionInvocationNotFound
	}
	if err != nil {
		return FunctionInvocation{}, fmt.Errorf("store: get function invocation %q: %w", id, err)
	}
	return inv, nil
}

// ListFunctionInvocations lists calls newest first; empty fn or status match any.
func (db *DB) ListFunctionInvocations(ctx context.Context, fn, status string, limit int) ([]FunctionInvocation, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+functionInvocationCols+` FROM function_invocations
		WHERE (? = '' OR function = ?) AND (? = '' OR status = ?) ORDER BY created_at DESC LIMIT ?`, fn, fn, status, status, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list function invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []FunctionInvocation{}
	for rows.Next() {
		inv, err := scanFunctionInvocation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan function invocation: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// CountFunctionInvocations returns call counts by state for one function.
func (db *DB) CountFunctionInvocations(ctx context.Context, fn string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT status, COUNT(*) FROM function_invocations WHERE function = ? GROUP BY status`, fn)
	if err != nil {
		return nil, fmt.Errorf("store: count function invocations %q: %w", fn, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, fmt.Errorf("store: scan function invocation count: %w", err)
		}
		out[s] = n
	}
	return out, rows.Err()
}

// RetryFunctionInvocation puts a dead call back in the queue with a fresh attempt budget.
func (db *DB) RetryFunctionInvocation(ctx context.Context, id string, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE function_invocations SET status = ?, attempts = 0, next_attempt_at = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`, FunctionInvocationQueued, fmtTS(now), fmtTS(now), id, FunctionInvocationDead)
	if err != nil {
		return fmt.Errorf("store: retry function invocation %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := db.GetFunctionInvocation(ctx, id); gerr != nil {
			return gerr
		}
		return ErrFunctionInvocationNotDead
	}
	return nil
}

// DeleteFunctionInvocation removes a call from the list.
func (db *DB) DeleteFunctionInvocation(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM function_invocations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete function invocation %q: %w", id, err)
	}
	return nil
}

// PruneFunctionInvocations drops finished calls last touched before cutoff.
func (db *DB) PruneFunctionInvocations(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM function_invocations WHERE status = ? AND updated_at < ?`, FunctionInvocationDone, fmtTS(cutoff))
	if err != nil {
		return 0, fmt.Errorf("store: prune function invocations: %w", err)
	}
	return res.RowsAffected()
}
