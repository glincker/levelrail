package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Delivery states of a status webhook delivery.
const (
	WebhookPending   = "pending"
	WebhookDelivered = "delivered"
	WebhookFailed    = "failed"
)

// StatusWebhook is one signed webhook subscriber of the status page. SecretPlain
// is empty while the signing secret is sealed in the secrets manager.
type StatusWebhook struct {
	ID                  string
	URL                 string
	Label               string
	Events              []string
	SecretPlain         string
	Sealed              bool
	Paused              bool
	PausedReason        string
	ConsecutiveFailures int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// StatusWebhookDelivery is one event for one subscriber: outbox row and log line.
type StatusWebhookDelivery struct {
	ID            string
	WebhookID     string
	EventID       string
	EventType     string
	Payload       string
	State         string
	Attempts      int
	NextAttemptAt time.Time
	LastStatus    int
	LastError     string
	CreatedAt     time.Time
	DeliveredAt   *time.Time
}

// Sentinel errors for status webhooks.
var (
	ErrStatusWebhookNotFound = errors.New("store: status webhook not found")
	ErrStatusWebhookLimit    = errors.New("store: status webhook limit reached")
)

const statusWebhookColumns = `SELECT id, url, label, events, secret_plain, sealed, paused, paused_reason, consecutive_failures, created_at, updated_at FROM status_webhooks`

func scanStatusWebhook(scan func(dest ...any) error) (StatusWebhook, error) {
	var (
		w              StatusWebhook
		events         string
		sealed, paused int
		created, upd   string
	)
	if err := scan(&w.ID, &w.URL, &w.Label, &events, &w.SecretPlain, &sealed, &paused, &w.PausedReason, &w.ConsecutiveFailures, &created, &upd); err != nil {
		return StatusWebhook{}, err
	}
	if events != "" {
		w.Events = strings.Split(events, ",")
	}
	w.Sealed, w.Paused = sealed == 1, paused == 1
	var err error
	if w.CreatedAt, err = parseStatusTime(created); err != nil {
		return StatusWebhook{}, err
	}
	if w.UpdatedAt, err = parseStatusTime(upd); err != nil {
		return StatusWebhook{}, err
	}
	return w, nil
}

// CreateStatusWebhook inserts a subscriber unless max subscribers already exist.
func (db *DB) CreateStatusWebhook(ctx context.Context, w StatusWebhook, limit int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create status webhook: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM status_webhooks`).Scan(&n); err != nil {
		return fmt.Errorf("store: count status webhooks: %w", err)
	}
	if limit > 0 && n >= limit {
		return ErrStatusWebhookLimit
	}
	now := statusTime(time.Now())
	if _, err := tx.ExecContext(ctx, `INSERT INTO status_webhooks (id, url, label, events, secret_plain, sealed, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.URL, w.Label, strings.Join(w.Events, ","), w.SecretPlain, boolInt(w.Sealed), now, now); err != nil {
		return fmt.Errorf("store: insert status webhook: %w", err)
	}
	return tx.Commit()
}

// ListStatusWebhooks returns subscribers oldest first.
func (db *DB) ListStatusWebhooks(ctx context.Context) ([]StatusWebhook, error) {
	rows, err := db.QueryContext(ctx, statusWebhookColumns+` ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list status webhooks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusWebhook
	for rows.Next() {
		w, err := scanStatusWebhook(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan status webhook: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetStatusWebhook returns one subscriber or ErrStatusWebhookNotFound.
func (db *DB) GetStatusWebhook(ctx context.Context, id string) (StatusWebhook, error) {
	w, err := scanStatusWebhook(db.QueryRowContext(ctx, statusWebhookColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusWebhook{}, ErrStatusWebhookNotFound
	}
	if err != nil {
		return StatusWebhook{}, fmt.Errorf("store: get status webhook %q: %w", id, err)
	}
	return w, nil
}

// SetStatusWebhookSecret replaces the stored secret column and its sealed flag.
func (db *DB) SetStatusWebhookSecret(ctx context.Context, id, plain string, sealed bool) error {
	res, err := db.ExecContext(ctx, `UPDATE status_webhooks SET secret_plain = ?, sealed = ?, updated_at = ? WHERE id = ?`,
		plain, boolInt(sealed), statusTime(time.Now()), id)
	return webhookAffected(res, err, "set secret")
}

// SetStatusWebhookPaused pauses or resumes a subscriber; resuming clears the failure streak.
func (db *DB) SetStatusWebhookPaused(ctx context.Context, id string, paused bool, reason string) error {
	res, err := db.ExecContext(ctx, `UPDATE status_webhooks SET paused = ?, paused_reason = ?, consecutive_failures = CASE WHEN ? = 0 THEN 0 ELSE consecutive_failures END, updated_at = ? WHERE id = ?`,
		boolInt(paused), reason, boolInt(paused), statusTime(time.Now()), id)
	return webhookAffected(res, err, "set paused")
}

func webhookAffected(res sql.Result, err error, what string) error {
	if err != nil {
		return fmt.Errorf("store: status webhook %s: %w", what, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStatusWebhookNotFound
	}
	return nil
}

// DeleteStatusWebhook removes a subscriber and its delivery log.
func (db *DB) DeleteStatusWebhook(ctx context.Context, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete status webhook: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM status_webhook_deliveries WHERE webhook_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete status webhook deliveries: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM status_webhooks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete status webhook: %w", err)
	}
	return tx.Commit()
}

// AddStatusWebhookDelivery inserts a delivery; a repeated (webhook, event) pair is ignored and reported as false.
func (db *DB) AddStatusWebhookDelivery(ctx context.Context, d StatusWebhookDelivery) (bool, error) {
	var delivered any
	if d.DeliveredAt != nil {
		delivered = statusTime(*d.DeliveredAt)
	}
	res, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO status_webhook_deliveries
		(id, webhook_id, event_id, event_type, payload, state, attempts, next_attempt_at, last_status, last_error, created_at, delivered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.WebhookID, d.EventID, d.EventType, d.Payload, d.State, d.Attempts, statusTime(d.NextAttemptAt),
		d.LastStatus, d.LastError, statusTime(d.CreatedAt), delivered)
	if err != nil {
		return false, fmt.Errorf("store: add status webhook delivery: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

const statusWebhookDeliveryColumns = `SELECT id, webhook_id, event_id, event_type, payload, state, attempts, next_attempt_at, last_status, last_error, created_at, delivered_at FROM status_webhook_deliveries`

func scanStatusWebhookDelivery(scan func(dest ...any) error) (StatusWebhookDelivery, error) {
	var (
		d                  StatusWebhookDelivery
		next, created      string
		delivered          sql.NullString
		err                error
		nextT, createdTime time.Time
	)
	if err = scan(&d.ID, &d.WebhookID, &d.EventID, &d.EventType, &d.Payload, &d.State, &d.Attempts, &next, &d.LastStatus, &d.LastError, &created, &delivered); err != nil {
		return d, err
	}
	if nextT, err = parseStatusTime(next); err != nil {
		return d, err
	}
	if createdTime, err = parseStatusTime(created); err != nil {
		return d, err
	}
	d.NextAttemptAt, d.CreatedAt = nextT, createdTime
	d.DeliveredAt, err = parseNullableStatusTime(delivered)
	return d, err
}

func queryStatusWebhookDeliveries(ctx context.Context, db *DB, q string, args ...any) ([]StatusWebhookDelivery, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query status webhook deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusWebhookDelivery
	for rows.Next() {
		d, err := scanStatusWebhookDelivery(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan status webhook delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DueStatusWebhookDeliveries returns pending deliveries whose retry time has come, oldest first.
func (db *DB) DueStatusWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]StatusWebhookDelivery, error) {
	return queryStatusWebhookDeliveries(ctx, db, statusWebhookDeliveryColumns+` WHERE state = ? AND next_attempt_at <= ? ORDER BY next_attempt_at LIMIT ?`,
		WebhookPending, statusTime(now), limit)
}

// ListStatusWebhookDeliveries returns a subscriber's delivery log, newest first.
func (db *DB) ListStatusWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]StatusWebhookDelivery, error) {
	return queryStatusWebhookDeliveries(ctx, db, statusWebhookDeliveryColumns+` WHERE webhook_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, webhookID, limit)
}

// RecordStatusWebhookAttempt stores one attempt's outcome and updates the subscriber's failure streak.
// Only attempts with countStreak set move the streak. A streak of pauseAfter failures pauses the subscriber; it reports whether that happened.
func (db *DB) RecordStatusWebhookAttempt(ctx context.Context, d StatusWebhookDelivery, ok, countStreak bool, pauseAfter, keep int) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: record status webhook attempt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var delivered any
	if d.DeliveredAt != nil {
		delivered = statusTime(*d.DeliveredAt)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE status_webhook_deliveries SET state = ?, attempts = ?, next_attempt_at = ?, last_status = ?, last_error = ?, delivered_at = ? WHERE id = ?`,
		d.State, d.Attempts, statusTime(d.NextAttemptAt), d.LastStatus, d.LastError, delivered, d.ID); err != nil {
		return false, fmt.Errorf("store: update status webhook delivery: %w", err)
	}
	paused := false
	switch {
	case !countStreak:
	case ok:
		if _, err := tx.ExecContext(ctx, `UPDATE status_webhooks SET consecutive_failures = 0 WHERE id = ?`, d.WebhookID); err != nil {
			return false, fmt.Errorf("store: reset status webhook failures: %w", err)
		}
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE status_webhooks SET consecutive_failures = consecutive_failures + 1 WHERE id = ?`, d.WebhookID); err != nil {
			return false, fmt.Errorf("store: count status webhook failure: %w", err)
		}
		if pauseAfter > 0 {
			res, err := tx.ExecContext(ctx, `UPDATE status_webhooks SET paused = 1, paused_reason = 'too many consecutive failures' WHERE id = ? AND paused = 0 AND consecutive_failures >= ?`, d.WebhookID, pauseAfter)
			if err != nil {
				return false, fmt.Errorf("store: auto-pause status webhook: %w", err)
			}
			n, _ := res.RowsAffected()
			paused = n > 0
		}
	}
	if keep > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM status_webhook_deliveries WHERE webhook_id = ? AND state != ? AND id NOT IN
			(SELECT id FROM status_webhook_deliveries WHERE webhook_id = ? ORDER BY created_at DESC, id DESC LIMIT ?)`, d.WebhookID, WebhookPending, d.WebhookID, keep); err != nil {
			return false, fmt.Errorf("store: prune status webhook deliveries: %w", err)
		}
	}
	return paused, tx.Commit()
}
