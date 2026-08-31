package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// MaxWebhookDeliveryPayloadBytes bounds how much of a webhook delivery's
// raw payload SaveWebhookDelivery persists: debug data for an operator to
// inspect and replay, not a permanent audit log, so an unusually large
// payload is truncated rather than stored in full.
const MaxWebhookDeliveryPayloadBytes = 64 * 1024

// WebhookDelivery is one recorded inbound git-provider webhook request
// (internal/api/git_webhook.go's handleGitPushWebhook): what arrived,
// whether it verified, what it resolved to, and what this control plane
// did about it. Exists so an operator can see why a delivery failed
// (bad signature, no git source connected, a downstream deploy error)
// without re-triggering a real push, and can replay a stored payload
// through the exact same processing path once whatever was wrong is
// fixed.
type WebhookDelivery struct {
	// ID is an opaque, mint-time-random identifier from
	// NewWebhookDeliveryID, the same shape NewDeployAttemptID already
	// establishes.
	ID          string
	ServiceName string
	// Provider and EventType are detected from request headers alone
	// (detectWebhookProviderAndEvent), debug metadata only: nothing in
	// this package's own processing branches on them.
	Provider  string
	EventType string
	// HeaderFields carries the small set of provider event-discriminator
	// headers (X-GitHub-Event, X-Gitlab-Event, X-Event-Key) needed to
	// replay this delivery through the identical processing branch it
	// originally took. Deliberately not the full header set: no
	// signature or token header value is ever stored here, only the
	// three name-of-event headers, none of which are secrets.
	HeaderFields   map[string]string
	SignatureValid bool
	// Matched reports whether ServiceName resolved to a connected git
	// source at receipt time, the "app not found" visibility this table
	// exists for in the first place.
	Matched          bool
	StatusCode       int
	Payload          []byte
	PayloadTruncated bool
	// Error is the same non-leaky status message the webhook response
	// itself returned (e.g. "invalid signature", "deploy failed"), never
	// a secret or the payload's own contents.
	Error      string
	ReceivedAt time.Time
}

const webhookDeliveryIDPrefix = "whd_"

// NewWebhookDeliveryID generates an opaque, URL-safe webhook-delivery
// identifier, the same mint scheme NewDeployAttemptID already
// establishes (fixed-length crypto/rand bytes, base64 URL encoding, a
// short prefix).
func NewWebhookDeliveryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate webhook delivery id: %w", err)
	}
	return webhookDeliveryIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveWebhookDelivery inserts a new webhook delivery row, truncating
// d.Payload to MaxWebhookDeliveryPayloadBytes if larger (setting
// PayloadTruncated on the stored row regardless of what the caller
// passed in): this is the one place that cap is enforced, so no caller
// can accidentally bypass it.
func (db *DB) SaveWebhookDelivery(ctx context.Context, d WebhookDelivery) error {
	payload := d.Payload
	if payload == nil {
		payload = []byte{}
	}
	truncated := d.PayloadTruncated
	if len(payload) > MaxWebhookDeliveryPayloadBytes {
		payload = payload[:MaxWebhookDeliveryPayloadBytes]
		truncated = true
	}

	headerFieldsJSON, err := json.Marshal(d.HeaderFields)
	if err != nil {
		return fmt.Errorf("store: marshal webhook delivery header fields %q: %w", d.ID, err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO webhook_deliveries (id, service_name, provider, event_type, header_fields, signature_valid, matched, status_code, payload, payload_truncated, error, received_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.ServiceName, d.Provider, d.EventType, string(headerFieldsJSON),
		boolToInt(d.SignatureValid), boolToInt(d.Matched), d.StatusCode,
		payload, boolToInt(truncated), d.Error, d.ReceivedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save webhook delivery %q: %w", d.ID, err)
	}
	return nil
}

// ErrWebhookDeliveryNotFound is returned by GetWebhookDelivery when no
// row has the given ID.
var ErrWebhookDeliveryNotFound = errors.New("store: webhook delivery not found")

// GetWebhookDelivery returns one webhook delivery by ID, or
// ErrWebhookDeliveryNotFound. Used by the replay handler to load the
// stored payload and header fields for the exact request it's about to
// re-run.
func (db *DB) GetWebhookDelivery(ctx context.Context, id string) (*WebhookDelivery, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, service_name, provider, event_type, header_fields, signature_valid, matched, status_code, payload, payload_truncated, error, received_at
		FROM webhook_deliveries WHERE id = ?
	`, id)
	d, err := scanWebhookDelivery(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWebhookDeliveryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get webhook delivery %q: %w", id, err)
	}
	return d, nil
}

// ListWebhookDeliveries returns up to limit deliveries for serviceName,
// newest first. before, when non-nil, cursor-paginates on receipt time,
// mirroring ListBackupHistory/ListAuditEntries: a webhook receiver sees
// steady traffic over an app's lifetime, so this must never degrade into
// a slow OFFSET query as history grows.
func (db *DB) ListWebhookDeliveries(ctx context.Context, serviceName string, limit int, before *time.Time) ([]WebhookDelivery, error) {
	var (
		rows *sql.Rows
		err  error
	)
	const selectCols = `id, service_name, provider, event_type, header_fields, signature_valid, matched, status_code, payload, payload_truncated, error, received_at`
	if before != nil {
		rows, err = db.QueryContext(ctx, `
			SELECT `+selectCols+`
			FROM webhook_deliveries
			WHERE service_name = ? AND received_at < ?
			ORDER BY received_at DESC
			LIMIT ?
		`, serviceName, before.UTC().Format(time.RFC3339Nano), limit)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT `+selectCols+`
			FROM webhook_deliveries
			WHERE service_name = ?
			ORDER BY received_at DESC
			LIMIT ?
		`, serviceName, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list webhook deliveries for %q: %w", serviceName, err)
	}
	defer func() { _ = rows.Close() }()

	var out []WebhookDelivery
	for rows.Next() {
		d, err := scanWebhookDelivery(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan webhook delivery row: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate webhook delivery rows: %w", err)
	}
	return out, nil
}

func scanWebhookDelivery(scan func(dest ...any) error) (*WebhookDelivery, error) {
	var (
		d                       WebhookDelivery
		headerFieldsJSON        string
		signatureValid, matched int
		payloadTruncated        int
		receivedAt              string
	)
	if err := scan(&d.ID, &d.ServiceName, &d.Provider, &d.EventType, &headerFieldsJSON,
		&signatureValid, &matched, &d.StatusCode, &d.Payload, &payloadTruncated, &d.Error, &receivedAt); err != nil {
		return nil, err
	}

	if headerFieldsJSON != "" {
		if err := json.Unmarshal([]byte(headerFieldsJSON), &d.HeaderFields); err != nil {
			return nil, fmt.Errorf("parse header_fields %q: %w", headerFieldsJSON, err)
		}
	}
	d.SignatureValid = intToBool(signatureValid)
	d.Matched = intToBool(matched)
	d.PayloadTruncated = intToBool(payloadTruncated)

	received, err := time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return nil, fmt.Errorf("parse received_at %q: %w", receivedAt, err)
	}
	d.ReceivedAt = received

	return &d, nil
}
