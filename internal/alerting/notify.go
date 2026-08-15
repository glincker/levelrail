package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// Event is what a firing (or resolved) rule hands to a Notifier: enough
// context to write a useful message without the notifier needing to go
// query anything itself.
type Event struct {
	Rule Rule
	// Resolved is true when this event is "the rule stopped firing,"
	// false when it's "the rule started firing." Sending a resolved
	// notification (not just a firing one) is deliberate: an operator
	// who only ever hears about problems starting, never ending, learns
	// to distrust the channel or mute it, exactly the alert-fatigue
	// failure mode a useful alerting feature has to avoid.
	Resolved bool
	// LogLines is populated only for a firing (not resolved) crashloop
	// event: the last up-to-200 lines of the failing container's logs,
	// per TASKS.md 2.7's literal requirement. Nil for threshold rules
	// and for resolved events.
	LogLines []string
}

// Notifier sends one Event somewhere. Every notify* function below
// satisfies this via notifyFunc, so the dispatch table in Dispatch
// stays a plain map, no interface boilerplate per channel.
type Notifier interface {
	Notify(ctx context.Context, ev Event) error
}

type notifyFunc func(ctx context.Context, client *http.Client, url string, ev Event) error

// httpNotifier adapts one notifyFunc plus an HTTP client into a
// Notifier, so Dispatch can pick the right payload shape by
// Rule.NotifyKind without a switch duplicated in every call site.
type httpNotifier struct {
	client *http.Client
	url    string
	build  notifyFunc
}

func (n httpNotifier) Notify(ctx context.Context, ev Event) error {
	return n.build(ctx, n.client, n.url, ev)
}

// SMTPConfig is the server-wide (not per-rule) configuration
// notifyEmail's Notifier needs. Unlike NotifyURL, there is no sensible
// zero-config default for an SMTP server, the same reasoning
// internal/secrets' master key has no default: a control plane that
// hasn't set one gets a clear, documented "not configured" error rather
// than silently trying (and failing) to connect somewhere.
type SMTPConfig struct {
	// Addr is host:port, e.g. "smtp.example.com:587".
	Addr     string
	Host     string // just the host part of Addr, for PlainAuth's identity check
	Username string
	Password string
	From     string
}

// NewNotifier builds the right Notifier for r.NotifyKind. An unknown or
// empty NotifyKind falls back to NotifyGeneric rather than erroring: a
// rule with a typo'd notify_kind should still get *a* notification,
// diagnosable from the payload shape, rather than silently notifying no
// one.
//
// smtpCfg is nil when no SMTP server is configured (the default): a rule
// with NotifyKind == NotifyEmail still gets a Notifier, one whose Notify
// always returns a clear "email is not configured" error rather than a
// nil-pointer panic or a silently dropped notification.
func NewNotifier(client *http.Client, smtpCfg *SMTPConfig, r Rule) Notifier {
	if client == nil {
		client = http.DefaultClient
	}
	if r.NotifyKind == NotifyEmail {
		return emailNotifier{cfg: smtpCfg, to: r.NotifyURL}
	}

	build := notifyGeneric
	switch r.NotifyKind {
	case NotifySlack:
		build = notifySlack
	case NotifyDiscord:
		build = notifyDiscord
	case NotifyTelegram:
		build = notifyTelegram
	}
	return httpNotifier{client: client, url: r.NotifyURL, build: build}
}

// genericPayload is the JSON body notifyGeneric sends: a plain,
// complete machine-readable description of the event, for any webhook
// receiver that isn't Slack or Discord specifically (a custom
// integration, a paging system, a log aggregator).
type genericPayload struct {
	RuleID      string     `json:"rule_id"`
	RuleName    string     `json:"rule_name"`
	Kind        string     `json:"kind"`
	ResourceID  string     `json:"resource_id"`
	Resolved    bool       `json:"resolved"`
	Value       *float64   `json:"value,omitempty"`
	Firing      bool       `json:"firing"`
	FiringSince *time.Time `json:"firing_since,omitempty"`
	LogLines    []string   `json:"log_lines,omitempty"`
}

func notifyGeneric(ctx context.Context, client *http.Client, url string, ev Event) error {
	payload := genericPayload{
		RuleID: ev.Rule.ID, RuleName: ev.Rule.Name, Kind: string(ev.Rule.Kind),
		ResourceID: ev.Rule.ResourceID, Resolved: ev.Resolved, Value: ev.Rule.LastValue,
		Firing: ev.Rule.Firing, FiringSince: ev.Rule.FiringSince, LogLines: ev.LogLines,
	}
	return postJSON(ctx, client, url, payload)
}

// slackPayload is Slack's incoming-webhook shape: a top-level "text"
// field is all a receiver actually requires, blocks/attachments are a
// richer optional layer this pass doesn't build (a real follow-up if a
// nicer-formatted message is wanted later, not required for the
// notification to work).
type slackPayload struct {
	Text string `json:"text"`
}

func notifySlack(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, slackPayload{Text: summaryText(ev)})
}

// discordPayload is Discord's incoming-webhook shape: the equivalent
// top-level "content" field.
type discordPayload struct {
	Content string `json:"content"`
}

func notifyDiscord(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, discordPayload{Content: summaryText(ev)})
}

// telegramPayload is the Telegram Bot API's sendMessage body. chat_id is
// required in the body itself (Telegram does not reliably merge it in
// from a query string), so notifyTelegram below parses it out of
// r.NotifyURL rather than relying on undocumented API behavior.
type telegramPayload struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// notifyTelegram sends via Telegram's Bot API, a plain HTTP POST like
// Slack/Discord above, "not much heavier than the existing webhooks."
// r.NotifyURL is the full sendMessage endpoint including the bot token
// in its path (e.g. "https://api.telegram.org/bot<TOKEN>/sendMessage"),
// exactly as an operator gets it from @BotFather's setup instructions,
// plus one required addition this package expects: a "chat_id" query
// parameter naming the destination chat, which this function extracts
// and moves into the JSON body Telegram actually requires it in.
func notifyTelegram(ctx context.Context, client *http.Client, rawURL string, ev Event) error {
	chatID, err := parseTelegramChatID(rawURL)
	if err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return postJSON(ctx, client, rawURL, telegramPayload{ChatID: chatID, Text: summaryText(ev)})
}

// parseTelegramChatID extracts the chat_id query parameter a Telegram
// notify_url must carry (notifyTelegram's own doc comment explains why
// it lives in the query string rather than the body Telegram actually
// wants it in). Shared by notifyTelegram (alert-rule notifications,
// this file) and deploy_notify.go's own Telegram send (deploy-outcome
// notifications): both need the identical parse, and duplicating it
// would risk the two silently drifting apart on what counts as a valid
// Telegram notify_url.
func parseTelegramChatID(rawURL string) (chatID string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid telegram notify_url: %w", err)
	}
	chatID = u.Query().Get("chat_id")
	if chatID == "" {
		return "", fmt.Errorf("telegram notify_url must include a chat_id query parameter")
	}
	return chatID, nil
}

// summaryText is the human-readable line every simple channel here
// (Slack, Discord, Telegram, email's body) sends: they're all
// fundamentally "one text field," so one summary builder serves all of
// them rather than duplicating this per channel.
func summaryText(ev Event) string {
	var b strings.Builder
	if ev.Resolved {
		fmt.Fprintf(&b, "[RESOLVED] %s (%s) on %s", ev.Rule.Name, ev.Rule.Kind, ev.Rule.ResourceID)
	} else {
		fmt.Fprintf(&b, "[FIRING] %s (%s) on %s", ev.Rule.Name, ev.Rule.Kind, ev.Rule.ResourceID)
	}
	if ev.Rule.LastValue != nil {
		fmt.Fprintf(&b, ", value=%v", *ev.Rule.LastValue)
	}
	if len(ev.LogLines) > 0 {
		fmt.Fprintf(&b, "\nLast %d log lines:\n```\n%s\n```", len(ev.LogLines), strings.Join(ev.LogLines, "\n"))
	}
	return b.String()
}

func postJSON(ctx context.Context, client *http.Client, url string, payload any) error {
	if url == "" {
		return fmt.Errorf("alerting: notify: no notify_url configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("alerting: notify: encode payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("alerting: notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("alerting: notify: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alerting: notify: receiver returned status %d", resp.StatusCode)
	}
	return nil
}

// emailNotifier sends one Event as a plain-text email via net/smtp.
// Unlike every other Notifier here, its transport is SMTP, not HTTP: it
// doesn't fit httpNotifier's notifyFunc shape (a URL plus a JSON body),
// so it implements Notifier directly instead.
type emailNotifier struct {
	cfg *SMTPConfig // nil means "no SMTP server configured"
	// to is r.NotifyURL: not a URL for this channel, the destination
	// email address, the same "NotifyURL is generically 'the
	// destination', interpreted per channel" reasoning notifyTelegram
	// above already establishes for its chat_id.
	to string
}

// Notify implements Notifier. net/smtp.SendMail has no context.Context
// parameter to plumb ctx's cancellation through; a hung SMTP connection
// therefore isn't cancellable the way an HTTP notifier's request is,
// a real, documented limitation, not an oversight.
func (n emailNotifier) Notify(_ context.Context, ev Event) error {
	if n.cfg == nil {
		return fmt.Errorf("alerting: notify: email is not configured on this control plane (set APP_SMTP_HOST, APP_SMTP_FROM, etc.)")
	}
	if n.to == "" {
		return fmt.Errorf("alerting: notify: no notify_url (destination email address) configured")
	}

	subject := fmt.Sprintf("[Levelrail] %s", ev.Rule.Name)
	if ev.Resolved {
		subject = fmt.Sprintf("[Levelrail][RESOLVED] %s", ev.Rule.Name)
	}
	if err := sendPlainEmail(n.cfg, n.to, subject, summaryText(ev)); err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return nil
}

// sendPlainEmail sends a plain-text email via cfg's SMTP server: the raw
// net/smtp mechanics (PlainAuth, message construction, SendMail) shared
// by emailNotifier.Notify above (alert-rule notifications) and
// deploy_notify.go's own email send (deploy-outcome notifications).
// Callers are responsible for their own "not configured" / "no
// destination address" checks before calling this (see
// emailNotifier.Notify above): those error messages are deliberately
// caller-specific, not generalized into this shared helper, so an
// operator debugging a misconfigured alert rule sees the exact wording
// this file already had before that extraction, not a generic one
// blended across both callers.
func sendPlainEmail(cfg *SMTPConfig, to, subject, body string) error {
	msg := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, cfg.From, subject, body)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	if err := smtp.SendMail(cfg.Addr, auth, cfg.From, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}
