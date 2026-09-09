package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/email"
)

func TestNotifyGeneric_PostsExpectedPayload(t *testing.T) {
	var gotBody genericPayload
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	value := 95.5
	r := Rule{ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web", NotifyURL: srv.URL, LastValue: &value, Firing: true}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody.RuleID != "r1" || gotBody.RuleName != "high cpu" || gotBody.ResourceID != "service:web" {
		t.Errorf("payload = %+v, missing expected identifying fields", gotBody)
	}
	if gotBody.Value == nil || *gotBody.Value != 95.5 {
		t.Errorf("payload.Value = %v, want 95.5", gotBody.Value)
	}
	if !gotBody.Firing {
		t.Error("payload.Firing = false, want true")
	}
}

func TestNotifySlack_PostsTextField(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifySlack}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	text, ok := got["text"].(string)
	if !ok || !strings.Contains(text, "high cpu") || !strings.Contains(text, "FIRING") {
		t.Errorf("Slack payload = %+v, want a text field mentioning the rule name and FIRING", got)
	}
}

func TestNotifyDiscord_PostsContentField(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "crashloop", Kind: KindCrashloop, ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyDiscord}
	notifier := NewNotifier(nil, nil, r)

	err := notifier.Notify(context.Background(), Event{Rule: r, LogLines: []string{"line 1", "line 2"}})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	content, ok := got["content"].(string)
	if !ok || !strings.Contains(content, "line 1") || !strings.Contains(content, "line 2") {
		t.Errorf("Discord payload = %+v, want content including the log lines", got)
	}
}

func TestNotify_ResolvedEvent(t *testing.T) {
	var gotBody genericPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: srv.URL}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r, Resolved: true}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !gotBody.Resolved {
		t.Error("payload.Resolved = false, want true")
	}
}

func TestNotify_NoURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "high cpu"}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when NotifyURL is empty")
	}
}

func TestNotify_ReceiverErrorStatus_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", NotifyURL: srv.URL}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when the receiver returns a non-2xx status")
	}
}

func TestNotifyTelegram_PostsChatIDAndText(t *testing.T) {
	var got telegramPayload
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		NotifyURL:  srv.URL + "/bot123456:ABC-DEF/sendMessage?chat_id=987654321",
		NotifyKind: NotifyTelegram,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if gotPath != "/bot123456:ABC-DEF/sendMessage" {
		t.Errorf("request path = %q, want the bot-token path preserved", gotPath)
	}
	if got.ChatID != "987654321" {
		t.Errorf("payload.ChatID = %q, want 987654321 (parsed from the notify_url's chat_id query param)", got.ChatID)
	}
	if !strings.Contains(got.Text, "high cpu") {
		t.Errorf("payload.Text = %q, want it to mention the rule name", got.Text)
	}
}

func TestNotifyTelegram_MissingChatID_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "x", NotifyURL: srv.URL + "/bot123/sendMessage", NotifyKind: NotifyTelegram}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when notify_url has no chat_id query parameter")
	}
}

func TestNotifyTelegram_InvalidURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "x", NotifyURL: "://not a url", NotifyKind: NotifyTelegram}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error for an unparseable notify_url")
	}
}

func TestNotifyPushover_PostsTokenUserAndMessage(t *testing.T) {
	var got pushoverPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		NotifyURL:  srv.URL + "/1/messages.json?token=app-token-123&user=user-key-456",
		NotifyKind: NotifyPushover,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.Token != "app-token-123" {
		t.Errorf("payload.Token = %q, want app-token-123 (parsed from the notify_url's token query param)", got.Token)
	}
	if got.User != "user-key-456" {
		t.Errorf("payload.User = %q, want user-key-456 (parsed from the notify_url's user query param)", got.User)
	}
	if !strings.Contains(got.Message, "high cpu") {
		t.Errorf("payload.Message = %q, want it to mention the rule name", got.Message)
	}
	if got.Title != "high cpu" {
		t.Errorf("payload.Title = %q, want the rule name", got.Title)
	}
}

func TestNotifyPushover_MissingCreds_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tests := []struct {
		name string
		url  string
	}{
		{"missing token", srv.URL + "/1/messages.json?user=user-key-456"},
		{"missing user", srv.URL + "/1/messages.json?token=app-token-123"},
		{"missing both", srv.URL + "/1/messages.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{ID: "r1", Name: "x", NotifyURL: tt.url, NotifyKind: NotifyPushover}
			notifier := NewNotifier(nil, nil, r)

			if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
				t.Error("Notify() error = nil, want an error when notify_url is missing token or user")
			}
		})
	}
}

func TestNotifyPushover_InvalidURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "x", NotifyURL: "://not a url", NotifyKind: NotifyPushover}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error for an unparseable notify_url")
	}
}

func withPagerDutyEventsURL(t *testing.T, url string) {
	t.Helper()
	original := pagerDutyEventsURL
	pagerDutyEventsURL = url
	t.Cleanup(func() { pagerDutyEventsURL = original })
}

func TestNotifyPagerDuty_PostsRoutingKeyAndSummary(t *testing.T) {
	var got pagerDutyPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	withPagerDutyEventsURL(t, srv.URL)

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		NotifyURL: "routing-key-123", NotifyKind: NotifyPagerDuty,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.RoutingKey != "routing-key-123" {
		t.Errorf("payload.RoutingKey = %q, want routing-key-123", got.RoutingKey)
	}
	if got.EventAction != "trigger" {
		t.Errorf("payload.EventAction = %q, want trigger", got.EventAction)
	}
	if !strings.Contains(got.Payload.Summary, "high cpu") {
		t.Errorf("payload.Payload.Summary = %q, want it to mention the rule name", got.Payload.Summary)
	}
	if got.Payload.Source != "service:web" {
		t.Errorf("payload.Payload.Source = %q, want the resource id", got.Payload.Source)
	}
	if got.Payload.Severity != "critical" {
		t.Errorf("payload.Payload.Severity = %q, want critical for a firing event", got.Payload.Severity)
	}
}

func TestNotifyPagerDuty_ResolvedEvent_SeverityInfo(t *testing.T) {
	var got pagerDutyPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	withPagerDutyEventsURL(t, srv.URL)

	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: "routing-key-123", NotifyKind: NotifyPagerDuty}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r, Resolved: true}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.Payload.Severity != "info" {
		t.Errorf("payload.Payload.Severity = %q, want info for a resolved event", got.Payload.Severity)
	}
}

func TestNotifyPagerDuty_MissingRoutingKey_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "x", NotifyKind: NotifyPagerDuty}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when notify_url (routing key) is empty")
	}
}

func TestNotifyTeams_PostsMessageCard(t *testing.T) {
	var got teamsPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "crashloop", Kind: KindCrashloop, ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyTeams}
	notifier := NewNotifier(nil, nil, r)

	err := notifier.Notify(context.Background(), Event{Rule: r, LogLines: []string{"line 1", "line 2"}})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.Type != "MessageCard" {
		t.Errorf("payload.Type = %q, want MessageCard", got.Type)
	}
	if !strings.Contains(got.Text, "line 1") || !strings.Contains(got.Text, "line 2") {
		t.Errorf("payload.Text = %q, want it to include the log lines", got.Text)
	}
}

func TestNotifyTeams_NoURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "x", NotifyKind: NotifyTeams}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when notify_url is empty")
	}
}

func TestNewNotifier_Email_NoSMTPConfigured_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: "ops@example.com", NotifyKind: NotifyEmail}
	notifier := NewNotifier(nil, nil, r) // no email.Sender

	err := notifier.Notify(context.Background(), Event{Rule: r})
	if err == nil {
		t.Fatal("Notify() error = nil, want a clear 'not configured' error when no email capability is set up")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %q, want it to say email is not configured", err.Error())
	}
}

func mustSMTPSender(t *testing.T, cfg email.SMTPConfig) email.Sender {
	t.Helper()
	sender, err := email.NewSender(email.Config{Backend: email.BackendSMTP, SMTP: &cfg})
	if err != nil {
		t.Fatalf("email.NewSender() error = %v", err)
	}
	return sender
}

func TestNewNotifier_Email_NoDestinationAddress_Errors(t *testing.T) {
	sender := mustSMTPSender(t, email.SMTPConfig{Addr: "smtp.example.com:587", Host: "smtp.example.com", From: "alerts@example.com"})
	r := Rule{ID: "r1", Name: "high cpu", NotifyKind: NotifyEmail} // NotifyURL (the "to" address) left empty
	notifier := NewNotifier(nil, sender, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when no destination address is configured")
	}
}

func TestNewNotifier_Email_UnreachableServer_ErrorPropagates(t *testing.T) {
	// Not a real send: no SMTP fixture here, this only proves
	// emailNotifier actually calls through to email.Sender.Send and
	// wraps whatever it returns, rather than silently swallowing a
	// connection failure.
	sender := mustSMTPSender(t, email.SMTPConfig{Addr: "127.0.0.1:1", Host: "127.0.0.1", From: "alerts@example.com"})
	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: "ops@example.com", NotifyKind: NotifyEmail}
	notifier := NewNotifier(nil, sender, r)

	err := notifier.Notify(context.Background(), Event{Rule: r})
	if err == nil {
		t.Fatal("Notify() error = nil, want an error when the SMTP server is unreachable")
	}
	if !strings.Contains(err.Error(), "send email") {
		t.Errorf("error = %q, want it wrapped with \"send email\"", err.Error())
	}
}

func TestNotifyMattermost_PostsTextField(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyMattermost}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	text, ok := got["text"].(string)
	if !ok || !strings.Contains(text, "high cpu") || !strings.Contains(text, "FIRING") {
		t.Errorf("Mattermost payload = %+v, want a text field mentioning the rule name and FIRING", got)
	}
}

func TestNotifyLark_PostsMsgTypeAndContent(t *testing.T) {
	var got larkPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyLark}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.MsgType != "text" {
		t.Errorf("payload.MsgType = %q, want text", got.MsgType)
	}
	if !strings.Contains(got.Content.Text, "high cpu") {
		t.Errorf("payload.Content.Text = %q, want it to mention the rule name", got.Content.Text)
	}
}

func TestNotifyGotify_PostsTitleMessageAndToken(t *testing.T) {
	var got gotifyPayload
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		NotifyURL: srv.URL + "/message?token=app-token-123", NotifyKind: NotifyGotify,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if gotPath != "/message?token=app-token-123" {
		t.Errorf("request URI = %q, want the token query parameter preserved", gotPath)
	}
	if got.Title != "high cpu" {
		t.Errorf("payload.Title = %q, want the rule name", got.Title)
	}
	if !strings.Contains(got.Message, "high cpu") {
		t.Errorf("payload.Message = %q, want it to mention the rule name", got.Message)
	}
	if got.Priority != 5 {
		t.Errorf("payload.Priority = %d, want 5 for a firing event", got.Priority)
	}
}

func TestNotifyGotify_ResolvedEvent_LowerPriority(t *testing.T) {
	var got gotifyPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: srv.URL + "/message?token=x", NotifyKind: NotifyGotify}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r, Resolved: true}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.Priority != 2 {
		t.Errorf("payload.Priority = %d, want 2 for a resolved event", got.Priority)
	}
}

func TestNotifyNtfy_PostsTitleAndMessage_NoAuthHeaderWhenNoToken(t *testing.T) {
	var got ntfyPayload
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		NotifyURL: srv.URL + "/my-topic", NotifyKind: NotifyNtfy,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want none when notify_url carries no auth token", gotAuth)
	}
	if got.Title != "high cpu" {
		t.Errorf("payload.Title = %q, want the rule name", got.Title)
	}
	if !strings.Contains(got.Message, "high cpu") {
		t.Errorf("payload.Message = %q, want it to mention the rule name", got.Message)
	}
	if got.Priority != 4 {
		t.Errorf("payload.Priority = %d, want 4 for a firing event", got.Priority)
	}
}

func TestNotifyNtfy_AuthTokenMovedToHeaderAndStrippedFromURL(t *testing.T) {
	var got ntfyPayload
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{
		ID: "r1", Name: "high cpu", NotifyURL: srv.URL + "/my-topic?auth=tk_secret", NotifyKind: NotifyNtfy,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r, Resolved: true}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if gotAuth != "Bearer tk_secret" {
		t.Errorf("Authorization header = %q, want Bearer tk_secret", gotAuth)
	}
	if gotPath != "/my-topic" {
		t.Errorf("request URI = %q, want the auth query parameter stripped", gotPath)
	}
	if got.Priority != 3 {
		t.Errorf("payload.Priority = %d, want 3 for a resolved event", got.Priority)
	}
}

func TestNotifyNtfy_InvalidURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "x", NotifyURL: "://not a url", NotifyKind: NotifyNtfy}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error for an unparseable notify_url")
	}
}

func TestNotifyResend_PostsAuthHeaderAndPayload(t *testing.T) {
	var got resendPayload
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	original := resendAPIURL
	resendAPIURL = srv.URL
	t.Cleanup(func() { resendAPIURL = original })

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		NotifyURL:  "https://api.resend.com/emails?key=re_secret123&to=ops%40example.com",
		NotifyKind: NotifyResend,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if gotAuth != "Bearer re_secret123" {
		t.Errorf("Authorization header = %q, want Bearer re_secret123", gotAuth)
	}
	if len(got.To) != 1 || got.To[0] != "ops@example.com" {
		t.Errorf("payload.To = %v, want [ops@example.com]", got.To)
	}
	if got.From != resendDefaultFrom {
		t.Errorf("payload.From = %q, want the default sandbox sender %q when no from param is given", got.From, resendDefaultFrom)
	}
	if !strings.Contains(got.Subject, "high cpu") {
		t.Errorf("payload.Subject = %q, want it to mention the rule name", got.Subject)
	}
}

func TestNotifyResend_CustomFromParam(t *testing.T) {
	var got resendPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	original := resendAPIURL
	resendAPIURL = srv.URL
	t.Cleanup(func() { resendAPIURL = original })

	r := Rule{
		ID: "r1", Name: "x",
		NotifyURL:  "https://api.resend.com/emails?key=re_secret&to=ops%40example.com&from=alerts%40example.com",
		NotifyKind: NotifyResend,
	}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.From != "alerts@example.com" {
		t.Errorf("payload.From = %q, want the explicit from param", got.From)
	}
}

func TestNotifyResend_ResolvedEvent_SubjectMarksResolved(t *testing.T) {
	var got resendPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	original := resendAPIURL
	resendAPIURL = srv.URL
	t.Cleanup(func() { resendAPIURL = original })

	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: "https://api.resend.com/emails?key=k&to=ops%40example.com", NotifyKind: NotifyResend}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r, Resolved: true}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !strings.Contains(got.Subject, "RESOLVED") {
		t.Errorf("payload.Subject = %q, want it to mention RESOLVED", got.Subject)
	}
}

func TestNotifyResend_MissingCreds_Errors(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"missing key", "https://api.resend.com/emails?to=ops%40example.com"},
		{"missing to", "https://api.resend.com/emails?key=re_secret"},
		{"missing both", "https://api.resend.com/emails"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{ID: "r1", Name: "x", NotifyURL: tt.url, NotifyKind: NotifyResend}
			notifier := NewNotifier(nil, nil, r)

			if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
				t.Error("Notify() error = nil, want an error when notify_url is missing key or to")
			}
		})
	}
}

func TestNotifyResend_InvalidURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "x", NotifyURL: "://not a url", NotifyKind: NotifyResend}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error for an unparseable notify_url")
	}
}

func TestNewNotifier_AllValidKinds_Recognized(t *testing.T) {
	kinds := []NotifyKind{
		NotifyGeneric, NotifySlack, NotifyDiscord, NotifyTelegram, NotifyPushover,
		NotifyPagerDuty, NotifyTeams, NotifyMattermost, NotifyLark, NotifyGotify,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			notifyURL := srv.URL
			switch kind {
			case NotifyTelegram:
				notifyURL = srv.URL + "?chat_id=1"
			case NotifyPushover:
				notifyURL = srv.URL + "?token=t&user=u"
			case NotifyPagerDuty:
				notifyURL = "routing-key"
			case NotifyGotify:
				notifyURL = srv.URL + "?token=t"
			}
			r := Rule{ID: "r1", Name: "x", NotifyURL: notifyURL, NotifyKind: kind}
			if kind == NotifyPagerDuty {
				withPagerDutyEventsURL(t, srv.URL)
			}
			notifier := NewNotifier(nil, nil, r)
			if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
				t.Errorf("Notify() error = %v for kind %q", err, kind)
			}
		})
	}
}

func TestNewNotifier_UnknownKind_FallsBackToGeneric(t *testing.T) {
	var got genericPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := Rule{ID: "r1", Name: "x", NotifyURL: srv.URL, NotifyKind: "typo'd-kind"}
	notifier := NewNotifier(nil, nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.RuleID != "r1" {
		t.Errorf("payload = %+v, want the generic shape (unknown NotifyKind falls back to generic)", got)
	}
}
