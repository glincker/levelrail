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
	// CertNotices is populated only for a firing (not resolved)
	// cert_expiry event: one line per non-healthy certificate, from
	// EvaluateCertExpiry, flagging a stalled-looking renewal specially.
	// Nil for every other rule kind and for resolved events.
	CertNotices []string
	// PatchNotices is populated only for a firing (not resolved)
	// patch_status event: one line per node over its security-patch
	// threshold, from EvaluatePatchStatus. Nil for every other rule kind
	// and for resolved events.
	PatchNotices []string
	// DiskSpaceNotices is populated only for a firing (not resolved)
	// node_disk_space event: one line per node over its disk-usage
	// threshold, from EvaluateNodeDiskSpace. Nil for every other rule
	// kind and for resolved events.
	DiskSpaceNotices []string
	// ResourceUsageNotices is populated only for a firing (not resolved)
	// node_resource_usage event: one line per node over its CPU and/or
	// memory threshold, from EvaluateNodeResourceUsage. Nil for every
	// other rule kind and for resolved events.
	ResourceUsageNotices []string
	// TaskFailureNotice is populated only for a firing (not resolved)
	// scheduled_task_failure event: the failing task's command,
	// consecutive-failure count, and last status, from
	// EvaluateScheduledTaskFailure. Empty for every other rule kind and
	// for resolved events.
	TaskFailureNotice string
	// DomainHealthNotices is populated only for a firing (not resolved)
	// domain_health event: one line per unhealthy domain on this rule's
	// own app, from EvaluateDomainHealth. Nil for every other rule kind
	// and for resolved events.
	DomainHealthNotices []string
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
	case NotifyResend:
		build = notifyResend
	case NotifyNtfy:
		build = notifyNtfy
	case NotifyGotify:
		build = notifyGotify
	case NotifyMattermost:
		build = notifyMattermost
	case NotifyLark:
		build = notifyLark
	case NotifyRocketChat:
		build = notifyRocketChat
	case NotifyOpsgenie:
		build = notifyOpsgenie
	case NotifyWebex:
		build = notifyWebex
	case NotifyGoogleChat:
		build = notifyGoogleChat
	}
	return httpNotifier{client: client, url: r.NotifyURL, build: build}
}

// genericPayload is the JSON body notifyGeneric sends: a plain,
// complete machine-readable description of the event, for any webhook
// receiver that isn't Slack or Discord specifically (a custom
// integration, a paging system, a log aggregator).
type genericPayload struct {
	RuleID               string     `json:"rule_id"`
	RuleName             string     `json:"rule_name"`
	Kind                 string     `json:"kind"`
	ResourceID           string     `json:"resource_id"`
	Resolved             bool       `json:"resolved"`
	Value                *float64   `json:"value,omitempty"`
	Firing               bool       `json:"firing"`
	FiringSince          *time.Time `json:"firing_since,omitempty"`
	LogLines             []string   `json:"log_lines,omitempty"`
	CertNotices          []string   `json:"cert_notices,omitempty"`
	PatchNotices         []string   `json:"patch_notices,omitempty"`
	DiskSpaceNotices     []string   `json:"disk_space_notices,omitempty"`
	ResourceUsageNotices []string   `json:"resource_usage_notices,omitempty"`
	TaskFailureNotice    string     `json:"task_failure_notice,omitempty"`
	DomainHealthNotices  []string   `json:"domain_health_notices,omitempty"`
}

func notifyGeneric(ctx context.Context, client *http.Client, url string, ev Event) error {
	payload := genericPayload{
		RuleID: ev.Rule.ID, RuleName: ev.Rule.Name, Kind: string(ev.Rule.Kind),
		ResourceID: ev.Rule.ResourceID, Resolved: ev.Resolved, Value: ev.Rule.LastValue,
		Firing: ev.Rule.Firing, FiringSince: ev.Rule.FiringSince, LogLines: ev.LogLines,
		CertNotices: ev.CertNotices, PatchNotices: ev.PatchNotices, DiskSpaceNotices: ev.DiskSpaceNotices,
		ResourceUsageNotices: ev.ResourceUsageNotices,
		TaskFailureNotice:    ev.TaskFailureNotice,
		DomainHealthNotices:  ev.DomainHealthNotices,
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

// mattermostPayload is Mattermost's incoming-webhook body, the same
// shape family as Slack's own incoming webhook (Mattermost documents its
// webhook format as Slack-compatible). Username/IconOverrideURL are
// omitted here: leaving them unset keeps whatever the webhook itself was
// configured with in Mattermost's admin console.
type mattermostPayload struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
	IconURL  string `json:"icon_url,omitempty"`
}

func notifyMattermost(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, mattermostPayload{Text: summaryText(ev)})
}

// larkContent is the "text" message body Lark (Feishu)'s incoming
// webhook expects nested under larkPayload.Content.
type larkContent struct {
	Text string `json:"text"`
}

// larkPayload is Lark/Feishu's incoming-webhook body for a plain text
// message (https://open.larksuite.com/document, custom bot webhooks).
type larkPayload struct {
	MsgType string      `json:"msg_type"`
	Content larkContent `json:"content"`
}

func notifyLark(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, larkPayload{MsgType: "text", Content: larkContent{Text: summaryText(ev)}})
}

// rocketChatPayload is Rocket.Chat's incoming-webhook body
// (https://docs.rocket.chat/docs/integrations#incoming-webhook-script), a
// superset of Slack's own shape rather than a byte-identical copy of it
// (unlike Mattermost's, which Mattermost itself documents as
// Slack-compatible): alias/emoji override the bot's posted name and
// avatar per message, fields this package uses to mark a firing event
// distinctly from a resolved one without needing a second payload shape.
type rocketChatPayload struct {
	Text  string `json:"text"`
	Alias string `json:"alias,omitempty"`
	Emoji string `json:"emoji,omitempty"`
}

func notifyRocketChat(ctx context.Context, client *http.Client, url string, ev Event) error {
	emoji := ":rotating_light:"
	if ev.Resolved {
		emoji = ":white_check_mark:"
	}
	return postJSON(ctx, client, url, rocketChatPayload{Text: summaryText(ev), Alias: "Levelrail", Emoji: emoji})
}

// webexPayload is a Cisco Webex Teams incoming webhook's message body
// (https://developer.webex.com/messaging/docs/api/guides/webhooks):
// markdown renders as Markdown in the space, letting summaryText's own
// code-fenced log lines and bullet lists actually format instead of
// showing up as literal backticks.
type webexPayload struct {
	Markdown string `json:"markdown"`
}

func notifyWebex(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, webexPayload{Markdown: summaryText(ev)})
}

// googleChatPayload is a Google Chat incoming webhook's message body
// (https://developers.google.com/workspace/chat/quickstart/webhooks): a
// single required "text" field, the same plain shape as Slack/Discord.
type googleChatPayload struct {
	Text string `json:"text"`
}

func notifyGoogleChat(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, googleChatPayload{Text: summaryText(ev)})
}

// opsgenieAPIURL is Opsgenie's fixed Alerts API endpoint. A var, not a
// const, so tests can point it at an httptest server, the same pattern
// pagerDutyEventsURL and resendAPIURL already use.
var opsgenieAPIURL = "https://api.opsgenie.com/v2/alerts"

// opsgeniePayload is the Alerts API's create-alert request body
// (https://docs.opsgenie.com/docs/alert-api#create-alert-request).
type opsgeniePayload struct {
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
	Priority    string `json:"priority,omitempty"`
}

// notifyOpsgenie always posts to opsgenieAPIURL, not rawURL: rawURL here
// packs the channel's API key as a query parameter against that fixed
// endpoint, the same "NotifyURL packs credentials, not a destination"
// convention notifyResend already establishes; see parseOpsgenieCreds.
// The key travels in Opsgenie's own "GenieKey <key>" Authorization
// scheme, not Bearer.
func notifyOpsgenie(ctx context.Context, client *http.Client, rawURL string, ev Event) error {
	key, err := parseOpsgenieCreds(rawURL)
	if err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	priority := "P1"
	if ev.Resolved {
		priority = "P5"
	}
	payload := opsgeniePayload{Message: ev.Rule.Name, Description: summaryText(ev), Priority: priority}
	return postJSONWithAuth(ctx, client, opsgenieAPIURL, payload, "GenieKey "+key)
}

// parseOpsgenieCreds extracts the key query parameter an Opsgenie
// notify_url must carry (see notifyOpsgenie's own doc comment for why it
// travels this way).
func parseOpsgenieCreds(rawURL string) (key string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid opsgenie notify_url: %w", err)
	}
	key = u.Query().Get("key")
	if key == "" {
		return "", fmt.Errorf("opsgenie notify_url must include a key query parameter")
	}
	return key, nil
}

// gotifyPayload is Gotify's message API body
// (https://gotify.net/docs/pushmsg). rawURL for this kind is the whole
// "<server>/message?token=<apptoken>" endpoint, a real, literal query
// parameter Gotify's own API expects there, not a packed convention like
// Pushover's or Resend's: no parsing needed, notifyGotify posts to it
// directly.
type gotifyPayload struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority,omitempty"`
}

func notifyGotify(ctx context.Context, client *http.Client, url string, ev Event) error {
	priority := 5
	if ev.Resolved {
		priority = 2
	}
	return postJSON(ctx, client, url, gotifyPayload{Title: ev.Rule.Name, Message: summaryText(ev), Priority: priority})
}

// ntfyPayload is ntfy's publish API body (https://docs.ntfy.sh/publish/).
// Topic isn't included here: rawURL already names the topic
// (https://ntfy.sh/<topic> or a self-hosted equivalent), so ntfy infers
// it from the request path the same way a plain-text POST to that URL
// would.
type ntfyPayload struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority,omitempty"`
}

// notifyNtfy posts to rawURL as given. An access-controlled ntfy topic
// carries its bearer token as an "auth" query parameter on rawURL (this
// package's own convention, parsed out here rather than forwarded in the
// query string, matching notifyTelegram/notifyPushover's own "credentials
// travel in the query string, not the final request" pattern): when
// present it's removed from the URL and sent as an Authorization header
// instead, since ntfy authenticates that way, not via the query string.
func notifyNtfy(ctx context.Context, client *http.Client, rawURL string, ev Event) error {
	token, cleanURL, err := extractNtfyToken(rawURL)
	if err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	priority := 4
	if ev.Resolved {
		priority = 3
	}
	payload := ntfyPayload{Title: ev.Rule.Name, Message: summaryText(ev), Priority: priority}
	if token == "" {
		return postJSON(ctx, client, cleanURL, payload)
	}
	return postJSONWithAuth(ctx, client, cleanURL, payload, "Bearer "+token)
}

// extractNtfyToken pulls an optional "auth" query parameter (an access
// token for a protected ntfy topic) out of rawURL, returning the token
// and rawURL with that parameter removed. No token present is not an
// error: most ntfy topics, including the public ntfy.sh ones, need none.
func extractNtfyToken(rawURL string) (token, cleanURL string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid ntfy notify_url: %w", err)
	}
	q := u.Query()
	token = q.Get("auth")
	if token == "" {
		return "", rawURL, nil
	}
	q.Del("auth")
	u.RawQuery = q.Encode()
	return token, u.String(), nil
}

// resendAPIURL is Resend's fixed transactional-email endpoint. A var,
// not a const, so tests can point it at an httptest server, the same
// pattern pagerDutyEventsURL already uses.
var resendAPIURL = "https://api.resend.com/emails"

// resendPayload is the Resend Emails API's request body
// (https://resend.com/docs/api-reference/emails/send-email).
type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// resendDefaultFrom is Resend's own sandbox sender: it works for any
// account without a verified sending domain, so a channel that only
// supplies an API key and a destination address still sends.
const resendDefaultFrom = "onboarding@resend.dev"

// notifyResend sends via the Resend Emails API rather than a webhook:
// Resend needs an API key (as an Authorization header, never a query
// parameter) and a destination address, neither of which fits
// notify.go's "NotifyURL is a webhook URL" convention. Rather than
// changing Rule's shape, rawURL packs both as query parameters against
// resendAPIURL as a fixed placeholder, the identical convention
// notifyPushover already established for its own two credentials
// (token/user) and notifyNtfy now reuses for its own optional token;
// parseResendCreds below reads them back out and notifyResend posts the
// real request to resendAPIURL with the key moved into the Authorization
// header where Resend's API actually expects it.
func notifyResend(ctx context.Context, client *http.Client, rawURL string, ev Event) error {
	key, to, from, err := parseResendCreds(rawURL)
	if err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	subject := fmt.Sprintf("[Levelrail] %s", ev.Rule.Name)
	if ev.Resolved {
		subject = fmt.Sprintf("[Levelrail][RESOLVED] %s", ev.Rule.Name)
	}
	payload := resendPayload{From: from, To: []string{to}, Subject: subject, Text: summaryText(ev)}
	return postJSONWithAuth(ctx, client, resendAPIURL, payload, "Bearer "+key)
}

// parseResendCreds extracts the key/to/from query parameters a Resend
// notify_url must carry (see notifyResend's own doc comment for why they
// travel this way). from defaults to resendDefaultFrom when absent, so a
// channel only has to supply the two credentials that actually vary per
// account.
func parseResendCreds(rawURL string) (key, to, from string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid resend notify_url: %w", err)
	}
	q := u.Query()
	key = q.Get("key")
	to = q.Get("to")
	if key == "" || to == "" {
		return "", "", "", fmt.Errorf("resend notify_url must include key and to query parameters")
	}
	from = q.Get("from")
	if from == "" {
		from = resendDefaultFrom
	}
	return key, to, from, nil
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
	if len(ev.CertNotices) > 0 {
		fmt.Fprintf(&b, "\nCertificates:\n- %s", strings.Join(ev.CertNotices, "\n- "))
	}
	if len(ev.PatchNotices) > 0 {
		fmt.Fprintf(&b, "\nNodes:\n- %s", strings.Join(ev.PatchNotices, "\n- "))
	}
	if len(ev.DiskSpaceNotices) > 0 {
		fmt.Fprintf(&b, "\nDisk space:\n- %s", strings.Join(ev.DiskSpaceNotices, "\n- "))
	}
	if len(ev.ResourceUsageNotices) > 0 {
		fmt.Fprintf(&b, "\nResource usage:\n- %s", strings.Join(ev.ResourceUsageNotices, "\n- "))
	}
	if ev.TaskFailureNotice != "" {
		fmt.Fprintf(&b, "\nScheduled task: %s", ev.TaskFailureNotice)
	}
	if len(ev.DomainHealthNotices) > 0 {
		fmt.Fprintf(&b, "\nDomains:\n- %s", strings.Join(ev.DomainHealthNotices, "\n- "))
	}
	return b.String()
}

func postJSON(ctx context.Context, client *http.Client, url string, payload any) error {
	return postJSONWithAuth(ctx, client, url, payload, "")
}

// postJSONWithAuth is postJSON plus an optional Authorization header,
// needed by the two kinds (Resend, and ntfy on a protected topic) that
// authenticate via a bearer header rather than a field in the JSON body
// itself. authHeader is skipped entirely when empty, so postJSON above
// is just this with no header to set.
func postJSONWithAuth(ctx context.Context, client *http.Client, url string, payload any, authHeader string) error {
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
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

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
