package alerting

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
)

// NotificationChannel is a global, connect-once notify destination
// (migrations/0003_notification_channels.sql's own doc comment), reused
// across apps' DeployTarget rows by ID instead of a fresh URL per app.
type NotificationChannel struct {
	ID        string
	Name      string
	Kind      NotifyKind
	NotifyURL string
	Enabled   bool
	CreatedAt string
	UpdatedAt string
}

// ErrNotificationChannelNotFound is returned by GetNotificationChannel
// and DeleteNotificationChannel when id doesn't match any row.
var ErrNotificationChannelNotFound = errors.New("alerting: notification channel not found")

const notificationChannelIDPrefix = "chn_"

// NewNotificationChannelID mirrors NewDeployTargetID's exact shape.
func NewNotificationChannelID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate notification channel id: %w", err)
	}
	return notificationChannelIDPrefix + hex.EncodeToString(b), nil
}

// SaveNotificationChannel creates or fully replaces a channel's
// configuration, the same upsert shape SaveDeployTarget already uses.
func (db *DB) SaveNotificationChannel(ctx context.Context, c NotificationChannel) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO notification_channels (id, name, kind, notify_url, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			notify_url = excluded.notify_url,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, c.ID, c.Name, string(c.Kind), c.NotifyURL, boolToInt(c.Enabled), now, now)
	if err != nil {
		return fmt.Errorf("alerting: save notification channel %q: %w", c.ID, err)
	}
	return nil
}

// GetNotificationChannel returns the channel with this ID, or
// ErrNotificationChannelNotFound.
func (db *DB) GetNotificationChannel(ctx context.Context, id string) (*NotificationChannel, error) {
	row := db.QueryRowContext(ctx, notificationChannelSelectColumns+` WHERE id = ?`, id)
	c, err := scanNotificationChannel(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotificationChannelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get notification channel %q: %w", id, err)
	}
	return c, nil
}

// ListNotificationChannels returns every channel, oldest first (creation
// order, the same order the settings page listing them wants).
func (db *DB) ListNotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := db.QueryContext(ctx, notificationChannelSelectColumns+` ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list notification channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NotificationChannel
	for rows.Next() {
		c, err := scanNotificationChannel(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan notification channel row: %w", err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate notification channel rows: %w", err)
	}
	return out, nil
}

// DeleteNotificationChannel removes a channel row. Attached
// deploy_notify_targets rows have channel_id cleared automatically by
// migrations/0004's ON DELETE SET NULL, never blocking this delete.
func (db *DB) DeleteNotificationChannel(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("alerting: delete notification channel %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("alerting: delete notification channel %q: %w", id, err)
	}
	if n == 0 {
		return ErrNotificationChannelNotFound
	}
	return nil
}

const notificationChannelSelectColumns = `
	SELECT id, name, kind, notify_url, enabled, created_at, updated_at
	FROM notification_channels`

func scanNotificationChannel(scan func(dest ...any) error) (*NotificationChannel, error) {
	var (
		c          NotificationChannel
		kind       string
		enabledInt int
	)
	if err := scan(&c.ID, &c.Name, &kind, &c.NotifyURL, &enabledInt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Kind = NotifyKind(kind)
	c.Enabled = enabledInt != 0
	return &c, nil
}

// sendTestNotification sends a fixed connectivity-check message via
// kind, reusing sendDeployOutcome's own per-channel payload logic.
func sendTestNotification(ctx context.Context, client *http.Client, sender email.Sender, kind NotifyKind, notifyURL string) error {
	if client == nil {
		client = http.DefaultClient
	}
	const testText = "Levelrail test notification. If you can see this, the connection works."

	switch kind {
	case NotifySlack:
		return postJSON(ctx, client, notifyURL, slackPayload{Text: testText})
	case NotifyDiscord:
		return postJSON(ctx, client, notifyURL, discordPayload{Content: testText})
	case NotifyTelegram:
		chatID, err := parseTelegramChatID(notifyURL)
		if err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		return postJSON(ctx, client, notifyURL, telegramPayload{ChatID: chatID, Text: testText})
	case NotifyEmail:
		if sender == nil {
			return fmt.Errorf("alerting: test notification: email is not configured on this control plane")
		}
		if notifyURL == "" {
			return fmt.Errorf("alerting: test notification: no destination email address configured")
		}
		if err := sender.Send(ctx, notifyURL, "[Levelrail] test notification", testText); err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		return nil
	default: // NotifyGeneric and any unknown/typo'd kind
		return postJSON(ctx, client, notifyURL, map[string]string{"message": testText})
	}
}

// SendTest fires a real test notification through kind/notifyURL,
// reusing this dispatcher's own HTTP client and email sender so a
// passing test predicts a real deploy notification will send the same
// way.
func (d *DeployDispatcher) SendTest(ctx context.Context, kind NotifyKind, notifyURL string) error {
	return sendTestNotification(ctx, d.client, d.sender, kind, notifyURL)
}
