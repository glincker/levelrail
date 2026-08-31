package alerting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

// NotificationDelivery is one recorded attempt to send through a
// NotificationChannel. Every real send path (deploy-outcome dispatch,
// alert-rule dispatch, and the existing test-send routes) records one of
// these via recordDelivery below, so a channel's delivery history
// reflects real send attempts, not just test-button clicks.
type NotificationDelivery struct {
	ID        string
	ChannelID string
	Trigger   string
	Success   bool
	Error     string
	CreatedAt string
}

const notificationDeliveryIDPrefix = "ndl_"

// NewNotificationDeliveryID mirrors NewNotificationChannelID's exact shape.
func NewNotificationDeliveryID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate notification delivery id: %w", err)
	}
	return notificationDeliveryIDPrefix + hex.EncodeToString(b), nil
}

// RecordNotificationDelivery persists one delivery attempt. Insert-only:
// a delivery record is never updated once written.
func (db *DB) RecordNotificationDelivery(ctx context.Context, d NotificationDelivery) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO notification_deliveries (id, channel_id, trigger_reason, success, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, d.ID, d.ChannelID, d.Trigger, boolToInt(d.Success), d.Error, now)
	if err != nil {
		return fmt.Errorf("alerting: record notification delivery %q: %w", d.ID, err)
	}
	return nil
}

// ListNotificationDeliveries returns up to limit delivery records for
// channelID, most recent first, cursor-paginated by before (an optional
// exclusive created_at cutoff), the same shape store.ListAuditEntries
// already uses. Callers are responsible for clamping limit.
func (db *DB) ListNotificationDeliveries(ctx context.Context, channelID string, limit int, before *time.Time) ([]NotificationDelivery, error) {
	query := `
		SELECT id, channel_id, trigger_reason, success, error, created_at
		FROM notification_deliveries
		WHERE channel_id = ?
	`
	args := []any{channelID}
	if before != nil {
		query += " AND created_at < ?"
		args = append(args, before.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("alerting: list notification deliveries for channel %q: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []NotificationDelivery
	for rows.Next() {
		var (
			d          NotificationDelivery
			successInt int
		)
		if err := rows.Scan(&d.ID, &d.ChannelID, &d.Trigger, &successInt, &d.Error, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("alerting: scan notification delivery row: %w", err)
		}
		d.Success = successInt != 0
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate notification delivery rows: %w", err)
	}
	return out, nil
}

// DeliveryRecorder is the narrow surface Engine and DeployDispatcher need
// to persist a delivery attempt. *DB satisfies it structurally.
type DeliveryRecorder interface {
	RecordNotificationDelivery(ctx context.Context, d NotificationDelivery) error
}

// recordDelivery persists one delivery attempt for channelID, logging
// (never propagating) a failure to generate an id or write the row: a
// delivery-history write must never affect whether the underlying send
// succeeded or block its caller. No-op when channelID is empty, which is
// a legacy deploy target or alert rule with no attached channel, so
// there is no channel-scoped history to write to.
func recordDelivery(ctx context.Context, recorder DeliveryRecorder, logger *slog.Logger, channelID, trigger string, sendErr error) {
	if channelID == "" {
		return
	}
	id, err := NewNotificationDeliveryID()
	if err != nil {
		logger.Error("alerting: generate notification delivery id failed", slog.String("error", err.Error()))
		return
	}
	errMsg := ""
	if sendErr != nil {
		errMsg = sendErr.Error()
	}
	d := NotificationDelivery{ID: id, ChannelID: channelID, Trigger: trigger, Success: sendErr == nil, Error: errMsg}
	if err := recorder.RecordNotificationDelivery(ctx, d); err != nil {
		logger.Error("alerting: record notification delivery failed",
			slog.String("channel_id", channelID), slog.String("trigger", trigger), slog.String("error", err.Error()))
	}
}
