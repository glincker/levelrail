package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// App event kinds recorded in app_events. Deploys and rollbacks are not
// events: the timeline derives them from deploy attempts.
const (
	AppEventRestart        = "restart"
	AppEventEnvChange      = "env_change"
	AppEventSecretChange   = "secret_change"
	AppEventConfigChange   = "config_change"
	AppEventScale          = "scale"
	AppEventSuspend        = "suspend"
	AppEventResume         = "resume"
	AppEventFreezeOverride = "freeze_override"
)

const (
	appEventTimeFormat       = "2006-01-02T15:04:05.000000000Z"
	appEventIDPrefix         = "evt_"
	defaultAppEventListLimit = 50
)

// AppEvent is one recorded config or lifecycle change. Keys holds env or
// secret key names only, never values.
type AppEvent struct {
	ID        string
	AppName   string
	Kind      string
	Actor     string
	Title     string
	Detail    string
	Keys      []string
	CreatedAt time.Time
}

// AppEventCursor is the (time, id) position after which ListAppEvents resumes.
type AppEventCursor struct {
	At time.Time
	ID string
}

// NewAppEventID mints an opaque event ID.
func NewAppEventID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: mint app event id: %w", err)
	}
	return appEventIDPrefix + hex.EncodeToString(b[:]), nil
}

// AddAppEvent records e, minting its ID and timestamp when unset.
func (db *DB) AddAppEvent(ctx context.Context, e AppEvent) error {
	if e.ID == "" {
		id, err := NewAppEventID()
		if err != nil {
			return err
		}
		e.ID = id
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	keys := e.Keys
	if keys == nil {
		keys = []string{}
	}
	keysJSON, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("store: marshal app event keys: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO app_events (id, app_name, kind, actor, title, detail, keys, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.AppName, e.Kind, e.Actor, e.Title, e.Detail, string(keysJSON), e.CreatedAt.UTC().Format(appEventTimeFormat))
	if err != nil {
		return fmt.Errorf("store: add app event for %q: %w", e.AppName, err)
	}
	return nil
}

// ListAppEvents returns name's events newest first, strictly older than
// before when non-nil, restricted to kinds when any are given. after, when
// non-zero, keeps only events at or after that time.
func (db *DB) ListAppEvents(ctx context.Context, name string, before *AppEventCursor, after time.Time, kinds []string, limit int) ([]AppEvent, error) {
	if limit <= 0 {
		limit = defaultAppEventListLimit
	}
	query := `SELECT id, app_name, kind, actor, title, detail, keys, created_at FROM app_events WHERE app_name = ?`
	args := []any{name}
	if before != nil {
		ts := before.At.UTC().Format(appEventTimeFormat)
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, ts, ts, before.ID)
	}
	if !after.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, after.UTC().Format(appEventTimeFormat))
	}
	if len(kinds) > 0 {
		query += ` AND kind IN (?` + strings.Repeat(", ?", len(kinds)-1) + `)`
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list app events for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppEvent
	for rows.Next() {
		var (
			e         AppEvent
			keysJSON  string
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.AppName, &e.Kind, &e.Actor, &e.Title, &e.Detail, &keysJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan app event: %w", err)
		}
		if err := json.Unmarshal([]byte(keysJSON), &e.Keys); err != nil {
			return nil, fmt.Errorf("store: decode app event keys: %w", err)
		}
		t, err := time.Parse(appEventTimeFormat, createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse app event time %q: %w", createdAt, err)
		}
		e.CreatedAt = t
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app events: %w", err)
	}
	return out, nil
}

// AppliedConfig is what a service's newest container was created with.
// EnvHashes maps app-owned env keys (literal and secret-backed) to short
// hashes of their values; Fields holds scalar settings such as port.
type AppliedConfig struct {
	ServiceName string
	Release     string
	EnvHashes   map[string]string
	Fields      map[string]string
	AppliedAt   time.Time
	// HeldUntil is when the held previous release is due for removal, zero
	// when none is held.
	HeldUntil time.Time
}

// SaveAppliedConfig replaces name's applied config snapshot.
func (db *DB) SaveAppliedConfig(ctx context.Context, c AppliedConfig) error {
	hashes, err := json.Marshal(nonNilMap(c.EnvHashes))
	if err != nil {
		return fmt.Errorf("store: marshal applied env hashes: %w", err)
	}
	fields, err := json.Marshal(nonNilMap(c.Fields))
	if err != nil {
		return fmt.Errorf("store: marshal applied fields: %w", err)
	}
	at := c.AppliedAt
	if at.IsZero() {
		at = time.Now()
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO applied_config (service_name, release, env_hashes, fields, applied_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (service_name) DO UPDATE SET
			release = excluded.release,
			env_hashes = excluded.env_hashes,
			fields = excluded.fields,
			applied_at = excluded.applied_at
	`, c.ServiceName, c.Release, string(hashes), string(fields), at.UTC().Format(appEventTimeFormat))
	if err != nil {
		return fmt.Errorf("store: save applied config for %q: %w", c.ServiceName, err)
	}
	return nil
}

// SetPreviousReleaseHeldUntil records when name's held previous release is
// due for removal; the zero time clears it.
func (db *DB) SetPreviousReleaseHeldUntil(ctx context.Context, name string, until time.Time) error {
	value := ""
	if !until.IsZero() {
		value = until.UTC().Format(appEventTimeFormat)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO applied_config (service_name, release, applied_at, held_until)
		VALUES (?, '', ?, ?)
		ON CONFLICT (service_name) DO UPDATE SET held_until = excluded.held_until
	`, name, time.Now().UTC().Format(appEventTimeFormat), value)
	if err != nil {
		return fmt.Errorf("store: set previous release hold for %q: %w", name, err)
	}
	return nil
}

// GetAppliedConfig returns name's snapshot, or nil when none was recorded
// (a container created before snapshots existed). A row that only carries a
// hold has an empty Release.
func (db *DB) GetAppliedConfig(ctx context.Context, name string) (*AppliedConfig, error) {
	var (
		c         AppliedConfig
		hashes    string
		fields    string
		appliedAt string
		heldUntil string
	)
	err := db.QueryRowContext(ctx, `SELECT service_name, release, env_hashes, fields, applied_at, held_until FROM applied_config WHERE service_name = ?`, name).
		Scan(&c.ServiceName, &c.Release, &hashes, &fields, &appliedAt, &heldUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get applied config for %q: %w", name, err)
	}
	if err := json.Unmarshal([]byte(hashes), &c.EnvHashes); err != nil {
		return nil, fmt.Errorf("store: decode applied env hashes: %w", err)
	}
	if err := json.Unmarshal([]byte(fields), &c.Fields); err != nil {
		return nil, fmt.Errorf("store: decode applied fields: %w", err)
	}
	t, err := time.Parse(appEventTimeFormat, appliedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse applied config time %q: %w", appliedAt, err)
	}
	c.AppliedAt = t
	if heldUntil != "" {
		if h, err := time.Parse(appEventTimeFormat, heldUntil); err == nil {
			c.HeldUntil = h
		}
	}
	return &c, nil
}

// DeleteAppTimelineData removes name's events and applied config.
func (db *DB) DeleteAppTimelineData(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_events WHERE app_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete app events for %q: %w", name, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM applied_config WHERE service_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete applied config for %q: %w", name, err)
	}
	return nil
}
