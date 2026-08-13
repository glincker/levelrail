package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestNewNotifier_Email_NoSMTPConfigured_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: "ops@example.com", NotifyKind: NotifyEmail}
	notifier := NewNotifier(nil, nil, r) // no SMTPConfig

	err := notifier.Notify(context.Background(), Event{Rule: r})
	if err == nil {
		t.Fatal("Notify() error = nil, want a clear 'not configured' error when no SMTP server is set up")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %q, want it to say email is not configured", err.Error())
	}
}

func TestNewNotifier_Email_NoDestinationAddress_Errors(t *testing.T) {
	cfg := &SMTPConfig{Addr: "smtp.example.com:587", Host: "smtp.example.com", From: "alerts@example.com"}
	r := Rule{ID: "r1", Name: "high cpu", NotifyKind: NotifyEmail} // NotifyURL (the "to" address) left empty
	notifier := NewNotifier(nil, cfg, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when no destination address is configured")
	}
}

func TestNewNotifier_Email_UnreachableServer_ErrorPropagates(t *testing.T) {
	// Not a real send: no SMTP fixture here, this only proves
	// emailNotifier actually calls through to net/smtp.SendMail and
	// wraps whatever it returns, rather than silently swallowing a
	// connection failure.
	cfg := &SMTPConfig{Addr: "127.0.0.1:1", Host: "127.0.0.1", From: "alerts@example.com"}
	r := Rule{ID: "r1", Name: "high cpu", NotifyURL: "ops@example.com", NotifyKind: NotifyEmail}
	notifier := NewNotifier(nil, cfg, r)

	err := notifier.Notify(context.Background(), Event{Rule: r})
	if err == nil {
		t.Fatal("Notify() error = nil, want an error when the SMTP server is unreachable")
	}
	if !strings.Contains(err.Error(), "send email") {
		t.Errorf("error = %q, want it wrapped with \"send email\"", err.Error())
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
