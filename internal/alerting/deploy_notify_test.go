package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/email"
)

func newTestDeployNotifyDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alerting.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test alerting db: %v", err)
		}
	})
	return db
}

func TestSaveGetDeployTarget_RoundTrips(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	target := DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://example.com/hook", NotifyKind: NotifySlack, Enabled: true}
	if err := db.SaveDeployTarget(ctx, target); err != nil {
		t.Fatalf("SaveDeployTarget() error = %v", err)
	}

	got, err := db.GetDeployTarget(ctx, "dnt_1")
	if err != nil {
		t.Fatalf("GetDeployTarget() error = %v", err)
	}
	if *got != target {
		t.Errorf("GetDeployTarget() = %+v, want %+v", *got, target)
	}
}

func TestGetDeployTarget_NotFound(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	if _, err := db.GetDeployTarget(context.Background(), "ghost"); err != ErrDeployTargetNotFound {
		t.Errorf("GetDeployTarget() error = %v, want ErrDeployTargetNotFound", err)
	}
}

func TestSaveDeployTarget_UpsertsOnConflict(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://old.example.com", NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://new.example.com", NotifyKind: NotifySlack, Enabled: false}); err != nil {
		t.Fatalf("upsert save: %v", err)
	}

	got, err := db.GetDeployTarget(ctx, "dnt_1")
	if err != nil {
		t.Fatalf("GetDeployTarget() error = %v", err)
	}
	if got.NotifyURL != "https://new.example.com" || got.NotifyKind != NotifySlack || got.Enabled {
		t.Errorf("GetDeployTarget() after upsert = %+v, want updated fields", *got)
	}
}

func TestListDeployTargetsForResource_ScopesByResource(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://a.example.com", NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_2", ResourceID: "service:web", NotifyURL: "https://b.example.com", NotifyKind: NotifySlack, Enabled: false}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_3", ResourceID: "service:worker", NotifyURL: "https://c.example.com", NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := db.ListDeployTargetsForResource(ctx, "service:web")
	if err != nil {
		t.Fatalf("ListDeployTargetsForResource() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d targets, want 2 (including the disabled one, excluding service:worker's)", len(got))
	}
}

func TestDeleteDeployTarget_IdempotentOnMissing(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	if err := db.DeleteDeployTarget(context.Background(), "ghost"); err != nil {
		t.Errorf("DeleteDeployTarget() on a missing id error = %v, want nil (idempotent delete)", err)
	}
}

func TestDeleteDeployTarget_RemovesRow(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://example.com", NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.DeleteDeployTarget(ctx, "dnt_1"); err != nil {
		t.Fatalf("DeleteDeployTarget() error = %v", err)
	}
	if _, err := db.GetDeployTarget(ctx, "dnt_1"); err != ErrDeployTargetNotFound {
		t.Errorf("GetDeployTarget() after delete error = %v, want ErrDeployTargetNotFound", err)
	}
}

func TestSummaryDeployText(t *testing.T) {
	succeeded := summaryDeployText(DeployOutcome{AppName: "web", Image: "levelrail/web:abc123", Succeeded: true})
	if !strings.Contains(succeeded, "SUCCEEDED") || !strings.Contains(succeeded, "web") || !strings.Contains(succeeded, "levelrail/web:abc123") {
		t.Errorf("summaryDeployText(succeeded) = %q, want it to mention SUCCEEDED, the app name, and the image", succeeded)
	}

	failed := summaryDeployText(DeployOutcome{AppName: "web", Image: "levelrail/web:abc123", Succeeded: false, Error: "buildkit: boom"})
	if !strings.Contains(failed, "FAILED") || !strings.Contains(failed, "buildkit: boom") {
		t.Errorf("summaryDeployText(failed) = %q, want it to mention FAILED and the error detail", failed)
	}
}

func TestSendDeployOutcome_Slack_PostsTextField(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := DeployTarget{NotifyURL: srv.URL, NotifyKind: NotifySlack, Enabled: true}
	err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})
	if err != nil {
		t.Fatalf("sendDeployOutcome() error = %v", err)
	}
	text, ok := got["text"].(string)
	if !ok || !strings.Contains(text, "web") || !strings.Contains(text, "SUCCEEDED") {
		t.Errorf("Slack payload = %+v, want a text field mentioning the app and SUCCEEDED", got)
	}
}

func TestSendDeployOutcome_Discord_PostsContentField(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := DeployTarget{NotifyURL: srv.URL, NotifyKind: NotifyDiscord, Enabled: true}
	err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web", Image: "web:1", Succeeded: false, Error: "boom"})
	if err != nil {
		t.Fatalf("sendDeployOutcome() error = %v", err)
	}
	content, ok := got["content"].(string)
	if !ok || !strings.Contains(content, "FAILED") || !strings.Contains(content, "boom") {
		t.Errorf("Discord payload = %+v, want content mentioning FAILED and the error", got)
	}
}

func TestSendDeployOutcome_Telegram_PostsChatIDAndText(t *testing.T) {
	var got telegramPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := DeployTarget{NotifyURL: srv.URL + "/bot123/sendMessage?chat_id=42", NotifyKind: NotifyTelegram, Enabled: true}
	err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})
	if err != nil {
		t.Fatalf("sendDeployOutcome() error = %v", err)
	}
	if got.ChatID != "42" {
		t.Errorf("ChatID = %q, want %q", got.ChatID, "42")
	}
	if !strings.Contains(got.Text, "web") {
		t.Errorf("Text = %q, want it to mention the app", got.Text)
	}
}

func TestSendDeployOutcome_Telegram_MissingChatID_Errors(t *testing.T) {
	target := DeployTarget{NotifyURL: "https://example.com/bot123/sendMessage", NotifyKind: NotifyTelegram, Enabled: true}
	if err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web"}); err == nil {
		t.Error("sendDeployOutcome() error = nil, want an error when notify_url has no chat_id query parameter")
	}
}

func TestSendDeployOutcome_Generic_PostsStructuredPayload(t *testing.T) {
	var got deployGenericPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := DeployTarget{NotifyURL: srv.URL, NotifyKind: NotifyGeneric, Enabled: true}
	err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web", Image: "web:1", Succeeded: false, Error: "boom"})
	if err != nil {
		t.Fatalf("sendDeployOutcome() error = %v", err)
	}
	if got.AppName != "web" || got.Image != "web:1" || got.Succeeded || got.Error != "boom" {
		t.Errorf("generic payload = %+v, want the full structured deploy outcome", got)
	}
}

func TestSendDeployOutcome_UnknownKind_FallsBackToGeneric(t *testing.T) {
	var got deployGenericPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := DeployTarget{NotifyURL: srv.URL, NotifyKind: "typo'd-kind", Enabled: true}
	err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})
	if err != nil {
		t.Fatalf("sendDeployOutcome() error = %v", err)
	}
	if got.AppName != "web" {
		t.Errorf("payload = %+v, want the generic shape (unknown NotifyKind falls back to generic)", got)
	}
}

func TestSendDeployOutcome_Email_NoSMTPConfigured_Errors(t *testing.T) {
	target := DeployTarget{NotifyURL: "ops@example.com", NotifyKind: NotifyEmail, Enabled: true}
	err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("sendDeployOutcome() error = %v, want a clear 'not configured' error", err)
	}
}

func TestSendDeployOutcome_Email_NoDestinationAddress_Errors(t *testing.T) {
	sender, err := email.NewSender(email.Config{Backend: email.BackendSMTP, SMTP: &email.SMTPConfig{Addr: "smtp.example.com:587", Host: "smtp.example.com", From: "alerts@example.com"}})
	if err != nil {
		t.Fatalf("email.NewSender() error = %v", err)
	}
	target := DeployTarget{NotifyKind: NotifyEmail, Enabled: true} // NotifyURL left empty
	if err := sendDeployOutcome(context.Background(), nil, sender, target, DeployOutcome{AppName: "web"}); err == nil {
		t.Error("sendDeployOutcome() error = nil, want an error when no destination address is configured")
	}
}

func TestSendDeployOutcome_ReceiverErrorStatus_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	target := DeployTarget{NotifyURL: srv.URL, NotifyKind: NotifyGeneric, Enabled: true}
	if err := sendDeployOutcome(context.Background(), nil, nil, target, DeployOutcome{AppName: "web"}); err == nil {
		t.Error("sendDeployOutcome() error = nil, want an error when the receiver returns a non-2xx status")
	}
}

// fakeDeployTargetStore-free: DeployDispatcher.Dispatch is exercised
// against a real *DB (same "real, not mocked" convention this package's
// own engine_test.go uses for RuleStore) rather than a fake, since the
// interesting behavior here (loading targets scoped to a resource,
// skipping disabled ones) is genuinely a database query, not logic worth
// faking around.

func TestDeployDispatcher_Dispatch_SendsToEnabledTargetsOnly(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_2", ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyGeneric, Enabled: false}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_3", ResourceID: "service:worker", NotifyURL: srv.URL, NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	dispatcher := NewDeployDispatcher(db, nil, nil, nil)
	dispatcher.Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})

	if received != 1 {
		t.Errorf("receiver got %d requests, want 1 (only the enabled service:web target)", received)
	}
}

// TestDeployDispatcher_Dispatch_MixedLegacyAndChannelTargets proves a
// legacy (no ChannelID) target and a channel-attached target on the
// same app both fire correctly from one Dispatch call.
func TestDeployDispatcher_Dispatch_MixedLegacyAndChannelTargets(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	var legacyHit, channelHit bool
	legacySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		legacyHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer legacySrv.Close()
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		channelHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer channelSrv.Close()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifyGeneric, NotifyURL: channelSrv.URL, Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_legacy", ResourceID: "service:web", NotifyURL: legacySrv.URL, NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed legacy target: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_channel", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("seed channel-attached target: %v", err)
	}

	dispatcher := NewDeployDispatcher(db, nil, nil, nil)
	dispatcher.Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})

	if !legacyHit {
		t.Error("legacy target never received a request")
	}
	if !channelHit {
		t.Error("channel-attached target never received a request")
	}
}

func TestDeployDispatcher_Dispatch_NoTargets_NoPanic(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	dispatcher := NewDeployDispatcher(db, nil, nil, nil)
	// Must not panic or block: a resource with zero configured targets is
	// the common case (deploy-outcome notifications are opt-in per app).
	dispatcher.Dispatch(context.Background(), "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})
}

// TestDeployDispatcher_Dispatch_RecordsDeliveryForChannelAttachedTarget
// proves a real deploy-outcome send writes delivery history for a
// channel-attached target, and that a legacy (no ChannelID) target
// writes none: there is no channel-scoped history to write to for a
// legacy target.
func TestDeployDispatcher_Dispatch_RecordsDeliveryForChannelAttachedTarget(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifyGeneric, NotifyURL: srv.URL, Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_channel", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("seed channel-attached target: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_legacy", ResourceID: "service:web", NotifyURL: srv.URL, NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed legacy target: %v", err)
	}

	dispatcher := NewDeployDispatcher(db, nil, nil, nil)
	dispatcher.Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})

	deliveries, err := db.ListNotificationDeliveries(ctx, "chn_1", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("got %d deliveries for chn_1, want 1 (only the channel-attached target writes history)", len(deliveries))
	}
	if deliveries[0].Trigger != "deploy-succeeded" || !deliveries[0].Success {
		t.Errorf("delivery = %+v, want a successful deploy-succeeded row", deliveries[0])
	}
}

// TestDeployDispatcher_Dispatch_RecordsFailedDelivery proves a failed
// send still records a delivery row with its error detail, and the
// deploy-failed trigger.
func TestDeployDispatcher_Dispatch_RecordsFailedDelivery(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifyGeneric, NotifyURL: "http://127.0.0.1:1/hook", Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	dispatcher := NewDeployDispatcher(db, nil, nil, nil)
	dispatcher.Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: false, Error: "buildkit: boom"})

	deliveries, err := db.ListNotificationDeliveries(ctx, "chn_1", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("got %d deliveries, want 1", len(deliveries))
	}
	if deliveries[0].Trigger != "deploy-failed" || deliveries[0].Success || deliveries[0].Error == "" {
		t.Errorf("delivery = %+v, want a failed deploy-failed row with a non-empty error", deliveries[0])
	}
}

func TestDeployDispatcher_Dispatch_TargetSendFailure_DoesNotPanic(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "", NotifyKind: NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	dispatcher := NewDeployDispatcher(db, nil, nil, nil)
	// An empty NotifyURL makes postJSON fail immediately (notify.go's own
	// "no notify_url configured" check): Dispatch must log and continue,
	// not panic or propagate, matching its own doc comment.
	dispatcher.Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})
}
