package alerting

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Deploy outcomes are one-shot pings, not firing or resolved alerts, so they live beside rules rather than in them.

// DeployTarget is a deploy-outcome notify destination scoped to one app.
type DeployTarget struct {
	ID         string
	ResourceID string
	// ChannelID attaches an already-connected NotificationChannel; empty
	// for legacy rows, which use NotifyURL/NotifyKind below directly.
	ChannelID  string
	NotifyURL  string
	NotifyKind NotifyKind
	Enabled    bool
}

// deployTargetIDPrefix mirrors store.deployAttemptIDPrefix's "dep_"
// convention (a short, greppable tag on an otherwise-opaque ID),
// adapted to this table.
const deployTargetIDPrefix = "dnt_"

// NewDeployTargetID generates a random, URL-safe deploy target identifier.
func NewDeployTargetID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate deploy target id: %w", err)
	}
	return deployTargetIDPrefix + hex.EncodeToString(b), nil
}

// SaveDeployTarget creates or fully replaces a deploy target's
// configuration, the same upsert shape SaveRule already uses.
func (db *DB) SaveDeployTarget(ctx context.Context, t DeployTarget) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_notify_targets (
			id, resource_id, channel_id, notify_url, notify_kind, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			resource_id = excluded.resource_id,
			channel_id = excluded.channel_id,
			notify_url = excluded.notify_url,
			notify_kind = excluded.notify_kind,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`,
		t.ID, t.ResourceID, nullIfEmpty(t.ChannelID), t.NotifyURL, string(t.NotifyKind), boolToInt(t.Enabled), now, now,
	)
	if err != nil {
		return fmt.Errorf("alerting: save deploy target %q: %w", t.ID, err)
	}
	return nil
}

// nullIfEmpty converts "" to a real SQL NULL: channel_id is a nullable
// FK, and an empty string would fail the constraint instead of meaning
// "no channel attached" (sql.ErrNoRows for FK id "" never exists).
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ErrDeployTargetNotFound is returned by GetDeployTarget when no target
// has that ID.
var ErrDeployTargetNotFound = errors.New("alerting: deploy target not found")

// GetDeployTarget returns the deploy target with this ID, or
// ErrDeployTargetNotFound.
func (db *DB) GetDeployTarget(ctx context.Context, id string) (*DeployTarget, error) {
	row := db.QueryRowContext(ctx, deployTargetSelectColumns+` WHERE t.id = ?`, id)
	t, err := scanDeployTarget(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeployTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get deploy target %q: %w", id, err)
	}
	return t, nil
}

// ListDeployTargetsForResource returns every deploy target for resourceID, enabled or not.
func (db *DB) ListDeployTargetsForResource(ctx context.Context, resourceID string) ([]DeployTarget, error) {
	rows, err := db.QueryContext(ctx, deployTargetSelectColumns+` WHERE t.resource_id = ? ORDER BY t.created_at`, resourceID)
	if err != nil {
		return nil, fmt.Errorf("alerting: list deploy targets for resource %q: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []DeployTarget
	for rows.Next() {
		t, err := scanDeployTarget(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan deploy target row: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate deploy target rows: %w", err)
	}
	return out, nil
}

// DeleteDeployTarget removes a deploy target by ID. Deleting a target
// that doesn't exist is not an error, the same idempotent-delete
// convention DeleteRule already follows.
func (db *DB) DeleteDeployTarget(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_notify_targets WHERE id = ?`, id); err != nil {
		return fmt.Errorf("alerting: delete deploy target %q: %w", id, err)
	}
	return nil
}

// deployTargetSelectColumns resolves the *effective* notify_url/
// notify_kind via COALESCE: the attached channel's values when set,
// this row's own columns otherwise (legacy rows, or a deleted channel).
const deployTargetSelectColumns = `
	SELECT t.id, t.resource_id, t.channel_id,
		COALESCE(c.notify_url, t.notify_url), COALESCE(c.kind, t.notify_kind),
		t.enabled, c.enabled
	FROM deploy_notify_targets t
	LEFT JOIN notification_channels c ON c.id = t.channel_id`

func scanDeployTarget(scan func(dest ...any) error) (*DeployTarget, error) {
	var (
		t              DeployTarget
		channelID      sql.NullString
		notifyKind     string
		enabledInt     int
		channelEnabled sql.NullInt64
	)
	if err := scan(&t.ID, &t.ResourceID, &channelID, &t.NotifyURL, &notifyKind, &enabledInt, &channelEnabled); err != nil {
		return nil, err
	}
	t.ChannelID = channelID.String
	t.NotifyKind = NotifyKind(notifyKind)
	// A disabled channel silences every target attached to it too, not
	// just ones explicitly disabled themselves.
	t.Enabled = enabledInt != 0 && (!channelEnabled.Valid || channelEnabled.Int64 != 0)
	return &t, nil
}

// DeployOutcome is one finished deploy attempt's terminal result, as sent by Dispatch.
type DeployOutcome struct {
	// AppName is the service name; the caller computes the resource ID used for lookup.
	AppName string
	// Image is the tag this attempt deployed or tried to deploy
	// (store.DeployAttempt.Image).
	Image string
	// Succeeded is store.DeployAttempt.Status == succeeded. Never called
	// for the non-terminal "running" status: see Dispatch's own doc
	// comment.
	Succeeded bool
	// Error is store.DeployAttempt.Error: only meaningful, and only ever
	// non-empty, when !Succeeded.
	Error string
	// AttemptID is the deploy attempt, used to read a failed build's output.
	AttemptID string
	// LogLines is the tail of the failing build or container output. The
	// dispatcher fills it for a failed deploy when a log source is set.
	LogLines []string
}

// deployLogExcerptLines is how many trailing lines a text channel shows.
const deployLogExcerptLines = 10

// deployLogExcerptBytes caps the excerpt so chat channel limits hold.
const deployLogExcerptBytes = 1200

// deployLogLines is how many lines the dispatcher attaches to a failed deploy.
const deployLogLines = 200

// deployLogLookback bounds the runtime log window used when there is no build log.
const deployLogLookback = 15 * time.Minute

func deployLogExcerpt(lines []string) string {
	if len(lines) > deployLogExcerptLines {
		lines = lines[len(lines)-deployLogExcerptLines:]
	}
	out := strings.Join(lines, "\n")
	if len(out) > deployLogExcerptBytes {
		out = "..." + out[len(out)-deployLogExcerptBytes:]
	}
	return out
}

// summaryDeployText is the human-readable line every simple channel
// (Slack, Discord, Telegram, email's body) sends for a deploy outcome,
// the deploy-outcome equivalent of summaryText for alert-rule events.
func summaryDeployText(ev DeployOutcome) string {
	if ev.Succeeded {
		return fmt.Sprintf("[DEPLOY SUCCEEDED] %s -> %s", ev.AppName, ev.Image)
	}
	msg := fmt.Sprintf("[DEPLOY FAILED] %s -> %s", ev.AppName, ev.Image)
	if ev.Error != "" {
		msg += ": " + ev.Error
	}
	if len(ev.LogLines) > 0 {
		msg += "\nLast log lines:\n" + deployLogExcerpt(ev.LogLines)
	}
	return msg
}

// Separate from genericPayload: alert and deploy fields would otherwise leak into each other's JSON.
type deployGenericPayload struct {
	AppName   string `json:"app_name"`
	Image     string `json:"image"`
	Succeeded bool   `json:"succeeded"`
	Error     string `json:"error,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`
	// LogLines is the last lines of the failing build or container.
	LogLines []string `json:"log_lines,omitempty"`
}

// sendDeployOutcome sends ev to one target, dispatching on t.NotifyKind.
// An unknown or empty NotifyKind falls back to the generic JSON shape so the notification still goes out.
// so a notification still goes out somewhere.
func sendDeployOutcome(ctx context.Context, client *http.Client, sender email.Sender, pushSender PushSender, t DeployTarget, ev DeployOutcome) error {
	if IsPluginKind(t.NotifyKind) {
		return ErrPluginChannelUnsupported
	}
	if IsMaskedNotifyTarget(t.NotifyURL) {
		return ErrChannelSecretNotRevealed
	}
	if client == nil {
		client = netguard.NewClient()
	}

	switch t.NotifyKind {
	case NotifySlack:
		return postJSON(ctx, client, t.NotifyURL, slackPayload{Text: summaryDeployText(ev)})
	case NotifyDiscord:
		return postJSON(ctx, client, t.NotifyURL, discordPayload{Content: summaryDeployText(ev)})
	case NotifyTelegram:
		chatID, err := parseTelegramChatID(t.NotifyURL)
		if err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		return postJSON(ctx, client, t.NotifyURL, telegramPayload{ChatID: chatID, Text: summaryDeployText(ev)})
	case NotifyPushover:
		token, user, err := parsePushoverCreds(t.NotifyURL)
		if err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		return postJSON(ctx, client, t.NotifyURL, pushoverPayload{Token: token, User: user, Message: summaryDeployText(ev), Title: ev.AppName})
	case NotifyPagerDuty:
		if t.NotifyURL == "" {
			return fmt.Errorf("alerting: notify deploy outcome: no notify_url (pagerduty routing key) configured")
		}
		severity := "critical"
		if ev.Succeeded {
			severity = "info"
		}
		return postJSON(ctx, client, pagerDutyEventsURL, pagerDutyPayload{
			RoutingKey: t.NotifyURL, EventAction: "trigger",
			Payload: pagerDutyDetails{Summary: summaryDeployText(ev), Source: ev.AppName, Severity: severity},
		})
	case NotifyTeams:
		text := summaryDeployText(ev)
		return postJSON(ctx, client, t.NotifyURL, teamsPayload{
			Type: "MessageCard", Context: "http://schema.org/extensions", Summary: text, Text: text,
		})
	case NotifyMattermost:
		return postJSON(ctx, client, t.NotifyURL, mattermostPayload{Text: summaryDeployText(ev)})
	case NotifyLark:
		return postJSON(ctx, client, t.NotifyURL, larkPayload{MsgType: "text", Content: larkContent{Text: summaryDeployText(ev)}})
	case NotifyRocketChat:
		emoji := ":rotating_light:"
		if ev.Succeeded {
			emoji = ":white_check_mark:"
		}
		return postJSON(ctx, client, t.NotifyURL, rocketChatPayload{Text: summaryDeployText(ev), Alias: "Levelrail", Emoji: emoji})
	case NotifyWebex:
		return postJSON(ctx, client, t.NotifyURL, webexPayload{Markdown: summaryDeployText(ev)})
	case NotifyGoogleChat:
		return postJSON(ctx, client, t.NotifyURL, googleChatPayload{Text: summaryDeployText(ev)})
	case NotifyOpsgenie:
		key, err := parseOpsgenieCreds(t.NotifyURL)
		if err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		priority := "P1"
		if ev.Succeeded {
			priority = "P5"
		}
		payload := opsgeniePayload{Message: fmt.Sprintf("deploy: %s", ev.AppName), Description: summaryDeployText(ev), Priority: priority}
		return postJSONWithAuth(ctx, client, opsgenieAPIURL, payload, "GenieKey "+key)
	case NotifyGotify:
		priority := 5
		if ev.Succeeded {
			priority = 2
		}
		return postJSON(ctx, client, t.NotifyURL, gotifyPayload{Title: ev.AppName, Message: summaryDeployText(ev), Priority: priority})
	case NotifyNtfy:
		token, cleanURL, err := extractNtfyToken(t.NotifyURL)
		if err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		priority := 4
		if ev.Succeeded {
			priority = 3
		}
		payload := ntfyPayload{Title: ev.AppName, Message: summaryDeployText(ev), Priority: priority}
		if token == "" {
			return postJSON(ctx, client, cleanURL, payload)
		}
		return postJSONWithAuth(ctx, client, cleanURL, payload, bearerPrefix+token)
	case NotifyResend:
		key, to, from, err := parseResendCreds(t.NotifyURL)
		if err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		subject := fmt.Sprintf("[Levelrail] deploy succeeded: %s", ev.AppName)
		if !ev.Succeeded {
			subject = fmt.Sprintf("[Levelrail] deploy FAILED: %s", ev.AppName)
		}
		payload := resendPayload{From: from, To: []string{to}, Subject: subject, Text: summaryDeployText(ev)}
		return postJSONWithAuth(ctx, client, resendAPIURL, payload, bearerPrefix+key)
	case NotifyEmail:
		if sender == nil {
			return fmt.Errorf("alerting: notify deploy outcome: email is not configured on this control plane")
		}
		if t.NotifyURL == "" {
			return fmt.Errorf("alerting: notify deploy outcome: no notify_url (destination email address) configured")
		}
		subject := fmt.Sprintf("[Levelrail] deploy succeeded: %s", ev.AppName)
		if !ev.Succeeded {
			subject = fmt.Sprintf("[Levelrail] deploy FAILED: %s", ev.AppName)
		}
		status := "succeeded"
		if !ev.Succeeded {
			status = "failed"
		}
		vals := map[string]string{"app_name": ev.AppName, "deploy_status": status}
		if err := sendTemplated(ctx, sender, mailtemplates.EventDeployNotification, t.NotifyURL, vals, subject, summaryDeployText(ev)); err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		return nil
	case NotifyWebpush:
		if pushSender == nil {
			return fmt.Errorf("alerting: notify deploy outcome: browser push is not configured on this control plane")
		}
		title := fmt.Sprintf("Deploy succeeded: %s", ev.AppName)
		if !ev.Succeeded {
			title = fmt.Sprintf("Deploy FAILED: %s", ev.AppName)
		}
		if err := pushSender.Send(ctx, title, summaryDeployText(ev)); err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		return nil
	default: // NotifyGeneric and any unknown/typo'd kind
		// Identical field layout, so a direct conversion is valid and preferred over a field-by-field literal.
		return postJSON(ctx, client, t.NotifyURL, deployGenericPayload(ev))
	}
}

// DeployDispatcher sends deploy-outcome notifications to the enabled targets of one app.
type DeployDispatcher struct {
	db         *DB
	client     *http.Client
	sender     email.Sender
	pushSender PushSender
	logger     *slog.Logger
	runtime    LogsSource
	buildLogs  DeployLogSource
	outcomes   *DeployOutcomeTracker
}

// DeployLogSource reads a deploy attempt's build output; *telemetry.DB satisfies it.
type DeployLogSource interface {
	QueryDeployLog(ctx context.Context, attemptID string) ([]telemetry.DeployLogEntry, error)
}

// WithOutcomeTracker feeds every terminal deploy outcome to t, which the deploy_failed rule reads.
func (d *DeployDispatcher) WithOutcomeTracker(t *DeployOutcomeTracker) *DeployDispatcher {
	d.outcomes = t
	return d
}

// Outcomes returns the tracker fed by this dispatcher, or nil when none is attached.
func (d *DeployDispatcher) Outcomes() DeployFailureSource {
	if d.outcomes == nil {
		return nil
	}
	return d.outcomes
}

// WithLogSources lets the dispatcher attach the tail of the failing build or
// container output to a failed deploy notification. Either may be nil.
func (d *DeployDispatcher) WithLogSources(runtime LogsSource, build DeployLogSource) *DeployDispatcher {
	d.runtime, d.buildLogs = runtime, build
	return d
}

func (d *DeployDispatcher) attachLogs(ctx context.Context, resourceID string, ev *DeployOutcome) {
	if ev.Succeeded || len(ev.LogLines) > 0 {
		return
	}
	var lines []string
	if ev.AttemptID != "" && d.buildLogs != nil {
		if entries, err := d.buildLogs.QueryDeployLog(ctx, ev.AttemptID); err == nil {
			for _, e := range entries {
				lines = append(lines, e.Message)
			}
		}
	}
	if len(lines) == 0 && d.runtime != nil {
		now := time.Now()
		entries, err := d.runtime.QueryLogs(ctx, resourceID, now.Add(-deployLogLookback), now, "")
		if err != nil {
			d.logger.Warn("alerting: deploy notification log lookup failed",
				slog.String("resource_id", resourceID), slog.String("error", err.Error()))
		}
		for _, e := range entries {
			lines = append(lines, e.Message)
		}
	}
	if len(lines) > deployLogLines {
		lines = lines[len(lines)-deployLogLines:]
	}
	ev.LogLines = lines
}

// NewDeployDispatcher builds a DeployDispatcher; nil senders fail sends with a 'not configured' error.
func NewDeployDispatcher(db *DB, client *http.Client, sender email.Sender, pushSender PushSender, logger *slog.Logger) *DeployDispatcher {
	if client == nil {
		client = netguard.NewClient()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DeployDispatcher{db: db, client: client, sender: sender, pushSender: pushSender, logger: logger}
}

// Dispatch sends ev to every enabled DeployTarget for its app. Call it only once the attempt is terminal.
// Failures are logged, never returned: the deploy has already finished and must not look failed.
func (d *DeployDispatcher) Dispatch(ctx context.Context, resourceID string, ev DeployOutcome) {
	if d.outcomes != nil {
		d.outcomes.Record(ev, time.Now())
	}
	d.attachLogs(ctx, resourceID, &ev)
	targets, err := d.db.ListDeployTargetsForResource(ctx, resourceID)
	if err != nil {
		d.logger.Error("alerting: list deploy targets failed",
			slog.String("resource_id", resourceID), slog.String("error", err.Error()))
		return
	}

	trigger := "deploy-succeeded"
	if !ev.Succeeded {
		trigger = "deploy-failed"
	}

	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		target, sendErr := d.db.RevealChannelTarget(ctx, t.ChannelID, t.NotifyURL)
		if sendErr == nil {
			t.NotifyURL = target
			sendErr = sendDeployOutcome(ctx, d.client, d.sender, d.pushSender, t, ev)
		}
		if sendErr != nil {
			d.logger.Error("alerting: deploy outcome notification failed",
				slog.String("target_id", t.ID), slog.String("resource_id", resourceID),
				slog.String("app_name", ev.AppName), slog.Bool("succeeded", ev.Succeeded),
				slog.String("error", sendErr.Error()))
		}
		recordDelivery(ctx, d.db, d.logger, t.ChannelID, trigger, sendErr)
	}
}
