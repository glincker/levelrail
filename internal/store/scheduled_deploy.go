package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Scheduled deploy kinds and statuses. pending is the only state a
// scheduled deploy can be canceled from; running is the claim a scanner
// takes before executing, so a restart can never fire it twice.
const (
	ScheduledDeployKindImage = "image"
	ScheduledDeployKindGit   = "git"

	ScheduledDeployPending         = "pending"
	ScheduledDeployRunning         = "running"
	ScheduledDeployFired           = "fired"
	ScheduledDeployPendingApproval = "pending_approval"
	ScheduledDeploySkippedFreeze   = "skipped_freeze"
	ScheduledDeployMissed          = "missed"
	ScheduledDeployCanceled        = "canceled"
	ScheduledDeployFailed          = "failed"

	scheduledDeployIDPrefix = "sdep_"
)

var (
	// ErrScheduledDeployNotFound is returned when no scheduled deploy has the id.
	ErrScheduledDeployNotFound = errors.New("store: scheduled deploy not found")
	// ErrScheduledDeployNotPending is returned when a cancel or claim finds the deploy already past pending.
	ErrScheduledDeployNotPending = errors.New("store: scheduled deploy is not pending")
)

// ScheduledDeploy is one one-shot deploy to run at RunAt.
type ScheduledDeploy struct {
	ID            string
	ServiceName   string
	Kind          string
	Image         string
	Ref           string
	RunAt         time.Time
	Status        string
	Reason        string
	ApprovalID    string
	CreatedByType string
	CreatedBy     string
	CreatedByName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	FinishedAt    *time.Time
}

// NewScheduledDeployID mints an opaque scheduled deploy id.
func NewScheduledDeployID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: mint scheduled deploy id: %w", err)
	}
	return scheduledDeployIDPrefix + hex.EncodeToString(b[:]), nil
}

const scheduledDeployColumns = `id, service_name, kind, image, ref, run_at, status, reason, approval_id,
	created_by_type, created_by, created_by_name, created_at, updated_at, finished_at`

func scanScheduledDeploy(scan func(dest ...any) error) (*ScheduledDeploy, error) {
	var d ScheduledDeploy
	var runAt, created, updated string
	var finished sql.NullString
	if err := scan(&d.ID, &d.ServiceName, &d.Kind, &d.Image, &d.Ref, &runAt, &d.Status, &d.Reason, &d.ApprovalID,
		&d.CreatedByType, &d.CreatedBy, &d.CreatedByName, &created, &updated, &finished); err != nil {
		return nil, err
	}
	var err error
	if d.RunAt, err = time.Parse(time.RFC3339Nano, runAt); err != nil {
		return nil, fmt.Errorf("parse run_at: %w", err)
	}
	if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if d.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	if d.FinishedAt, err = parseNullableTime(finished, "finished_at"); err != nil {
		return nil, err
	}
	return &d, nil
}

// Fixed-width so SQL string comparison of run_at orders correctly.
func tsString(t time.Time) string { return t.UTC().Format(appEventTimeFormat) }

// CreateScheduledDeploy inserts d as pending, minting its id and timestamps.
func (db *DB) CreateScheduledDeploy(ctx context.Context, d ScheduledDeploy) (ScheduledDeploy, error) {
	if d.ID == "" {
		id, err := NewScheduledDeployID()
		if err != nil {
			return ScheduledDeploy{}, err
		}
		d.ID = id
	}
	now := time.Now().UTC()
	d.Status, d.CreatedAt, d.UpdatedAt = ScheduledDeployPending, now, now
	_, err := db.ExecContext(ctx, `
		INSERT INTO scheduled_deploys (`+scheduledDeployColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, d.ID, d.ServiceName, d.Kind, d.Image, d.Ref, tsString(d.RunAt), d.Status, d.Reason, d.ApprovalID,
		d.CreatedByType, d.CreatedBy, d.CreatedByName, tsString(now), tsString(now))
	if err != nil {
		return ScheduledDeploy{}, fmt.Errorf("store: create scheduled deploy for %q: %w", d.ServiceName, err)
	}
	return d, nil
}

// GetScheduledDeploy returns one scheduled deploy of serviceName by id.
func (db *DB) GetScheduledDeploy(ctx context.Context, serviceName, id string) (*ScheduledDeploy, error) {
	row := db.QueryRowContext(ctx, `SELECT `+scheduledDeployColumns+` FROM scheduled_deploys WHERE id = ? AND service_name = ?`, id, serviceName)
	d, err := scanScheduledDeploy(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrScheduledDeployNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get scheduled deploy %q: %w", id, err)
	}
	return d, nil
}

// ListScheduledDeploys lists serviceName's scheduled deploys (every app when
// empty), soonest run_at first among pending ones. statuses filters when non-empty.
func (db *DB) ListScheduledDeploys(ctx context.Context, serviceName string, statuses []string, limit int) ([]ScheduledDeploy, error) {
	q := `SELECT ` + scheduledDeployColumns + ` FROM scheduled_deploys WHERE 1=1`
	var args []any
	if serviceName != "" {
		q += ` AND service_name = ?`
		args = append(args, serviceName)
	}
	if len(statuses) > 0 {
		q += ` AND status IN (?` + strings.Repeat(`,?`, len(statuses)-1) + `)`
		for _, s := range statuses {
			args = append(args, s)
		}
	}
	q += ` ORDER BY run_at DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	return db.queryScheduledDeploys(ctx, q, args...)
}

// ListDueScheduledDeploys returns pending deploys whose run_at is at or before now,
// plus running ones claimed before staleBefore (a crash between claim and finish).
func (db *DB) ListDueScheduledDeploys(ctx context.Context, now, staleBefore time.Time) ([]ScheduledDeploy, error) {
	return db.queryScheduledDeploys(ctx, `SELECT `+scheduledDeployColumns+` FROM scheduled_deploys
		WHERE (status = ? AND run_at <= ?) OR (status = ? AND updated_at <= ?)
		ORDER BY run_at, id`,
		ScheduledDeployPending, tsString(now), ScheduledDeployRunning, tsString(staleBefore))
}

func (db *DB) queryScheduledDeploys(ctx context.Context, q string, args ...any) ([]ScheduledDeploy, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled deploys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ScheduledDeploy
	for rows.Next() {
		d, err := scanScheduledDeploy(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan scheduled deploy: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled deploys: %w", err)
	}
	return out, nil
}

// ClaimScheduledDeploy moves a pending deploy to running; ErrScheduledDeployNotPending
// means a cancel or another tick got there first.
func (db *DB) ClaimScheduledDeploy(ctx context.Context, id string, at time.Time) error {
	return db.transitionScheduledDeploy(ctx, id, ScheduledDeployPending, ScheduledDeployRunning, "", "", at, false)
}

// CancelScheduledDeploy cancels a pending deploy of serviceName.
func (db *DB) CancelScheduledDeploy(ctx context.Context, serviceName, id string, at time.Time) (*ScheduledDeploy, error) {
	d, err := db.GetScheduledDeploy(ctx, serviceName, id)
	if err != nil {
		return nil, err
	}
	if err := db.transitionScheduledDeploy(ctx, id, ScheduledDeployPending, ScheduledDeployCanceled, "canceled before it ran", "", at, true); err != nil {
		return nil, err
	}
	return db.GetScheduledDeploy(ctx, d.ServiceName, id)
}

// FinishScheduledDeploy records the terminal status of a deploy this process claimed.
func (db *DB) FinishScheduledDeploy(ctx context.Context, id, status, reason, approvalID string, at time.Time) error {
	return db.transitionScheduledDeploy(ctx, id, ScheduledDeployRunning, status, reason, approvalID, at, true)
}

func (db *DB) transitionScheduledDeploy(ctx context.Context, id, from, to, reason, approvalID string, at time.Time, finish bool) error {
	finished := any(nil)
	if finish {
		finished = tsString(at)
	}
	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_deploys
		SET status = ?, reason = CASE WHEN ? = '' THEN reason ELSE ? END,
		    approval_id = CASE WHEN ? = '' THEN approval_id ELSE ? END,
		    updated_at = ?, finished_at = COALESCE(?, finished_at)
		WHERE id = ? AND status = ?
	`, to, reason, reason, approvalID, approvalID, tsString(at), finished, id, from)
	if err != nil {
		return fmt.Errorf("store: move scheduled deploy %q to %s: %w", id, to, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrScheduledDeployNotPending
	}
	return nil
}
