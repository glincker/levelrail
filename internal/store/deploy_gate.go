package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DeploySafetySettings is one app's readiness gate and post-deploy watch
// overrides (migrations/0460_deploy_safety.sql). A nil field inherits the
// platform default.
type DeploySafetySettings struct {
	ServiceName      string
	SuccessThreshold *int
	StableWindow     *time.Duration
	DrainTimeout     *time.Duration
	WatchWindow      *time.Duration
	WatchMaxRestarts *int
	WatchErrorRate   *float64
	WatchMinRequests *int
	AutoRollback     *bool
	// WatchMode is off, notify or auto; it wins over AutoRollback.
	WatchMode         *string
	WatchLatencyRatio *float64
	WatchSoftRollback *bool
	UpdatedAt         time.Time
}

// GetDeploySafetySettings returns serviceName's overrides, or a value with
// every field nil when it has none.
func (db *DB) GetDeploySafetySettings(ctx context.Context, serviceName string) (DeploySafetySettings, error) {
	var (
		s                                   = DeploySafetySettings{ServiceName: serviceName}
		threshold, restarts, minReq, autoRB sql.NullInt64
		stableMS, drainMS, watchMS          sql.NullInt64
		errRate, latencyRatio               sql.NullFloat64
		watchMode                           sql.NullString
		softRB                              sql.NullInt64
		updatedAt                           string
	)
	err := db.QueryRowContext(ctx, `
		SELECT success_threshold, stable_window_ms, drain_timeout_ms, watch_window_ms,
		       watch_max_restarts, watch_error_rate, watch_min_requests, auto_rollback, updated_at,
		       watch_mode, watch_latency_ratio, watch_soft_rollback
		FROM deploy_safety_settings WHERE service_name = ?
	`, serviceName).Scan(&threshold, &stableMS, &drainMS, &watchMS, &restarts, &errRate, &minReq, &autoRB, &updatedAt,
		&watchMode, &latencyRatio, &softRB)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("store: get deploy safety settings for %q: %w", serviceName, err)
	}
	s.SuccessThreshold = nullIntPtr(threshold)
	s.StableWindow = nullMSPtr(stableMS)
	s.DrainTimeout = nullMSPtr(drainMS)
	s.WatchWindow = nullMSPtr(watchMS)
	s.WatchMaxRestarts = nullIntPtr(restarts)
	if errRate.Valid {
		v := errRate.Float64
		s.WatchErrorRate = &v
	}
	s.WatchMinRequests = nullIntPtr(minReq)
	if autoRB.Valid {
		v := autoRB.Int64 == 1
		s.AutoRollback = &v
	}
	if watchMode.Valid {
		v := watchMode.String
		s.WatchMode = &v
	}
	if latencyRatio.Valid {
		v := latencyRatio.Float64
		s.WatchLatencyRatio = &v
	}
	if softRB.Valid {
		v := softRB.Int64 == 1
		s.WatchSoftRollback = &v
	}
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// SaveDeploySafetySettings replaces s.ServiceName's overrides.
func (db *DB) SaveDeploySafetySettings(ctx context.Context, s DeploySafetySettings) error {
	var autoRB sql.NullInt64
	if s.AutoRollback != nil {
		autoRB = sql.NullInt64{Valid: true}
		if *s.AutoRollback {
			autoRB.Int64 = 1
		}
	}
	var errRate sql.NullFloat64
	if s.WatchErrorRate != nil {
		errRate = sql.NullFloat64{Float64: *s.WatchErrorRate, Valid: true}
	}
	var softRB sql.NullInt64
	if s.WatchSoftRollback != nil {
		softRB = sql.NullInt64{Valid: true}
		if *s.WatchSoftRollback {
			softRB.Int64 = 1
		}
	}
	var watchMode sql.NullString
	if s.WatchMode != nil {
		watchMode = sql.NullString{String: *s.WatchMode, Valid: true}
	}
	var latencyRatio sql.NullFloat64
	if s.WatchLatencyRatio != nil {
		latencyRatio = sql.NullFloat64{Float64: *s.WatchLatencyRatio, Valid: true}
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_safety_settings (service_name, success_threshold, stable_window_ms, drain_timeout_ms,
			watch_window_ms, watch_max_restarts, watch_error_rate, watch_min_requests, auto_rollback, updated_at,
			watch_mode, watch_latency_ratio, watch_soft_rollback)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name) DO UPDATE SET
			success_threshold = excluded.success_threshold, stable_window_ms = excluded.stable_window_ms,
			drain_timeout_ms = excluded.drain_timeout_ms, watch_window_ms = excluded.watch_window_ms,
			watch_max_restarts = excluded.watch_max_restarts, watch_error_rate = excluded.watch_error_rate,
			watch_min_requests = excluded.watch_min_requests, auto_rollback = excluded.auto_rollback,
			updated_at = excluded.updated_at, watch_mode = excluded.watch_mode,
			watch_latency_ratio = excluded.watch_latency_ratio, watch_soft_rollback = excluded.watch_soft_rollback
	`, s.ServiceName, intPtrNull(s.SuccessThreshold), msPtrNull(s.StableWindow), msPtrNull(s.DrainTimeout),
		msPtrNull(s.WatchWindow), intPtrNull(s.WatchMaxRestarts), errRate, intPtrNull(s.WatchMinRequests), autoRB,
		time.Now().UTC().Format(time.RFC3339Nano), watchMode, latencyRatio, softRB)
	if err != nil {
		return fmt.Errorf("store: save deploy safety settings for %q: %w", s.ServiceName, err)
	}
	return nil
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func nullMSPtr(v sql.NullInt64) *time.Duration {
	if !v.Valid {
		return nil
	}
	d := time.Duration(v.Int64) * time.Millisecond
	return &d
}

func intPtrNull(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func msPtrNull(p *time.Duration) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: p.Milliseconds(), Valid: true}
}

// DeployGateEvent is one step on a release's way to traffic or through its
// post-deploy watch.
type DeployGateEvent struct {
	ID              int64
	DeployAttemptID string
	ServiceName     string
	// Image resolves DeployAttemptID when it is empty; it is not stored.
	Image     string
	Phase     string
	Status    string
	Reason    string
	Message   string
	Container string
	At        time.Time
}

// RecordGateEvent stores e. With no DeployAttemptID it attaches to the
// newest succeeded or failed attempt that deployed (ServiceName, Image),
// and inserts nothing when there is none.
func (db *DB) RecordGateEvent(ctx context.Context, e DeployGateEvent) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	at := e.At.UTC().Format(time.RFC3339Nano)
	var err error
	if e.DeployAttemptID != "" {
		_, err = db.ExecContext(ctx, `
			INSERT INTO deploy_gate_events (deploy_attempt_id, service_name, phase, status, reason, message, container, at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, e.DeployAttemptID, e.ServiceName, e.Phase, e.Status, e.Reason, e.Message, e.Container, at)
	} else {
		_, err = db.ExecContext(ctx, `
			INSERT INTO deploy_gate_events (deploy_attempt_id, service_name, phase, status, reason, message, container, at)
			SELECT id, service_name, ?, ?, ?, ?, ?, ?
			FROM deploy_attempts
			WHERE service_name = ? AND image = ? AND status IN (?, ?)
			ORDER BY started_at DESC LIMIT 1
		`, e.Phase, e.Status, e.Reason, e.Message, e.Container, at,
			e.ServiceName, e.Image, DeployAttemptStatusSucceeded, DeployAttemptStatusFailed)
	}
	if err != nil {
		return fmt.Errorf("store: record gate event for %q: %w", e.ServiceName, err)
	}
	return nil
}

const gateEventColumns = "id, deploy_attempt_id, service_name, phase, status, reason, message, container, at"

func (db *DB) queryGateEvents(ctx context.Context, what, where string, args ...any) ([]DeployGateEvent, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+gateEventColumns+` FROM deploy_gate_events WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployGateEvent
	for rows.Next() {
		var (
			e  DeployGateEvent
			at string
		)
		if err := rows.Scan(&e.ID, &e.DeployAttemptID, &e.ServiceName, &e.Phase, &e.Status, &e.Reason, &e.Message, &e.Container, &at); err != nil {
			return nil, fmt.Errorf("store: %s: scan: %w", what, err)
		}
		e.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	return out, nil
}

// ListGateEvents returns attemptID's gate timeline in the order it happened.
func (db *DB) ListGateEvents(ctx context.Context, attemptID string) ([]DeployGateEvent, error) {
	return db.queryGateEvents(ctx, "list gate events", "deploy_attempt_id = ? ORDER BY at ASC, id ASC", attemptID)
}

// ListGateEventsSince returns every event of phase and status at or after
// since, oldest first.
func (db *DB) ListGateEventsSince(ctx context.Context, phase, status string, since time.Time) ([]DeployGateEvent, error) {
	return db.queryGateEvents(ctx, "list gate events since", "phase = ? AND status = ? AND at >= ? ORDER BY at ASC, id ASC",
		phase, status, since.UTC().Format(time.RFC3339Nano))
}

// MarkRolloutFailureReason stamps reason onto the newest succeeded or
// failed attempt that deployed (serviceName, image).
func (db *DB) MarkRolloutFailureReason(ctx context.Context, serviceName, image, reason string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET reason = ?
		WHERE id = (
			SELECT id FROM deploy_attempts
			WHERE service_name = ? AND image = ? AND status IN (?, ?)
			ORDER BY started_at DESC LIMIT 1
		)
	`, reason, serviceName, image, DeployAttemptStatusSucceeded, DeployAttemptStatusFailed)
	if err != nil {
		return fmt.Errorf("store: mark rollout failure reason for %q: %w", serviceName, err)
	}
	return nil
}

// FailDeployAttemptWithReason flips a succeeded attempt to failed after the
// fact, recording why. Reports whether it changed anything.
func (db *DB) FailDeployAttemptWithReason(ctx context.Context, id, reason, message string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, rollout_state = ?, reason = ?, error = ?, finished_at = COALESCE(finished_at, ?)
		WHERE id = ? AND status = ?
	`, DeployAttemptStatusFailed, RolloutStateFailed, reason, message, time.Now().UTC().Format(time.RFC3339Nano),
		id, DeployAttemptStatusSucceeded)
	if err != nil {
		return false, fmt.Errorf("store: fail deploy attempt %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: fail deploy attempt %q: rows affected: %w", id, err)
	}
	return n > 0, nil
}

// LatestDeployAttemptFor returns the newest succeeded attempt that
// deployed (serviceName, image), or nil when there is none.
func (db *DB) LatestDeployAttemptFor(ctx context.Context, serviceName, image string) (*DeployAttempt, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+deployAttemptColumns+` FROM deploy_attempts
		WHERE service_name = ? AND image = ? AND status = ?
		ORDER BY started_at DESC LIMIT 1
	`, serviceName, image, DeployAttemptStatusSucceeded)
	a, err := scanDeployAttempt(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: latest deploy attempt for %q: %w", serviceName, err)
	}
	return a, nil
}
