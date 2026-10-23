package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// HTTP header names and values shared by the outbound notification senders.
const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
	bearerPrefix      = "Bearer "
)

// Event is what a firing (or resolved) rule hands to a Notifier: enough
// context to write a useful message without the notifier needing to go
// query anything itself.
type Event struct {
	Rule Rule
	// Resolved events are sent too, so operators do not learn to mute a channel that only reports problems starting.
	Resolved bool
	// LogLines is populated only for a firing (not resolved) crashloop
	// event: the last up-to-200 lines of the failing container's logs.
	// Nil for threshold rules and for resolved events.
	LogLines []string
	// CertNotices is set only on a firing cert_expiry event.
	CertNotices []string
	// CertFacts and CertBaseURL drive the actionable cert expiry email; the base URL is empty when no public address is configured.
	CertFacts   []CertFact
	CertBaseURL string
	// PatchNotices is set only on a firing patch_status event.
	PatchNotices []string
	// DiskSpaceNotices is set only on a firing node_disk_space event.
	DiskSpaceNotices []string
	// NodeOfflineNotices is populated only for a firing node_offline
	// event: one line per offline node, from EvaluateNodeOffline.
	NodeOfflineNotices []string
	// NodeCertNotices is populated only for a firing node_cert_expiring
	// event: one line per node whose agent certificate needs attention.
	NodeCertNotices []string
	// ResourceUsageNotices is set only on a firing node_resource_usage event.
	ResourceUsageNotices []string
	// TaskFailureNotice is set only on a firing scheduled_task_failure event.
	TaskFailureNotice string
	// DomainHealthNotices is set only on a firing domain_health event.
	DomainHealthNotices []string
	// BackupMissingNotice is set only on a firing backup_missing event.
	BackupMissingNotice string
	// VersionSkewNotice is set only on a firing version_skew event.
	VersionSkewNotice string
	// BudgetNotices is populated only for a firing budget_threshold event:
	// one line per budget that crossed a threshold.
	BudgetNotices []string

	// SLONotice is set only for a firing slo_burn event: which burn-rate tier
	// tripped and how much error budget is left.
	SLONotice string

	// LogNotice is set for a firing log_pattern or log_level_rate event: the
	// count or rate that tripped. LogLines then holds the matching samples.
	LogNotice string

	// Changes is what changed on the app shortly before it fired; set for
	// firing app-scoped events only. ChangesLink points at its dashboard page.
	Changes     *changes.Result
	ChangesLink string

	// Headline replaces the default first line for flapping and grouped
	// messages; GroupNotices lists the alerts folded into a grouped one.
	Headline     string
	GroupNotices []string
	GroupCount   int
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

// PushSender sends one browser push to every registered subscription; *webpush.Sender satisfies it.
type PushSender interface {
	Send(ctx context.Context, title, body string) error
}

// NewNotifier picks the Notifier for r.NotifyKind, falling back to generic so a mistyped kind still notifies someone.
func NewNotifier(client *http.Client, sender email.Sender, pushSender PushSender, r Rule) Notifier {
	if client == nil {
		client = netguard.NewClient()
	}
	if IsPluginKind(r.NotifyKind) {
		return pluginNotifier{plugin: PluginName(r.NotifyKind)}
	}
	if r.NotifyKind == NotifyEmail {
		return emailNotifier{sender: sender, to: r.NotifyURL}
	}
	if r.NotifyKind == NotifyWebpush {
		return webpushNotifier{sender: pushSender}
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

// genericPayload is the full machine-readable event, for webhook receivers that are not Slack or Discord.
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
	NodeOfflineNotices   []string   `json:"node_offline_notices,omitempty"`
	NodeCertNotices      []string   `json:"node_cert_notices,omitempty"`
	TaskFailureNotice    string     `json:"task_failure_notice,omitempty"`
	DomainHealthNotices  []string   `json:"domain_health_notices,omitempty"`
	BackupMissingNotice  string     `json:"backup_missing_notice,omitempty"`
	VersionSkewNotice    string     `json:"version_skew_notice,omitempty"`
	BudgetNotices        []string   `json:"budget_notices,omitempty"`
	Headline             string     `json:"headline,omitempty"`
	GroupNotices         []string   `json:"group_notices,omitempty"`
	GroupCount           int        `json:"group_count,omitempty"`
	Severity             string     `json:"severity,omitempty"`

	SLONotice     string           `json:"slo_notice,omitempty"`
	LogNotice     string           `json:"log_notice,omitempty"`
	RecentChanges []changes.Change `json:"recent_changes,omitempty"`
	ChangesLink   string           `json:"changes_link,omitempty"`
}

func notifyGeneric(ctx context.Context, client *http.Client, url string, ev Event) error {
	payload := genericPayload{
		RuleID: ev.Rule.ID, RuleName: ev.Rule.Name, Kind: string(ev.Rule.Kind),
		ResourceID: ev.Rule.ResourceID, Resolved: ev.Resolved, Value: ev.Rule.LastValue,
		Firing: ev.Rule.Firing, FiringSince: ev.Rule.FiringSince, LogLines: ev.LogLines,
		CertNotices: ev.CertNotices, PatchNotices: ev.PatchNotices, DiskSpaceNotices: ev.DiskSpaceNotices,
		ResourceUsageNotices: ev.ResourceUsageNotices,
		NodeOfflineNotices:   ev.NodeOfflineNotices,
		NodeCertNotices:      ev.NodeCertNotices,
		TaskFailureNotice:    ev.TaskFailureNotice,
		DomainHealthNotices:  ev.DomainHealthNotices,
		BackupMissingNotice:  ev.BackupMissingNotice,
		VersionSkewNotice:    ev.VersionSkewNotice,
		BudgetNotices:        ev.BudgetNotices,
		Headline:             ev.Headline,
		GroupNotices:         ev.GroupNotices,
		GroupCount:           ev.GroupCount,
		Severity:             ev.Rule.Severity,
		ChangesLink:          ev.ChangesLink,
		SLONotice:            ev.SLONotice,
		LogNotice:            ev.LogNotice,
	}
	if ev.Changes != nil {
		payload.RecentChanges = ev.Changes.Changes[:min(len(ev.Changes.Changes), changes.NotifyMaxLines)]
	}
	return postJSON(ctx, client, url, payload)
}

// slackPayload needs only the top-level text field; blocks and attachments are optional.
type slackPayload struct {
	Text string `json:"text"`
}

func notifySlack(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, slackPayload{Text: summaryText(ev)})
}

// Receiver hard limits on message length, in bytes.
const (
	discordMaxContent   = 2000
	pagerDutyMaxSummary = 1024
)

// discordPayload is Discord's incoming-webhook shape: the equivalent
// top-level "content" field.
type discordPayload struct {
	Content string `json:"content"`
}

func notifyDiscord(ctx context.Context, client *http.Client, url string, ev Event) error {
	return postJSON(ctx, client, url, discordPayload{Content: summaryTextCapped(ev, discordMaxContent)})
}

// chat_id goes in the body, since Telegram does not reliably read it from a query string.
type telegramPayload struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// rawURL is the full sendMessage endpoint with the bot token in its path; chat_id is moved from the query into the body.
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

// Token and user come from rawURL's query string, the same way notifyTelegram reads chat_id.
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

// Always posts to the fixed Events URL; rawURL carries the routing key, not a destination.
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
			Summary:  summaryTextCapped(ev, pagerDutyMaxSummary),
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

// Mattermost's webhook is Slack-compatible; username and icon are left unset so the webhook's own config applies.
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

// Rocket.Chat is a superset of Slack's shape; alias and emoji override the posted name and avatar per message.
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

// markdown makes code fences and bullets render in the space instead of showing as literal backticks.
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

// Always posts to the fixed Opsgenie URL; rawURL carries the API key. Auth uses the GenieKey scheme, not Bearer.
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

// rawURL is the full Gotify message endpoint including its token, so notifyGotify posts to it directly.
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

type ntfyPayload struct {
	Topic    string `json:"topic"`
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority,omitempty"`
}

// notifyNtfy posts to rawURL. An "auth" query parameter carries the ntfy token and is sent as an Authorization header.
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
	return postNtfy(ctx, client, cleanURL, token, payload)
}

// postNtfy publishes through ntfy's JSON API: the server root with the topic in the body.
// JSON posted to a topic path is published as literal message text instead.
func postNtfy(ctx context.Context, client *http.Client, topicURL, token string, payload ntfyPayload) error {
	u, err := url.Parse(topicURL)
	if err != nil {
		return fmt.Errorf("invalid ntfy notify_url: %w", err)
	}
	dir, topic := path.Split(strings.TrimRight(u.Path, "/"))
	if topic == "" {
		return fmt.Errorf("ntfy notify_url must end in a topic, such as https://ntfy.sh/my-topic")
	}
	u.Path, u.RawPath, u.RawQuery = dir, "", ""
	payload.Topic = topic
	authHeader := ""
	if token != "" {
		authHeader = bearerPrefix + token
	}
	return postJSONWithAuth(ctx, client, u.String(), payload, authHeader)
}

// Pulls the optional auth token out of rawURL; most ntfy topics need none.
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

// notifyResend sends via the Resend Emails API. The key and destination ride in rawURL's query string and move to an Authorization header.
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
	return postJSONWithAuth(ctx, client, resendAPIURL, payload, bearerPrefix+key)
}

// from defaults to resendDefaultFrom, so a channel only supplies the per-account credentials.
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

// Shared with deploy_notify.go's Telegram send, so both parse notify_url identically.
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

// One text summary serves every simple channel (Slack, Discord, Telegram, email body).
func summaryText(ev Event) string {
	return summaryBody(ev) + changesSuffix(ev)
}

// summaryTextCapped is summaryText limited to limit bytes for receivers with a
// hard message limit; the changes section survives and the body is cut.
func summaryTextCapped(ev Event, limit int) string {
	body, suffix := summaryBody(ev), changesSuffix(ev)
	if len(body)+len(suffix) <= limit {
		return body + suffix
	}
	keep := limit - len(suffix) - len("...")
	if keep < 0 {
		keep = 0
	}
	return strings.ToValidUTF8(body[:min(keep, len(body))], "") + "..." + suffix
}

func changesSuffix(ev Event) string {
	if ev.Changes == nil || ev.Resolved {
		return ""
	}
	if block := changes.NotifyBlock(*ev.Changes, ev.ChangesLink); block != "" {
		return "\n" + block
	}
	return ""
}

func summaryBody(ev Event) string {
	var b strings.Builder
	switch {
	case ev.Headline != "":
		b.WriteString(ev.Headline)
	case ev.Resolved:
		fmt.Fprintf(&b, "[RESOLVED] %s (%s) on %s", ev.Rule.Name, ev.Rule.Kind, ev.Rule.ResourceID)
	default:
		fmt.Fprintf(&b, "[FIRING] %s (%s) on %s", ev.Rule.Name, ev.Rule.Kind, ev.Rule.ResourceID)
	}
	if ev.Headline == "" && ev.Rule.LastValue != nil {
		fmt.Fprintf(&b, ", value=%v", *ev.Rule.LastValue)
	}
	if len(ev.GroupNotices) > 0 {
		fmt.Fprintf(&b, "\nAlerts:\n- %s", strings.Join(ev.GroupNotices, "\n- "))
	}
	if ev.LogNotice != "" {
		fmt.Fprintf(&b, "\n%s", ev.LogNotice)
	}
	if len(ev.LogLines) > 0 {
		label := "Last"
		if ev.LogNotice != "" {
			label = "Matching sample"
		}
		fmt.Fprintf(&b, "\n%s %d log lines:\n```\n%s\n```", label, len(ev.LogLines), strings.Join(ev.LogLines, "\n"))
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
	if len(ev.NodeOfflineNotices) > 0 {
		fmt.Fprintf(&b, "\nOffline nodes:\n- %s", strings.Join(ev.NodeOfflineNotices, "\n- "))
	}
	if len(ev.NodeCertNotices) > 0 {
		fmt.Fprintf(&b, "\nNode certificates:\n- %s", strings.Join(ev.NodeCertNotices, "\n- "))
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
	if ev.BackupMissingNotice != "" {
		fmt.Fprintf(&b, "\nBackup: %s", ev.BackupMissingNotice)
	}
	if ev.VersionSkewNotice != "" {
		fmt.Fprintf(&b, "\nVersion: %s", ev.VersionSkewNotice)
	}
	if len(ev.BudgetNotices) > 0 {
		fmt.Fprintf(&b, "\nBudgets:\n- %s", strings.Join(ev.BudgetNotices, "\n- "))
	}
	if ev.SLONotice != "" {
		fmt.Fprintf(&b, "\n%s", ev.SLONotice)
	}
	return b.String()
}

func postJSON(ctx context.Context, client *http.Client, url string, payload any) error {
	return postJSONWithAuth(ctx, client, url, payload, "")
}

// Kept small because Engine.Tick notifies rules sequentially, so retries delay every later rule in the pass.
const notifyMaxAttempts = 3

// A var so tests can shrink it and keep retry-path tests fast.
var notifyRetryBaseDelay = 500 * time.Millisecond

// notifyHTTPError carries the status so retries apply only to 5xx and 429, not to permanent 4xx errors.
type notifyHTTPError struct {
	statusCode int
}

func (e *notifyHTTPError) Error() string {
	return fmt.Sprintf("alerting: notify: receiver returned status %d", e.statusCode)
}

// Transport errors and 5xx/429 are retried; other 4xx responses fail identically on every attempt.
func isRetryableNotifyError(err error) bool {
	var httpErr *notifyHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode >= http.StatusInternalServerError || httpErr.statusCode == http.StatusTooManyRequests
	}
	return true
}

// Adds an optional Authorization header for Resend and protected ntfy topics.
// Retries transient failures with exponential backoff, so one hiccup does not drop a notification.
func postJSONWithAuth(ctx context.Context, client *http.Client, url string, payload any, authHeader string) error {
	if url == "" {
		return fmt.Errorf("alerting: notify: no notify_url configured")
	}
	if IsMaskedNotifyTarget(url) {
		return ErrChannelSecretNotRevealed
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("alerting: notify: encode payload: %w", err)
	}

	var lastErr error
	delay := notifyRetryBaseDelay
	for attempt := 1; attempt <= notifyMaxAttempts; attempt++ {
		lastErr = postJSONAttempt(ctx, client, url, body, authHeader)
		if lastErr == nil {
			return nil
		}
		if attempt == notifyMaxAttempts || !isRetryableNotifyError(lastErr) {
			return lastErr
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
		delay *= 2
	}
	return lastErr
}

func postJSONAttempt(ctx context.Context, client *http.Client, url string, body []byte, authHeader string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("alerting: notify: build request: %w", err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("alerting: notify: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &notifyHTTPError{statusCode: resp.StatusCode}
	}
	return nil
}

// emailNotifier implements Notifier directly: its transport is SMTP, not an HTTP URL plus JSON body.
type emailNotifier struct {
	sender email.Sender // nil means "no email capability configured"
	// NotifyURL is the destination email address for this channel.
	to string
}

func (n emailNotifier) Notify(ctx context.Context, ev Event) error {
	if n.sender == nil {
		return fmt.Errorf("alerting: notify: email is not configured on this control plane")
	}
	if n.to == "" {
		return fmt.Errorf("alerting: notify: no notify_url (destination email address) configured")
	}

	if !ev.Resolved && len(ev.CertFacts) > 0 {
		return n.notifyCert(ctx, ev)
	}

	subject := fmt.Sprintf("[Levelrail] %s", ev.Rule.Name)
	if ev.Resolved {
		subject = fmt.Sprintf("[Levelrail][RESOLVED] %s", ev.Rule.Name)
	}
	status := "firing"
	if ev.Resolved {
		status = "resolved"
	}
	vals := map[string]string{
		"alert_name": ev.Rule.Name, "severity": ev.Rule.Severity + " (" + status + ")",
		"alert_summary": summaryText(ev), "app_name": ev.Rule.ResourceID,
	}
	if err := sendTemplated(ctx, n.sender, mailtemplates.EventAlertNotification, n.to, vals, subject, summaryText(ev)); err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return nil
}

// webpushNotifier implements Notifier directly: every registered subscription receives the push, not r.NotifyURL.
type webpushNotifier struct {
	sender PushSender // nil means "no browser push capability configured"
}

func (n webpushNotifier) Notify(ctx context.Context, ev Event) error {
	if n.sender == nil {
		return fmt.Errorf("alerting: notify: browser push is not configured on this control plane")
	}
	title := fmt.Sprintf("[FIRING] %s", ev.Rule.Name)
	if ev.Resolved {
		title = fmt.Sprintf("[RESOLVED] %s", ev.Rule.Name)
	}
	if ev.Headline != "" {
		title = ev.Headline
	}
	if err := n.sender.Send(ctx, title, summaryText(ev)); err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return nil
}

// Transport failures and SMTP 4xx replies are retried; 5xx replies are permanent, and retrying them only delays the fix.
func isRetryableEmailError(err error) bool {
	var sendErr *email.SendError
	if errors.As(err, &sendErr) && sendErr.Attempts > 0 {
		return false
	}
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) {
		return protoErr.Code < 500
	}
	return true
}

// Retries transient SMTP failures with the same backoff as the HTTP channels.
func sendEmailWithRetry(ctx context.Context, sender email.Sender, to, subject, body string) error {
	var lastErr error
	delay := notifyRetryBaseDelay
	for attempt := 1; attempt <= notifyMaxAttempts; attempt++ {
		lastErr = sender.Send(ctx, to, subject, body)
		if lastErr == nil {
			return nil
		}
		if attempt == notifyMaxAttempts || !isRetryableEmailError(lastErr) {
			return lastErr
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
		delay *= 2
	}
	return lastErr
}

func (n emailNotifier) notifyCert(ctx context.Context, ev Event) error {
	f := ev.CertFacts[0]
	left := fmt.Sprintf("expires in %d days", f.DaysLeft)
	switch {
	case f.DaysLeft < 0:
		left = "has expired"
	case f.DaysLeft == 0:
		left = "expires today"
	case f.DaysLeft == 1:
		left = "expires in 1 day"
	}
	vals := map[string]string{"domain": f.Domain, "issuer": f.Issuer, "days_left": left, "reason": f.Reason}
	key := mailtemplates.EventCertExpiryPlain
	if l := BuildCertLinks(ev.CertBaseURL, f, ""); l.Primary != "" {
		key = mailtemplates.EventCertExpiry
		vals["primary_label"], vals["primary_url"], vals["inbox_url"], vals["settings_url"] = l.PrimaryLabel, l.Primary, l.Inbox, l.Settings
	}
	subject := fmt.Sprintf("Certificate for %s %s", f.Domain, left)
	if err := sendTemplated(ctx, n.sender, key, n.to, vals, subject, f.Notice()); err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	return nil
}
