package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Database upgrade run states (migrations/0428_db_upgrade_runs.sql).
const (
	DBUpgradeStatePending   = "pending"
	DBUpgradeStateBackingUp = "backing_up"
	DBUpgradeStateUpgrading = "upgrading"
	DBUpgradeStateVerifying = "verifying"
	DBUpgradeStateSucceeded = "succeeded"
	DBUpgradeStateReverted  = "reverted"
	DBUpgradeStateFailed    = "failed"
)

// Database upgrade run sources.
const (
	DBUpgradeSourceAuto   = "auto"
	DBUpgradeSourceManual = "manual"
)

// Automatic upgrade levels a policy may allow. Majors are never automatic.
const (
	DBAutoUpgradeOff   = "off"
	DBAutoUpgradePatch = "patch"
	DBAutoUpgradeMinor = "minor"
)

// DBUpgradePlatformDefault is the policy row every database without its own row inherits.
const DBUpgradePlatformDefault = "*"

// ErrDBUpgradeRunNotFound is returned when an upgrade run id matches no row.
var ErrDBUpgradeRunNotFound = errors.New("store: database upgrade run not found")

// DBUpgradeTerminal reports whether state is a final state.
func DBUpgradeTerminal(state string) bool {
	return state == DBUpgradeStateSucceeded || state == DBUpgradeStateReverted || state == DBUpgradeStateFailed
}

// DBUpgradeRun is one upgrade attempt of a managed database.
type DBUpgradeRun struct {
	ID              string
	DatabaseName    string
	Engine          string
	FromVersion     string
	ToVersion       string
	Kind            string
	Source          string
	State           string
	Phase           string
	VerifyAfter     bool
	RevertOnFailure bool
	Notify          []string
	BackupID        string
	VerificationID  string
	FromImageDigest string
	SnapshotVolume  string
	RevertPath      string
	Reason          string
	RequestedBy     string
	// Timings maps each state and phase entered to its RFC3339 start time.
	Timings        map[string]string
	CreatedAt      string
	PhaseStartedAt string
	FinishedAt     string
}

const dbUpgradeRunColumns = `id, database_name, engine, from_version, to_version, kind, source, state, phase,
	verify_after, revert_on_failure, notify_json, backup_id, verification_id, from_image_digest,
	snapshot_volume, revert_path, reason, requested_by, timings_json, created_at, phase_started_at, finished_at`

// CreateDBUpgradeRun inserts a new run.
func (db *DB) CreateDBUpgradeRun(ctx context.Context, r DBUpgradeRun) error {
	notify, timings, err := encodeRunJSON(r)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO db_upgrade_runs (`+dbUpgradeRunColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.DatabaseName, r.Engine, r.FromVersion, r.ToVersion, r.Kind, r.Source, r.State, r.Phase,
		boolToInt(r.VerifyAfter), boolToInt(r.RevertOnFailure), notify, r.BackupID, r.VerificationID, r.FromImageDigest,
		r.SnapshotVolume, r.RevertPath, r.Reason, r.RequestedBy, timings, r.CreatedAt, r.PhaseStartedAt, r.FinishedAt)
	if err != nil {
		return fmt.Errorf("store: create database upgrade run %q: %w", r.ID, err)
	}
	return nil
}

// UpdateDBUpgradeRun rewrites every mutable field of a run.
func (db *DB) UpdateDBUpgradeRun(ctx context.Context, r DBUpgradeRun) error {
	notify, timings, err := encodeRunJSON(r)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE db_upgrade_runs SET state = ?, phase = ?, notify_json = ?, backup_id = ?,
		verification_id = ?, from_image_digest = ?, snapshot_volume = ?, revert_path = ?, reason = ?,
		timings_json = ?, phase_started_at = ?, finished_at = ? WHERE id = ?`,
		r.State, r.Phase, notify, r.BackupID, r.VerificationID, r.FromImageDigest, r.SnapshotVolume,
		r.RevertPath, r.Reason, timings, r.PhaseStartedAt, r.FinishedAt, r.ID)
	if err != nil {
		return fmt.Errorf("store: update database upgrade run %q: %w", r.ID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("store: update database upgrade run %q: %w", r.ID, err)
	} else if n == 0 {
		return ErrDBUpgradeRunNotFound
	}
	return nil
}

func encodeRunJSON(r DBUpgradeRun) (notify, timings string, err error) {
	n, err := json.Marshal(nonNilStringSlice(r.Notify))
	if err != nil {
		return "", "", fmt.Errorf("store: encode upgrade notify: %w", err)
	}
	tm := r.Timings
	if tm == nil {
		tm = map[string]string{}
	}
	t, err := json.Marshal(tm)
	if err != nil {
		return "", "", fmt.Errorf("store: encode upgrade timings: %w", err)
	}
	return string(n), string(t), nil
}

func nonNilStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func scanDBUpgradeRun(scan func(dest ...any) error) (DBUpgradeRun, error) {
	var (
		r                      DBUpgradeRun
		verify, revert         int
		notifyJSON, timingJSON string
	)
	if err := scan(&r.ID, &r.DatabaseName, &r.Engine, &r.FromVersion, &r.ToVersion, &r.Kind, &r.Source, &r.State, &r.Phase,
		&verify, &revert, &notifyJSON, &r.BackupID, &r.VerificationID, &r.FromImageDigest,
		&r.SnapshotVolume, &r.RevertPath, &r.Reason, &r.RequestedBy, &timingJSON, &r.CreatedAt, &r.PhaseStartedAt, &r.FinishedAt); err != nil {
		return DBUpgradeRun{}, err
	}
	r.VerifyAfter = verify != 0
	r.RevertOnFailure = revert != 0
	if err := json.Unmarshal([]byte(notifyJSON), &r.Notify); err != nil {
		return DBUpgradeRun{}, fmt.Errorf("decode notify: %w", err)
	}
	if err := json.Unmarshal([]byte(timingJSON), &r.Timings); err != nil {
		return DBUpgradeRun{}, fmt.Errorf("decode timings: %w", err)
	}
	return r, nil
}

// GetDBUpgradeRun returns one run or ErrDBUpgradeRunNotFound.
func (db *DB) GetDBUpgradeRun(ctx context.Context, id string) (DBUpgradeRun, error) {
	r, err := scanDBUpgradeRun(db.QueryRowContext(ctx, `SELECT `+dbUpgradeRunColumns+` FROM db_upgrade_runs WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return DBUpgradeRun{}, ErrDBUpgradeRunNotFound
	}
	if err != nil {
		return DBUpgradeRun{}, fmt.Errorf("store: get database upgrade run %q: %w", id, err)
	}
	return r, nil
}

// ListDBUpgradeRuns returns a database's runs, newest first, at most limit (0 means all).
func (db *DB) ListDBUpgradeRuns(ctx context.Context, databaseName string, limit int) ([]DBUpgradeRun, error) {
	q := `WHERE database_name = ? ORDER BY created_at DESC, id DESC`
	args := []any{databaseName}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	return db.queryDBUpgradeRuns(ctx, q, args...)
}

// ListActiveDBUpgradeRuns returns every run not yet in a final state, oldest first.
func (db *DB) ListActiveDBUpgradeRuns(ctx context.Context) ([]DBUpgradeRun, error) {
	return db.queryDBUpgradeRuns(ctx, `WHERE state NOT IN (?, ?, ?) ORDER BY created_at, id`,
		DBUpgradeStateSucceeded, DBUpgradeStateReverted, DBUpgradeStateFailed)
}

func (db *DB) queryDBUpgradeRuns(ctx context.Context, where string, args ...any) ([]DBUpgradeRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+dbUpgradeRunColumns+` FROM db_upgrade_runs `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list database upgrade runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DBUpgradeRun
	for rows.Next() {
		r, err := scanDBUpgradeRun(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan database upgrade run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate database upgrade runs: %w", err)
	}
	return out, nil
}

// DBUpgradePolicy is a database's (or the platform default's) automatic upgrade policy.
type DBUpgradePolicy struct {
	DatabaseName    string
	AutoUpgrade     string
	WindowCron      string
	WindowDuration  time.Duration
	WindowTimezone  string
	BackupBefore    bool
	VerifyAfter     bool
	RevertOnFailure bool
	Notify          []string
	UpdatedBy       string
	UpdatedAt       string
}

const dbUpgradePolicyColumns = `database_name, auto_upgrade, window_cron, window_duration_seconds, window_timezone,
	backup_before, verify_after, revert_on_failure, notify_json, updated_by, updated_at`

// GetDBUpgradePolicy returns the policy row for name; found is false when none exists.
func (db *DB) GetDBUpgradePolicy(ctx context.Context, name string) (p DBUpgradePolicy, found bool, err error) {
	p, err = scanDBUpgradePolicy(db.QueryRowContext(ctx, `SELECT `+dbUpgradePolicyColumns+` FROM db_upgrade_policies WHERE database_name = ?`, name).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return DBUpgradePolicy{}, false, nil
	}
	if err != nil {
		return DBUpgradePolicy{}, false, fmt.Errorf("store: get database upgrade policy %q: %w", name, err)
	}
	return p, true, nil
}

// ListDBUpgradePolicies returns every policy row, the platform default included.
func (db *DB) ListDBUpgradePolicies(ctx context.Context) ([]DBUpgradePolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+dbUpgradePolicyColumns+` FROM db_upgrade_policies ORDER BY database_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list database upgrade policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DBUpgradePolicy
	for rows.Next() {
		p, err := scanDBUpgradePolicy(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan database upgrade policy: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate database upgrade policies: %w", err)
	}
	return out, nil
}

func scanDBUpgradePolicy(scan func(dest ...any) error) (DBUpgradePolicy, error) {
	var (
		p                      DBUpgradePolicy
		seconds                int64
		backup, verify, revert int
		notifyJSON             string
	)
	if err := scan(&p.DatabaseName, &p.AutoUpgrade, &p.WindowCron, &seconds, &p.WindowTimezone,
		&backup, &verify, &revert, &notifyJSON, &p.UpdatedBy, &p.UpdatedAt); err != nil {
		return DBUpgradePolicy{}, err
	}
	p.WindowDuration = time.Duration(seconds) * time.Second
	p.BackupBefore = backup != 0
	p.VerifyAfter = verify != 0
	p.RevertOnFailure = revert != 0
	if err := json.Unmarshal([]byte(notifyJSON), &p.Notify); err != nil {
		return DBUpgradePolicy{}, fmt.Errorf("decode notify: %w", err)
	}
	return p, nil
}

// SaveDBUpgradePolicy creates or replaces a policy row.
func (db *DB) SaveDBUpgradePolicy(ctx context.Context, p DBUpgradePolicy) error {
	notify, err := json.Marshal(nonNilStringSlice(p.Notify))
	if err != nil {
		return fmt.Errorf("store: encode upgrade policy notify: %w", err)
	}
	tz := p.WindowTimezone
	if tz == "" {
		tz = "UTC"
	}
	_, err = db.ExecContext(ctx, `INSERT INTO db_upgrade_policies (`+dbUpgradePolicyColumns+`)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT (database_name) DO UPDATE SET
			auto_upgrade = excluded.auto_upgrade, window_cron = excluded.window_cron,
			window_duration_seconds = excluded.window_duration_seconds, window_timezone = excluded.window_timezone,
			verify_after = excluded.verify_after, revert_on_failure = excluded.revert_on_failure,
			notify_json = excluded.notify_json, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		p.DatabaseName, p.AutoUpgrade, p.WindowCron, int64(p.WindowDuration/time.Second), tz,
		boolToInt(p.VerifyAfter), boolToInt(p.RevertOnFailure), string(notify), p.UpdatedBy, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save database upgrade policy %q: %w", p.DatabaseName, err)
	}
	return nil
}

// DeleteDBUpgradePolicy removes a database's own row so it inherits the
// platform default again. The default row itself is never deleted.
func (db *DB) DeleteDBUpgradePolicy(ctx context.Context, name string) error {
	if name == DBUpgradePlatformDefault {
		return errors.New("store: the platform default upgrade policy cannot be deleted")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM db_upgrade_policies WHERE database_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete database upgrade policy %q: %w", name, err)
	}
	return nil
}
