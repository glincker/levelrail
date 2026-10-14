package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Delivery states of one event webhook delivery.
const (
	EventDeliveryPending   = "pending"
	EventDeliveryDelivered = "delivered"
	EventDeliveryDead      = "dead"
)

// ErrEventWebhookNotFound is returned when no webhook has the given id.
var ErrEventWebhookNotFound = errors.New("store: event webhook not found")

// ErrEventWebhookNameTaken is returned when a webhook name is already in use.
var ErrEventWebhookNameTaken = errors.New("store: event webhook name already in use")

// ErrEventDeliveryNotFound is returned when no delivery has the given id.
var ErrEventDeliveryNotFound = errors.New("store: event webhook delivery not found")

// EventWebhook is one outbound event subscription.
type EventWebhook struct {
	ID        string
	Name      string
	URL       string
	Enabled   bool
	Events    []string
	CreatedAt string
	UpdatedAt string
}

// EventWebhookDelivery is one event queued for, or sent to, a webhook.
type EventWebhookDelivery struct {
	ID             string
	WebhookID      string
	EventID        string
	EventType      string
	Payload        string
	State          string
	Attempts       int
	ResponseStatus int
	LastError      string
	NextAttemptAt  string
	CreatedAt      string
	UpdatedAt      string
}

// EventWebhookSecretsKey is the internal/secrets serviceName a webhook's
// signing secret is stored under (envKey "signing").
func EventWebhookSecretsKey(id string) string { return "event-webhook/" + id }

func newEventID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate %s id: %w", prefix, err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewEventWebhookID mints a webhook id.
func NewEventWebhookID() (string, error) { return newEventID("whk") }

// NewEventDeliveryID mints a delivery id.
func NewEventDeliveryID() (string, error) { return newEventID("dlv") }

const eventWebhookColumns = `id, name, url, enabled, events, created_at, updated_at`

func scanEventWebhook(s interface{ Scan(dest ...any) error }) (EventWebhook, error) {
	var w EventWebhook
	var enabled int
	var events string
	if err := s.Scan(&w.ID, &w.Name, &w.URL, &enabled, &events, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return EventWebhook{}, err
	}
	w.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(events), &w.Events); err != nil {
		return EventWebhook{}, fmt.Errorf("decode events: %w", err)
	}
	return w, nil
}

func wrapEventWebhookWrite(err error, id string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed: event_webhooks.name") {
		return ErrEventWebhookNameTaken
	}
	return fmt.Errorf("store: write event webhook %q: %w", id, err)
}

// SaveEventWebhook inserts a webhook.
func (db *DB) SaveEventWebhook(ctx context.Context, w EventWebhook) error {
	events, err := encodeEvents(w.Events)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO event_webhooks (`+eventWebhookColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.URL, exportBoolInt(w.Enabled), events, w.CreatedAt, w.UpdatedAt)
	return wrapEventWebhookWrite(err, w.ID)
}

// UpdateEventWebhook replaces every mutable field of a webhook.
func (db *DB) UpdateEventWebhook(ctx context.Context, w EventWebhook) error {
	events, err := encodeEvents(w.Events)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE event_webhooks SET name = ?, url = ?, enabled = ?, events = ?, updated_at = ? WHERE id = ?`,
		w.Name, w.URL, exportBoolInt(w.Enabled), events, w.UpdatedAt, w.ID)
	if err != nil {
		return wrapEventWebhookWrite(err, w.ID)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEventWebhookNotFound
	}
	return nil
}

func encodeEvents(e []string) (string, error) {
	if e == nil {
		e = []string{}
	}
	b, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("store: encode event webhook events: %w", err)
	}
	return string(b), nil
}

// GetEventWebhook returns one webhook.
func (db *DB) GetEventWebhook(ctx context.Context, id string) (EventWebhook, error) {
	w, err := scanEventWebhook(db.QueryRowContext(ctx, `SELECT `+eventWebhookColumns+` FROM event_webhooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return EventWebhook{}, ErrEventWebhookNotFound
	}
	if err != nil {
		return EventWebhook{}, fmt.Errorf("store: get event webhook %q: %w", id, err)
	}
	return w, nil
}

// ListEventWebhooks returns every webhook ordered by name.
func (db *DB) ListEventWebhooks(ctx context.Context) ([]EventWebhook, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+eventWebhookColumns+` FROM event_webhooks ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list event webhooks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EventWebhook
	for rows.Next() {
		w, err := scanEventWebhook(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan event webhook: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteEventWebhook removes a webhook and its delivery log.
func (db *DB) DeleteEventWebhook(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM event_webhook_deliveries WHERE webhook_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete event webhook deliveries %q: %w", id, err)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM event_webhooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete event webhook %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEventWebhookNotFound
	}
	return nil
}

const eventDeliveryColumns = `id, webhook_id, event_id, event_type, payload, state, attempts, response_status, last_error, next_attempt_at, created_at, updated_at`

func scanEventDelivery(s interface{ Scan(dest ...any) error }) (EventWebhookDelivery, error) {
	var d EventWebhookDelivery
	err := s.Scan(&d.ID, &d.WebhookID, &d.EventID, &d.EventType, &d.Payload, &d.State, &d.Attempts,
		&d.ResponseStatus, &d.LastError, &d.NextAttemptAt, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// SaveEventDelivery inserts a delivery.
func (db *DB) SaveEventDelivery(ctx context.Context, d EventWebhookDelivery) error {
	_, err := db.ExecContext(ctx, `INSERT INTO event_webhook_deliveries (`+eventDeliveryColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.WebhookID, d.EventID, d.EventType, d.Payload, d.State, d.Attempts, d.ResponseStatus,
		d.LastError, d.NextAttemptAt, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save event delivery %q: %w", d.ID, err)
	}
	return nil
}

// UpdateEventDelivery writes the mutable delivery fields.
func (db *DB) UpdateEventDelivery(ctx context.Context, d EventWebhookDelivery) error {
	_, err := db.ExecContext(ctx, `UPDATE event_webhook_deliveries SET state = ?, attempts = ?, response_status = ?, last_error = ?, next_attempt_at = ?, updated_at = ? WHERE id = ?`,
		d.State, d.Attempts, d.ResponseStatus, d.LastError, d.NextAttemptAt, d.UpdatedAt, d.ID)
	if err != nil {
		return fmt.Errorf("store: update event delivery %q: %w", d.ID, err)
	}
	return nil
}

// GetEventDelivery returns one delivery.
func (db *DB) GetEventDelivery(ctx context.Context, id string) (EventWebhookDelivery, error) {
	d, err := scanEventDelivery(db.QueryRowContext(ctx, `SELECT `+eventDeliveryColumns+` FROM event_webhook_deliveries WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return EventWebhookDelivery{}, ErrEventDeliveryNotFound
	}
	if err != nil {
		return EventWebhookDelivery{}, fmt.Errorf("store: get event delivery %q: %w", id, err)
	}
	return d, nil
}

// ListDueEventDeliveries returns pending deliveries whose retry time has
// passed, oldest first.
func (db *DB) ListDueEventDeliveries(ctx context.Context, now string, limit int) ([]EventWebhookDelivery, error) {
	return db.queryEventDeliveries(ctx, `WHERE state = ? AND next_attempt_at <= ? ORDER BY next_attempt_at LIMIT ?`,
		EventDeliveryPending, now, limit)
}

// ListEventDeliveries returns a webhook's newest deliveries first.
func (db *DB) ListEventDeliveries(ctx context.Context, webhookID string, limit int) ([]EventWebhookDelivery, error) {
	return db.queryEventDeliveries(ctx, `WHERE webhook_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, webhookID, limit)
}

func (db *DB) queryEventDeliveries(ctx context.Context, tail string, args ...any) ([]EventWebhookDelivery, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+eventDeliveryColumns+` FROM event_webhook_deliveries `+tail, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list event deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EventWebhookDelivery
	for rows.Next() {
		d, err := scanEventDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan event delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteEventDeliveriesOlderThan prunes finished deliveries before cutoff.
func (db *DB) DeleteEventDeliveriesOlderThan(ctx context.Context, cutoff string) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM event_webhook_deliveries WHERE state != ? AND updated_at < ?`, EventDeliveryPending, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: prune event deliveries: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
