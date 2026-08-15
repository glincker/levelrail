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
	"time"
)

// This file is deploy-outcome notifications: a Slack/Discord/Telegram/
// generic-webhook/email ping fired once, synchronously, when a deploy
// attempt (store.DeployAttempt) reaches a terminal status, distinct from
// this package's existing threshold/crashloop alert rules (rules.go,
// evaluate.go, engine.go, crashloop.go) in one load-bearing way: a
// deploy outcome is not evaluated on a tick against stored state, it is
// reported exactly once by whichever of the three real deploy-trigger
// call sites (internal/webhook, internal/api/deploys.go,
// internal/api/builds.go) just finished a deploy_attempts row. There is
// no "firing," "pending," or "resolved" here, only "this one attempt
// just succeeded or failed."
//
// Deliberately kept in this package (not a new internal/deploynotify
// package) and in a dedicated table (not a new alert_rules.kind): see
// migrations/0002_deploy_notify_targets.sql's own doc comment for why a
// separate table, and this file's own functions below for why staying
// in this package lets deploy-outcome sends reuse notify.go's actual
// Slack/Discord/Telegram/generic payload shapes and its postJSON/
// sendPlainEmail/parseTelegramChatID helpers directly, with zero new
// exports and zero duplicated payload code, rather than either forcing
// a deploy outcome through alerting.Event/Rule (whose Kind/Firing/
// LastValue/comparator fields describe a threshold or crashloop
// evaluation, not "did this deploy succeed") or duplicating four HTTP
// payload shapes and an SMTP sender in a separate package for the sake
// of a nominal separation that would buy nothing.

// DeployTarget is a notify destination for deploy-outcome events,
// scoped to one app (ResourceID, matching Rule.ResourceID's own
// "service:<name>" convention) via internal/api's own resourceIDForApp.
// Deliberately much narrower than Rule: no metric/comparator/threshold,
// no restart-count/window, no evaluation state
// (pending_since/firing/firing_since/last_evaluated_at/last_value),
// because none of that describes anything about "where to send a
// deploy's outcome."
type DeployTarget struct {
	ID         string
	ResourceID string
	NotifyURL  string
	NotifyKind NotifyKind
	Enabled    bool
}

// deployTargetIDPrefix mirrors store.deployAttemptIDPrefix's "dep_"
// convention (a short, greppable tag on an otherwise-opaque ID),
// adapted to this table.
const deployTargetIDPrefix = "dnt_"

// NewDeployTargetID generates a random, URL-safe deploy-target
// identifier, the same shape NewRuleID already mints for alert_rules:
// exported so internal/api can assign one when creating a target from a
// request body that doesn't include one, the identical "ID is never
// caller-chosen data" reasoning NewRuleID's own doc comment gives.
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
			id, resource_id, notify_url, notify_kind, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			resource_id = excluded.resource_id,
			notify_url = excluded.notify_url,
			notify_kind = excluded.notify_kind,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`,
		t.ID, t.ResourceID, t.NotifyURL, string(t.NotifyKind), boolToInt(t.Enabled), now, now,
	)
	if err != nil {
		return fmt.Errorf("alerting: save deploy target %q: %w", t.ID, err)
	}
	return nil
}

// ErrDeployTargetNotFound is returned by GetDeployTarget when no target
// has that ID.
var ErrDeployTargetNotFound = errors.New("alerting: deploy target not found")

// GetDeployTarget returns the deploy target with this ID, or
// ErrDeployTargetNotFound.
func (db *DB) GetDeployTarget(ctx context.Context, id string) (*DeployTarget, error) {
	row := db.QueryRowContext(ctx, deployTargetSelectColumns+` FROM deploy_notify_targets WHERE id = ?`, id)
	t, err := scanDeployTarget(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeployTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get deploy target %q: %w", id, err)
	}
	return t, nil
}

// ListDeployTargetsForResource returns every deploy target scoped to
// resourceID, ordered by created_at, regardless of enabled state: the
// same "list everything, let the caller decide what to show or use"
// shape ListRulesForResource already establishes, used both by
// internal/api's CRUD handlers (list including disabled targets, so an
// operator can see and re-enable a paused one) and by Dispatch below
// (which filters to enabled targets itself).
func (db *DB) ListDeployTargetsForResource(ctx context.Context, resourceID string) ([]DeployTarget, error) {
	rows, err := db.QueryContext(ctx, deployTargetSelectColumns+` FROM deploy_notify_targets WHERE resource_id = ? ORDER BY created_at`, resourceID)
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

const deployTargetSelectColumns = `
	SELECT id, resource_id, notify_url, notify_kind, enabled`

func scanDeployTarget(scan func(dest ...any) error) (*DeployTarget, error) {
	var (
		t          DeployTarget
		notifyKind string
		enabledInt int
	)
	if err := scan(&t.ID, &t.ResourceID, &t.NotifyURL, &notifyKind, &enabledInt); err != nil {
		return nil, err
	}
	t.NotifyKind = NotifyKind(notifyKind)
	t.Enabled = enabledInt != 0
	return &t, nil
}

// DeployOutcome is what Dispatch sends: one finished deploy attempt's
// terminal outcome, everything summaryDeployText and deployGenericPayload
// need to describe it. Deliberately not alerting.Event: see this file's
// own package-level doc comment for why forcing this through
// Event/Rule would be the wrong fit.
type DeployOutcome struct {
	// AppName is the deploy's service name (store.DeployAttempt.ServiceName),
	// used both in the message text and to look up which DeployTarget
	// rows apply (resourceIDForApp(AppName) is computed by the caller,
	// internal/api and internal/webhook, the same way it already is for
	// alert rules; this package only ever sees the resulting resource ID).
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
	return msg
}

// deployGenericPayload is the JSON body a generic-webhook DeployTarget
// receives: app name, image/tag, success flag, error detail. Not
// genericPayload (notify.go): the two events describe fundamentally
// different things (an evaluated alert rule's state vs. one finished
// deploy attempt), and forcing them into one JSON shape would mean
// either leaking meaningless firing/threshold/value fields into
// deploy-outcome payloads or meaningless image/succeeded fields into
// alert-rule payloads.
type deployGenericPayload struct {
	AppName   string `json:"app_name"`
	Image     string `json:"image"`
	Succeeded bool   `json:"succeeded"`
	Error     string `json:"error,omitempty"`
}

// sendDeployOutcome sends ev to one target, dispatching on
// t.NotifyKind exactly the way NewNotifier does for a Rule: an unknown
// or empty NotifyKind falls back to the generic JSON shape, the same
// "still notify somebody, diagnosable from the payload shape" reasoning
// NewNotifier's own doc comment gives. Reuses notify.go's slackPayload/
// discordPayload/telegramPayload structs, postJSON, parseTelegramChatID,
// and sendPlainEmail directly (same package, no exports needed): only
// the message text (summaryDeployText) and the generic JSON shape
// (deployGenericPayload) are new.
func sendDeployOutcome(ctx context.Context, client *http.Client, smtpCfg *SMTPConfig, t DeployTarget, ev DeployOutcome) error {
	if client == nil {
		client = http.DefaultClient
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
	case NotifyEmail:
		if smtpCfg == nil {
			return fmt.Errorf("alerting: notify deploy outcome: email is not configured on this control plane (set APP_SMTP_HOST, APP_SMTP_FROM, etc.)")
		}
		if t.NotifyURL == "" {
			return fmt.Errorf("alerting: notify deploy outcome: no notify_url (destination email address) configured")
		}
		subject := fmt.Sprintf("[Levelrail] deploy succeeded: %s", ev.AppName)
		if !ev.Succeeded {
			subject = fmt.Sprintf("[Levelrail] deploy FAILED: %s", ev.AppName)
		}
		if err := sendPlainEmail(smtpCfg, t.NotifyURL, subject, summaryDeployText(ev)); err != nil {
			return fmt.Errorf("alerting: notify deploy outcome: %w", err)
		}
		return nil
	default: // NotifyGeneric and any unknown/typo'd kind
		// deployGenericPayload has the identical field names/types/order
		// as DeployOutcome (only its json struct tags differ, which a
		// struct conversion ignores), so a direct conversion here is both
		// valid and, per staticcheck's S1016, preferred over a literal
		// that just repeats every field name.
		return postJSON(ctx, client, t.NotifyURL, deployGenericPayload(ev))
	}
}

// DeployDispatcher sends a deploy-outcome notification to every enabled
// DeployTarget scoped to one app. Built once in cmd/levelrail/main.go
// (mirroring how alertingNewNotifier is built once and shared by
// alerting.Engine) and shared by reference into internal/webhook and
// internal/api, the three real deploy-trigger call sites this task's own
// task description names.
type DeployDispatcher struct {
	db      *DB
	client  *http.Client
	smtpCfg *SMTPConfig
	logger  *slog.Logger
}

// NewDeployDispatcher builds a DeployDispatcher. client defaults to
// http.DefaultClient if nil (matching NewNotifier's own default);
// smtpCfg may be nil (an email-kind target then always fails with a
// clear "not configured" error, matching NewNotifier's own emailNotifier
// behavior); logger defaults to slog.Default() if nil.
func NewDeployDispatcher(db *DB, client *http.Client, smtpCfg *SMTPConfig, logger *slog.Logger) *DeployDispatcher {
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DeployDispatcher{db: db, client: client, smtpCfg: smtpCfg, logger: logger}
}

// Dispatch sends ev to every enabled DeployTarget scoped to
// resourceIDForApp(ev.AppName)'s resource ID (resourceID, computed by
// the caller: this package has no dependency on internal/api's own
// resourceIDForApp helper, matching how it already has none on that
// package for alert rules). Must only be called once a deploy attempt
// has reached a terminal status (succeeded or failed), never for
// "running": there is nothing sensible to notify about an attempt that
// is still in progress, and none of the three real call sites
// (internal/webhook's beginDeployAttempt finish closure,
// internal/api/deploys.go's recordPlainDeployAttempt,
// internal/api/builds.go's beginBuildDeployAttempt finish closure) ever
// call this before FinishDeployAttempt has already persisted a terminal
// status.
//
// A lookup failure or an individual target's send failure is logged,
// never returned: the deploy attempt this notification describes has
// already finished and already been persisted by the time Dispatch
// runs, so a notification failing to send must never be surfaced as if
// the deploy itself had failed, the identical "must never block the
// real operation" reasoning Engine.dispatch's own doc comment gives one
// layer up for alert rules.
func (d *DeployDispatcher) Dispatch(ctx context.Context, resourceID string, ev DeployOutcome) {
	targets, err := d.db.ListDeployTargetsForResource(ctx, resourceID)
	if err != nil {
		d.logger.Error("alerting: list deploy targets failed",
			slog.String("resource_id", resourceID), slog.String("error", err.Error()))
		return
	}

	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		if err := sendDeployOutcome(ctx, d.client, d.smtpCfg, t, ev); err != nil {
			d.logger.Error("alerting: deploy outcome notification failed",
				slog.String("target_id", t.ID), slog.String("resource_id", resourceID),
				slog.String("app_name", ev.AppName), slog.Bool("succeeded", ev.Succeeded),
				slog.String("error", err.Error()))
		}
	}
}
