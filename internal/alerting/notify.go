package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
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

// NewNotifier builds the right Notifier for r.NotifyKind. An unknown or
// empty NotifyKind falls back to NotifyGeneric rather than erroring, so a
// typo'd notify_kind still notifies someone, diagnosable from the
// payload shape. sender may be nil, in which case an email-kind rule
// fails with a clear "not configured" error.
func NewNotifier(client *http.Client, sender email.Sender, r Rule) Notifier {
	if client == nil {
		client = http.DefaultClient
	}
	if r.NotifyKind == NotifyEmail {
		return emailNotifier{sender: sender, to: r.NotifyURL}
	}

	build := notifyGeneric
	switch r.NotifyKind {
	case NotifySlack:
		build = notifySlack
	case NotifyDiscord:
		build = notifyDiscord
	case NotifyTelegram:
		build = notifyTelegram
	case NotifyPushover:
		build = notifyPushover
	case NotifyPagerDuty:
		build = notifyPagerDuty
	case NotifyTeams:
		build = notifyTeams
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

// pushoverPayload is the Pushover Message API's request body
// (https://pushover.net/api). Title is omitted when empty, so a plain
// message still sends without an empty title line.
type pushoverPayload struct {
	Token   string `json:"token"`
	User    string `json:"user"`
	Message string `json:"message"`
	Title   string `json:"title,omitempty"`
}

// notifyPushover parses token/user out of rawURL's query string, the
// same convention notifyTelegram uses for chat_id, then posts to rawURL
// (Pushover's fixed messages.json endpoint, carrying those two query
// params) exactly as notifyTelegram posts to its own rawURL.
func notifyPushover(ctx context.Context, client *http.Client, rawURL string, ev Event) error {
	token, user, err := parsePushoverCreds(rawURL)
	if err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return postJSON(ctx, client, rawURL, pushoverPayload{Token: token, User: user, Message: summaryText(ev), Title: ev.Rule.Name})
}

// parsePushoverCreds extracts the token and user query parameters a
// Pushover notify_url must carry (see notifyPushover's own doc comment).
func parsePushoverCreds(rawURL string) (token, user string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid pushover notify_url: %w", err)
	}
	token = u.Query().Get("token")
	user = u.Query().Get("user")
	if token == "" || user == "" {
		return "", "", fmt.Errorf("pushover notify_url must include token and user query parameters")
	}
	return token, user, nil
}

// pagerDutyEventsURL is PagerDuty's fixed Events API v2 endpoint. A var,
// not a const, so tests can point it at an httptest server.
var pagerDutyEventsURL = "https://events.pagerduty.com/v2/enqueue"

// pagerDutyPayload is the Events API v2 enqueue body
// (https://developer.pagerduty.com/api-reference/reference/events-v2/openapiv3.json).
type pagerDutyPayload struct {
	RoutingKey  string           `json:"routing_key"`
	EventAction string           `json:"event_action"`
	Payload     pagerDutyDetails `json:"payload"`
}

type pagerDutyDetails struct {
	Summary  string `json:"summary"`
	Source   string `json:"source"`
	Severity string `json:"severity"`
}

// notifyPagerDuty always posts to pagerDutyEventsURL, not rawURL:
// rawURL here is the channel's Integration/Routing Key, not a webhook
// URL, the same "NotifyURL is generically 'the destination'" convention
// notifyTelegram and notifyPushover already establish for their own
// non-URL query parameters.
func notifyPagerDuty(ctx context.Context, client *http.Client, rawURL string, ev Event) error {
	if rawURL == "" {
		return fmt.Errorf("alerting: notify: no notify_url (pagerduty routing key) configured")
	}
	severity := "critical"
	if ev.Resolved {
		severity = "info"
	}
	payload := pagerDutyPayload{
		RoutingKey:  rawURL,
		EventAction: "trigger",
		Payload: pagerDutyDetails{
			Summary:  summaryText(ev),
			Source:   ev.Rule.ResourceID,
			Severity: severity,
		},
	}
	return postJSON(ctx, client, pagerDutyEventsURL, payload)
}

// teamsPayload is a Microsoft Teams incoming-webhook connector's
// MessageCard body (https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/connectors-using).
type teamsPayload struct {
	Type    string `json:"@type"`
	Context string `json:"@context"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

func notifyTeams(ctx context.Context, client *http.Client, url string, ev Event) error {
	text := summaryText(ev)
	return postJSON(ctx, client, url, teamsPayload{
		Type: "MessageCard", Context: "http://schema.org/extensions", Summary: text, Text: text,
	})
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

// emailNotifier sends one Event as a plain-text email via an
// email.Sender. Unlike every other Notifier here, its transport isn't
// HTTP: it doesn't fit httpNotifier's notifyFunc shape (a URL plus a
// JSON body), so it implements Notifier directly instead.
type emailNotifier struct {
	sender email.Sender // nil means "no email capability configured"
	// to is r.NotifyURL: not a URL for this channel, the destination
	// email address, the same "NotifyURL is generically 'the
	// destination', interpreted per channel" reasoning notifyTelegram
	// above already establishes for its chat_id.
	to string
}

func (n emailNotifier) Notify(ctx context.Context, ev Event) error {
	if n.sender == nil {
		return fmt.Errorf("alerting: notify: email is not configured on this control plane")
	}
	if n.to == "" {
		return fmt.Errorf("alerting: notify: no notify_url (destination email address) configured")
	}

	subject := fmt.Sprintf("[Levelrail] %s", ev.Rule.Name)
	if ev.Resolved {
		subject = fmt.Sprintf("[Levelrail][RESOLVED] %s", ev.Rule.Name)
	}
	if err := n.sender.Send(ctx, n.to, subject, summaryText(ev)); err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return nil
}
