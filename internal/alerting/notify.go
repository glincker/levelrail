package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// NewNotifier builds the right Notifier for r.NotifyKind. An unknown or
// empty NotifyKind falls back to NotifyGeneric rather than erroring: a
// rule with a typo'd notify_kind should still get *a* notification,
// diagnosable from the payload shape, rather than silently notifying no
// one.
func NewNotifier(client *http.Client, r Rule) Notifier {
	if client == nil {
		client = http.DefaultClient
	}
	build := notifyGeneric
	switch r.NotifyKind {
	case NotifySlack:
		build = notifySlack
	case NotifyDiscord:
		build = notifyDiscord
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

// summaryText is the human-readable line both Slack and Discord's
// simple webhook shapes send: they're both "one text field," so one
// summary builder serves both rather than duplicating this per channel.
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
