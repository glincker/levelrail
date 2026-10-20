package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by the pipeline webhook queries.
var (
	ErrPipelineWebhookNotFound   = errors.New("store: pipeline webhook trigger not found")
	ErrPipelineDeliveryNotFound  = errors.New("store: pipeline webhook delivery not found")
	ErrPipelineDeliveryDuplicate = errors.New("store: pipeline webhook delivery already received")
)

// Webhook delivery outcomes.
const (
	WebhookOutcomeReceived = "received"
	WebhookOutcomeStarted  = "started"
	WebhookOutcomeRejected = "rejected"
	WebhookOutcomeFailed   = "failed"
)

// PipelineWebhookTrigger is a pipeline's inbound webhook. TokenHash is the
// sha256 of the URL token; HMAC means requests must also be signed.
type PipelineWebhookTrigger struct {
	PipelineID string
	TokenHash  string
	HMAC       bool
	Enabled    bool
	CreatedAt  time.Time
	RotatedAt  time.Time
	LastUsedAt *time.Time
}

// PipelineWebhookDelivery is one authenticated request to a trigger.
type PipelineWebhookDelivery struct {
	ID             string
	PipelineID     string
	ReceivedAt     time.Time
	StatusCode     int
	Outcome        string
	Reason         string
	RunID          string
	IdempotencyKey string
	ContentType    string
	Query          string
	Body           []byte
}

// PipelineWebhookSecretsKey is the secrets namespace holding a trigger's
// HMAC shared secret, apart from the app's runtime env secrets.
func PipelineWebhookSecretsKey(pipelineID string) string {
	return "pipeline-webhook/" + pipelineID
}

// SavePipelineWebhookTrigger creates or replaces a pipeline's trigger.
func (db *DB) SavePipelineWebhookTrigger(ctx context.Context, t PipelineWebhookTrigger) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO pipeline_webhook_triggers (pipeline_id, token_hash, hmac, enabled, created_at, rotated_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (pipeline_id) DO UPDATE SET token_hash = excluded.token_hash, hmac = excluded.hmac,
			enabled = excluded.enabled, rotated_at = excluded.rotated_at
	`, t.PipelineID, t.TokenHash, t.HMAC, t.Enabled, formatTime(t.CreatedAt), formatTime(t.RotatedAt), formatTimePtr(t.LastUsedAt)); err != nil {
		return fmt.Errorf("store: save pipeline webhook trigger: %w", err)
	}
	return nil
}

// GetPipelineWebhookTrigger returns a pipeline's trigger, or ErrPipelineWebhookNotFound.
func (db *DB) GetPipelineWebhookTrigger(ctx context.Context, pipelineID string) (PipelineWebhookTrigger, error) {
	var t PipelineWebhookTrigger
	var created, rotated string
	var used sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT pipeline_id, token_hash, hmac, enabled, created_at, rotated_at, last_used_at
		FROM pipeline_webhook_triggers WHERE pipeline_id = ?`, pipelineID).Scan(&t.PipelineID, &t.TokenHash, &t.HMAC, &t.Enabled, &created, &rotated, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return PipelineWebhookTrigger{}, ErrPipelineWebhookNotFound
	}
	if err != nil {
		return PipelineWebhookTrigger{}, fmt.Errorf("store: get pipeline webhook trigger: %w", err)
	}
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return PipelineWebhookTrigger{}, fmt.Errorf("parse created_at: %w", err)
	}
	if t.RotatedAt, err = time.Parse(time.RFC3339Nano, rotated); err != nil {
		return PipelineWebhookTrigger{}, fmt.Errorf("parse rotated_at: %w", err)
	}
	if t.LastUsedAt, err = parseTimePtr(used); err != nil {
		return PipelineWebhookTrigger{}, fmt.Errorf("parse last_used_at: %w", err)
	}
	return t, nil
}

// DeletePipelineWebhookTrigger removes the trigger. Deliveries stay for history.
func (db *DB) DeletePipelineWebhookTrigger(ctx context.Context, pipelineID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM pipeline_webhook_triggers WHERE pipeline_id = ?`, pipelineID); err != nil {
		return fmt.Errorf("store: delete pipeline webhook trigger: %w", err)
	}
	return nil
}

// TouchPipelineWebhookTrigger records that the trigger just received a request.
func (db *DB) TouchPipelineWebhookTrigger(ctx context.Context, pipelineID string, at time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE pipeline_webhook_triggers SET last_used_at = ? WHERE pipeline_id = ?`, formatTime(at), pipelineID); err != nil {
		return fmt.Errorf("store: touch pipeline webhook trigger: %w", err)
	}
	return nil
}

// AddPipelineWebhookDelivery inserts a delivery. A repeated non-empty
// idempotency key returns ErrPipelineDeliveryDuplicate and the first
// delivery, so the caller can answer with the run it already started.
func (db *DB) AddPipelineWebhookDelivery(ctx context.Context, d PipelineWebhookDelivery) (PipelineWebhookDelivery, error) {
	if d.Body == nil {
		d.Body = []byte{}
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO pipeline_webhook_deliveries (id, pipeline_id, received_at, status_code, outcome, reason, run_id, idempotency_key, content_type, query, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.PipelineID, formatTime(d.ReceivedAt), d.StatusCode, d.Outcome, d.Reason, d.RunID, d.IdempotencyKey, d.ContentType, d.Query, d.Body)
	if err != nil && d.IdempotencyKey != "" && strings.Contains(err.Error(), "UNIQUE") {
		first, gerr := db.getPipelineDeliveryByKey(ctx, d.PipelineID, d.IdempotencyKey)
		if gerr != nil {
			return PipelineWebhookDelivery{}, gerr
		}
		return first, ErrPipelineDeliveryDuplicate
	}
	if err != nil {
		return PipelineWebhookDelivery{}, fmt.Errorf("store: add pipeline webhook delivery: %w", err)
	}
	return d, nil
}

// FinishPipelineWebhookDelivery records how a received delivery ended.
func (db *DB) FinishPipelineWebhookDelivery(ctx context.Context, id, outcome, reason, runID string, status int) error {
	if _, err := db.ExecContext(ctx, `UPDATE pipeline_webhook_deliveries SET outcome = ?, reason = ?, run_id = ?, status_code = ? WHERE id = ?`,
		outcome, reason, runID, status, id); err != nil {
		return fmt.Errorf("store: finish pipeline webhook delivery: %w", err)
	}
	return nil
}

const pipelineDeliveryCols = `id, pipeline_id, received_at, status_code, outcome, reason, run_id, idempotency_key, content_type, query, body`

func scanPipelineDelivery(scan func(...any) error) (PipelineWebhookDelivery, error) {
	var d PipelineWebhookDelivery
	var received string
	if err := scan(&d.ID, &d.PipelineID, &received, &d.StatusCode, &d.Outcome, &d.Reason, &d.RunID, &d.IdempotencyKey, &d.ContentType, &d.Query, &d.Body); err != nil {
		return PipelineWebhookDelivery{}, err
	}
	var err error
	if d.ReceivedAt, err = time.Parse(time.RFC3339Nano, received); err != nil {
		return PipelineWebhookDelivery{}, fmt.Errorf("parse received_at: %w", err)
	}
	return d, nil
}

func (db *DB) getPipelineDeliveryByKey(ctx context.Context, pipelineID, key string) (PipelineWebhookDelivery, error) {
	d, err := scanPipelineDelivery(db.QueryRowContext(ctx, `SELECT `+pipelineDeliveryCols+` FROM pipeline_webhook_deliveries WHERE pipeline_id = ? AND idempotency_key = ?`, pipelineID, key).Scan)
	if err != nil {
		return PipelineWebhookDelivery{}, fmt.Errorf("store: get pipeline webhook delivery by key: %w", err)
	}
	return d, nil
}

// GetPipelineWebhookDelivery returns one delivery, or ErrPipelineDeliveryNotFound.
func (db *DB) GetPipelineWebhookDelivery(ctx context.Context, pipelineID, id string) (PipelineWebhookDelivery, error) {
	d, err := scanPipelineDelivery(db.QueryRowContext(ctx, `SELECT `+pipelineDeliveryCols+` FROM pipeline_webhook_deliveries WHERE pipeline_id = ? AND id = ?`, pipelineID, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PipelineWebhookDelivery{}, ErrPipelineDeliveryNotFound
	}
	if err != nil {
		return PipelineWebhookDelivery{}, fmt.Errorf("store: get pipeline webhook delivery: %w", err)
	}
	return d, nil
}

// ListPipelineWebhookDeliveries returns a pipeline's deliveries newest first.
func (db *DB) ListPipelineWebhookDeliveries(ctx context.Context, pipelineID string, limit int) ([]PipelineWebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT `+pipelineDeliveryCols+` FROM pipeline_webhook_deliveries WHERE pipeline_id = ? ORDER BY received_at DESC, id DESC LIMIT ?`, pipelineID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline webhook deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineWebhookDelivery
	for rows.Next() {
		d, err := scanPipelineDelivery(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan pipeline webhook delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PrunePipelineWebhookDeliveries keeps the newest keep deliveries of a pipeline.
func (db *DB) PrunePipelineWebhookDeliveries(ctx context.Context, pipelineID string, keep int) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM pipeline_webhook_deliveries WHERE pipeline_id = ? AND id NOT IN
			(SELECT id FROM pipeline_webhook_deliveries WHERE pipeline_id = ? ORDER BY received_at DESC, id DESC LIMIT ?)
	`, pipelineID, pipelineID, keep); err != nil {
		return fmt.Errorf("store: prune pipeline webhook deliveries: %w", err)
	}
	return nil
}
