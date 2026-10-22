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
	"github.com/GLINCKER/levelrail/kit/netguard"
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
	// Off by default: it needs InteractiveSecret and a real Slack app or Discord application, not just a webhook URL.
	InteractiveApprovals bool
	// Verifies inbound button clicks came from this channel's app: a Slack signing secret or a Discord Ed25519 public key.
	InteractiveSecret string
	// NotifyDeviceLogin opts this channel into a link-only notice when a CLI
	// device login awaits approval. The notice never carries the code.
	NotifyDeviceLogin bool
	// NotifyDeviceLoginExpired opts this channel into a link-only notice when
	// a pending CLI login lapses unapproved.
	NotifyDeviceLoginExpired bool
	CreatedAt                string
	UpdatedAt                string
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

// SaveNotificationChannel creates or replaces a channel's configuration; secrets are sealed and a masked value keeps the stored one.
func (db *DB) SaveNotificationChannel(ctx context.Context, c NotificationChannel) error {
	if cs := db.secretsOrNil(); cs != nil {
		urlCol, secretCol, err := db.sealChannel(ctx, cs, c)
		if err != nil {
			return err
		}
		c.NotifyURL, c.InteractiveSecret = urlCol, secretCol
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO notification_channels (
			id, name, kind, notify_url, enabled,
			interactive_approvals, interactive_secret, notify_device_login, notify_device_login_expired, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			notify_url = excluded.notify_url,
			enabled = excluded.enabled,
			interactive_approvals = excluded.interactive_approvals,
			interactive_secret = excluded.interactive_secret,
			notify_device_login = excluded.notify_device_login,
			notify_device_login_expired = excluded.notify_device_login_expired,
			updated_at = excluded.updated_at
	`, c.ID, c.Name, string(c.Kind), c.NotifyURL, boolToInt(c.Enabled),
		boolToInt(c.InteractiveApprovals), c.InteractiveSecret, boolToInt(c.NotifyDeviceLogin), boolToInt(c.NotifyDeviceLoginExpired), now, now)
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
	if cs := db.secretsOrNil(); cs != nil {
		if err := cs.DeleteAll(ctx, NotificationChannelSecretsKey(id)); err != nil {
			return fmt.Errorf("alerting: delete sealed secrets for channel %q: %w", id, err)
		}
	}
	return nil
}

const notificationChannelSelectColumns = `
	SELECT id, name, kind, notify_url, enabled,
		interactive_approvals, interactive_secret, notify_device_login, notify_device_login_expired, created_at, updated_at
	FROM notification_channels`

func scanNotificationChannel(scan func(dest ...any) error) (*NotificationChannel, error) {
	var (
		c                NotificationChannel
		kind             string
		enabledInt       int
		interactiveInt   int
		deviceLoginInt   int
		deviceExpiredInt int
	)
	if err := scan(&c.ID, &c.Name, &kind, &c.NotifyURL, &enabledInt,
		&interactiveInt, &c.InteractiveSecret, &deviceLoginInt, &deviceExpiredInt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Kind = NotifyKind(kind)
	c.Enabled = enabledInt != 0
	c.InteractiveApprovals = interactiveInt != 0
	c.NotifyDeviceLogin = deviceLoginInt != 0
	c.NotifyDeviceLoginExpired = deviceExpiredInt != 0
	return &c, nil
}

// sendTestNotification sends a fixed connectivity-check message via
// kind, reusing sendDeployOutcome's own per-channel payload logic.
func sendTestNotification(ctx context.Context, client *http.Client, sender email.Sender, pushSender PushSender, kind NotifyKind, notifyURL string) error {
	return sendTextNotification(ctx, client, sender, pushSender, kind, notifyURL, "Levelrail test notification. If you can see this, the connection works.")
}

func sendTextNotification(ctx context.Context, client *http.Client, sender email.Sender, pushSender PushSender, kind NotifyKind, notifyURL, testText string) error {
	if IsPluginKind(kind) {
		return ErrPluginChannelUnsupported
	}
	if IsMaskedNotifyTarget(notifyURL) {
		return ErrChannelSecretNotRevealed
	}
	if client == nil {
		client = netguard.NewClient()
	}

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
	case NotifyPushover:
		token, user, err := parsePushoverCreds(notifyURL)
		if err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		return postJSON(ctx, client, notifyURL, pushoverPayload{Token: token, User: user, Message: testText})
	case NotifyPagerDuty:
		if notifyURL == "" {
			return fmt.Errorf("alerting: test notification: no notify_url (pagerduty routing key) configured")
		}
		return postJSON(ctx, client, pagerDutyEventsURL, pagerDutyPayload{
			RoutingKey: notifyURL, EventAction: "trigger",
			Payload: pagerDutyDetails{Summary: testText, Source: "notification-test", Severity: "info"},
		})
	case NotifyTeams:
		return postJSON(ctx, client, notifyURL, teamsPayload{
			Type: "MessageCard", Context: "http://schema.org/extensions", Summary: testText, Text: testText,
		})
	case NotifyMattermost:
		return postJSON(ctx, client, notifyURL, mattermostPayload{Text: testText})
	case NotifyLark:
		return postJSON(ctx, client, notifyURL, larkPayload{MsgType: "text", Content: larkContent{Text: testText}})
	case NotifyRocketChat:
		return postJSON(ctx, client, notifyURL, rocketChatPayload{Text: testText, Alias: "Levelrail", Emoji: ":white_check_mark:"})
	case NotifyWebex:
		return postJSON(ctx, client, notifyURL, webexPayload{Markdown: testText})
	case NotifyGoogleChat:
		return postJSON(ctx, client, notifyURL, googleChatPayload{Text: testText})
	case NotifyOpsgenie:
		key, err := parseOpsgenieCreds(notifyURL)
		if err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		payload := opsgeniePayload{Message: "Levelrail test notification", Description: testText, Priority: "P5"}
		return postJSONWithAuth(ctx, client, opsgenieAPIURL, payload, "GenieKey "+key)
	case NotifyGotify:
		return postJSON(ctx, client, notifyURL, gotifyPayload{Title: "Levelrail", Message: testText, Priority: 5})
	case NotifyNtfy:
		token, cleanURL, err := extractNtfyToken(notifyURL)
		if err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		payload := ntfyPayload{Title: "Levelrail", Message: testText, Priority: 4}
		return postNtfy(ctx, client, cleanURL, token, payload)
	case NotifyResend:
		key, to, from, err := parseResendCreds(notifyURL)
		if err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		payload := resendPayload{From: from, To: []string{to}, Subject: "[Levelrail] test notification", Text: testText}
		return postJSONWithAuth(ctx, client, resendAPIURL, payload, bearerPrefix+key)
	case NotifyEmail:
		if sender == nil {
			return fmt.Errorf("alerting: test notification: email is not configured on this control plane")
		}
		if notifyURL == "" {
			return fmt.Errorf("alerting: test notification: no destination email address configured")
		}
		if err := sendEmailWithRetry(ctx, sender, notifyURL, "[Levelrail] test notification", testText); err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		return nil
	case NotifyWebpush:
		if pushSender == nil {
			return fmt.Errorf("alerting: test notification: browser push is not configured on this control plane")
		}
		if err := pushSender.Send(ctx, "Levelrail", testText); err != nil {
			return fmt.Errorf("alerting: test notification: %w", err)
		}
		return nil
	default: // NotifyGeneric and any unknown/typo'd kind
		return postJSON(ctx, client, notifyURL, map[string]string{"message": testText})
	}
}

// SendTest reuses the dispatcher's HTTP client and email sender, so a passing test predicts a real send.
func (d *DeployDispatcher) SendTest(ctx context.Context, kind NotifyKind, notifyURL string) error {
	return sendTestNotification(ctx, d.client, d.sender, d.pushSender, kind, notifyURL)
}

// SendChannelTest fires a test notification through a saved channel,
// decrypting its credential only for this send.
func (d *DeployDispatcher) SendChannelTest(ctx context.Context, c NotificationChannel) error {
	revealed, err := d.db.RevealNotificationChannel(ctx, c)
	if err != nil {
		return err
	}
	return sendTestNotification(ctx, d.client, d.sender, d.pushSender, revealed.Kind, revealed.NotifyURL)
}

// SendNotice sends a plain-text notice through channel c, using the same
// per-kind payload logic and SSRF-guarded client as SendTest.
func (d *DeployDispatcher) SendNotice(ctx context.Context, c NotificationChannel, text string) error {
	revealed, err := d.db.RevealNotificationChannel(ctx, c)
	if err != nil {
		return err
	}
	return sendTextNotification(ctx, d.client, d.sender, d.pushSender, revealed.Kind, revealed.NotifyURL, text)
}
