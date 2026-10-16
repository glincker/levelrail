package store

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Sleep event kinds and reasons recorded in sleep_events (migrations/0790).
const (
	SleepEventSlept      = "slept"
	SleepEventWoke       = "woke"
	SleepEventEnabled    = "enabled"
	SleepEventDisabled   = "disabled"
	SleepEventWakeFailed = "wake_failed"

	SleepReasonIdleTimeout = "idle_timeout"
	SleepReasonManual      = "manual"
	SleepReasonRequest     = "request"
	SleepReasonSchedule    = "schedule"
	SleepReasonDeploy      = "deploy"
	SleepReasonAPI         = "api"
	SleepReasonPort        = "port"

	EnvSleepEventsKeep     = "APP_SLEEP_EVENTS_KEEP"
	defaultSleepEventsKeep = 500
	sleepEventListLimit    = 100
	sleepEventMaxLimit     = 1000
)

// SleepEvent is one entry of an app's sleep history. DurationSeconds is how
// long the app had slept (woke rows), WakeLatencyMs how long a wake took to
// become healthy; both are nil when unknown. TriggerSource is a coarse label,
// never a client address.
type SleepEvent struct {
	ID              int64
	ServiceName     string
	Kind            string
	At              time.Time
	Reason          string
	IdleMinutes     int
	TriggerPath     string
	TriggerSource   string
	Detail          string
	DurationSeconds *int64
	WakeLatencyMs   *int64
	Actor           string
}

func sleepEventsKeep() int {
	if n, err := strconv.Atoi(os.Getenv(EnvSleepEventsKeep)); err == nil && n > 0 {
		return n
	}
	return defaultSleepEventsKeep
}

// AddSleepEvent stores e and trims the app's history to the retention bound.
// It returns the new row's ID.
func (db *DB) AddSleepEvent(ctx context.Context, e SleepEvent) (int64, error) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if e.Actor == "" {
		e.Actor = "system"
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO sleep_events (service_name, kind, at, reason, idle_minutes, trigger_path, trigger_source, detail, duration_seconds, wake_latency_ms, actor)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ServiceName, e.Kind, e.At.UTC().Format(appEventTimeFormat), e.Reason, e.IdleMinutes, e.TriggerPath, e.TriggerSource, e.Detail, e.DurationSeconds, e.WakeLatencyMs, e.Actor)
	if err != nil {
		return 0, fmt.Errorf("store: add sleep event for %q: %w", e.ServiceName, err)
	}
	id, _ := res.LastInsertId()
	_, err = db.ExecContext(ctx, `
		DELETE FROM sleep_events WHERE service_name = ? AND id NOT IN (
			SELECT id FROM sleep_events WHERE service_name = ? ORDER BY at DESC, id DESC LIMIT ?)
	`, e.ServiceName, e.ServiceName, sleepEventsKeep())
	if err != nil {
		return id, fmt.Errorf("store: trim sleep events for %q: %w", e.ServiceName, err)
	}
	return id, nil
}

// SetSleepEventLatency records how long a wake took to become healthy.
func (db *DB) SetSleepEventLatency(ctx context.Context, id int64, latencyMs int64) error {
	if _, err := db.ExecContext(ctx, `UPDATE sleep_events SET wake_latency_ms = ? WHERE id = ?`, latencyMs, id); err != nil {
		return fmt.Errorf("store: set sleep event latency: %w", err)
	}
	return nil
}

// ListSleepEvents returns name's events newest first, at or after since when
// non-zero. limit is clamped to a sane page.
func (db *DB) ListSleepEvents(ctx context.Context, name string, since time.Time, limit int) ([]SleepEvent, error) {
	return db.querySleepEvents(ctx, name, since, limit, true)
}

// ListSleepEventsAsc is ListSleepEvents oldest first, for interval math.
func (db *DB) ListSleepEventsAsc(ctx context.Context, name string, since time.Time) ([]SleepEvent, error) {
	return db.querySleepEvents(ctx, name, since, sleepEventMaxLimit*10, false)
}

func (db *DB) querySleepEvents(ctx context.Context, name string, since time.Time, limit int, desc bool) ([]SleepEvent, error) {
	if limit <= 0 {
		limit = sleepEventListLimit
	}
	q := `SELECT id, service_name, kind, at, reason, idle_minutes, trigger_path, trigger_source, detail, duration_seconds, wake_latency_ms, actor FROM sleep_events WHERE service_name = ?`
	args := []any{name}
	if !since.IsZero() {
		q += ` AND at >= ?`
		args = append(args, since.UTC().Format(appEventTimeFormat))
	}
	if desc {
		q += ` ORDER BY at DESC, id DESC LIMIT ?`
	} else {
		q += ` ORDER BY at ASC, id ASC LIMIT ?`
	}
	args = append(args, min(limit, sleepEventMaxLimit*10))
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list sleep events for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var out []SleepEvent
	for rows.Next() {
		var e SleepEvent
		var at string
		if err := rows.Scan(&e.ID, &e.ServiceName, &e.Kind, &at, &e.Reason, &e.IdleMinutes, &e.TriggerPath, &e.TriggerSource, &e.Detail, &e.DurationSeconds, &e.WakeLatencyMs, &e.Actor); err != nil {
			return nil, fmt.Errorf("store: scan sleep event: %w", err)
		}
		if e.At, err = time.Parse(appEventTimeFormat, at); err != nil {
			return nil, fmt.Errorf("store: parse sleep event time: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LastSleepStateEventBefore returns the newest slept or woke event strictly
// before t, or nil. It tells whether an app was already asleep at a window start.
func (db *DB) LastSleepStateEventBefore(ctx context.Context, name string, t time.Time) (*SleepEvent, error) {
	rows, err := db.QueryContext(ctx, `SELECT kind, at FROM sleep_events WHERE service_name = ? AND kind IN ('slept','woke') AND at < ? ORDER BY at DESC, id DESC LIMIT 1`,
		name, t.UTC().Format(appEventTimeFormat))
	if err != nil {
		return nil, fmt.Errorf("store: last sleep state event for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var e SleepEvent
	var at string
	if err := rows.Scan(&e.Kind, &at); err != nil {
		return nil, fmt.Errorf("store: scan sleep state event: %w", err)
	}
	var perr error
	e.ServiceName = name
	e.At, perr = time.Parse(appEventTimeFormat, at)
	if perr != nil {
		return nil, fmt.Errorf("store: parse sleep state event time: %w", perr)
	}
	return &e, nil
}
