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
	notifier := NewNotifier(nil, r)

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
	notifier := NewNotifier(nil, r)

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
	notifier := NewNotifier(nil, r)

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
	notifier := NewNotifier(nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r, Resolved: true}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !gotBody.Resolved {
		t.Error("payload.Resolved = false, want true")
	}
}

func TestNotify_NoURL_Errors(t *testing.T) {
	r := Rule{ID: "r1", Name: "high cpu"}
	notifier := NewNotifier(nil, r)

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
	notifier := NewNotifier(nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err == nil {
		t.Error("Notify() error = nil, want an error when the receiver returns a non-2xx status")
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
	notifier := NewNotifier(nil, r)

	if err := notifier.Notify(context.Background(), Event{Rule: r}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got.RuleID != "r1" {
		t.Errorf("payload = %+v, want the generic shape (unknown NotifyKind falls back to generic)", got)
	}
}
