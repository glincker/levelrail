package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSaveGetNotificationChannel_RoundTrips(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	channel := NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifySlack, NotifyURL: "https://hooks.slack.com/x", Enabled: true}
	if err := db.SaveNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("SaveNotificationChannel() error = %v", err)
	}

	got, err := db.GetNotificationChannel(ctx, "chn_1")
	if err != nil {
		t.Fatalf("GetNotificationChannel() error = %v", err)
	}
	if got.Name != channel.Name || got.Kind != channel.Kind || got.NotifyURL != channel.NotifyURL || !got.Enabled {
		t.Errorf("GetNotificationChannel() = %+v, want matching %+v", *got, channel)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Error("CreatedAt/UpdatedAt are empty, want server-assigned timestamps")
	}
}

func TestGetNotificationChannel_NotFound(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	if _, err := db.GetNotificationChannel(context.Background(), "ghost"); err != ErrNotificationChannelNotFound {
		t.Errorf("GetNotificationChannel() error = %v, want ErrNotificationChannelNotFound", err)
	}
}

func TestListNotificationChannels_OldestFirst(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "First", Kind: NotifyGeneric, NotifyURL: "https://a.example.com", Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_2", Name: "Second", Kind: NotifyDiscord, NotifyURL: "https://b.example.com", Enabled: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := db.ListNotificationChannels(ctx)
	if err != nil {
		t.Fatalf("ListNotificationChannels() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "chn_1" || got[1].ID != "chn_2" {
		t.Errorf("ListNotificationChannels() = %+v, want [chn_1, chn_2] in creation order", got)
	}
}

func TestDeleteNotificationChannel_NotFound(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	if err := db.DeleteNotificationChannel(context.Background(), "ghost"); err != ErrNotificationChannelNotFound {
		t.Errorf("DeleteNotificationChannel() error = %v, want ErrNotificationChannelNotFound", err)
	}
}

// TestDeleteNotificationChannel_ClearsAttachedDeployTargets proves
// migrations/0004's ON DELETE SET NULL: deleting a channel an app is
// attached to must clear channel_id, not block the delete or the row.
func TestDeleteNotificationChannel_ClearsAttachedDeployTargets(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifySlack, NotifyURL: "https://hooks.slack.com/x", Enabled: true}); err != nil {
		t.Fatalf("SaveNotificationChannel() error = %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("SaveDeployTarget() error = %v", err)
	}

	if err := db.DeleteNotificationChannel(ctx, "chn_1"); err != nil {
		t.Fatalf("DeleteNotificationChannel() error = %v", err)
	}

	got, err := db.GetDeployTarget(ctx, "dnt_1")
	if err != nil {
		t.Fatalf("GetDeployTarget() after channel delete error = %v, want the row to still exist", err)
	}
	if got.ChannelID != "" {
		t.Errorf("ChannelID = %q, want empty after the channel it pointed at was deleted", got.ChannelID)
	}
	if got.NotifyURL != "" {
		t.Errorf("NotifyURL = %q, want empty: this row never had its own notify_url, only a now-deleted channel", got.NotifyURL)
	}
}

// TestDeployTarget_ResolvesFromAttachedChannel proves a target created
// with only a ChannelID resolves NotifyURL/NotifyKind from the channel.
func TestDeployTarget_ResolvesFromAttachedChannel(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifySlack, NotifyURL: "https://hooks.slack.com/x", Enabled: true}); err != nil {
		t.Fatalf("SaveNotificationChannel() error = %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("SaveDeployTarget() error = %v", err)
	}

	got, err := db.GetDeployTarget(ctx, "dnt_1")
	if err != nil {
		t.Fatalf("GetDeployTarget() error = %v", err)
	}
	if got.NotifyURL != "https://hooks.slack.com/x" || got.NotifyKind != NotifySlack {
		t.Errorf("GetDeployTarget() = %+v, want NotifyURL/NotifyKind resolved from the attached channel", *got)
	}
}

// TestDeployTarget_BackwardCompat_NoChannelID proves a legacy row (no
// ChannelID) resolves from its own NotifyURL/NotifyKind columns.
func TestDeployTarget_BackwardCompat_NoChannelID(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://legacy.example.com/hook", NotifyKind: NotifyDiscord, Enabled: true}); err != nil {
		t.Fatalf("SaveDeployTarget() error = %v", err)
	}

	got, err := db.GetDeployTarget(ctx, "dnt_1")
	if err != nil {
		t.Fatalf("GetDeployTarget() error = %v", err)
	}
	if got.ChannelID != "" {
		t.Errorf("ChannelID = %q, want empty for a legacy row", got.ChannelID)
	}
	if got.NotifyURL != "https://legacy.example.com/hook" || got.NotifyKind != NotifyDiscord {
		t.Errorf("GetDeployTarget() = %+v, want the row's own legacy NotifyURL/NotifyKind unchanged", *got)
	}
}

// TestDeployTarget_DisabledChannel_SilencesTarget proves a disabled
// channel silences an attached target even if the target is enabled.
func TestDeployTarget_DisabledChannel_SilencesTarget(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Paused", Kind: NotifyGeneric, NotifyURL: "https://example.com/hook", Enabled: false}); err != nil {
		t.Fatalf("SaveNotificationChannel() error = %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("SaveDeployTarget() error = %v", err)
	}

	got, err := db.GetDeployTarget(ctx, "dnt_1")
	if err != nil {
		t.Fatalf("GetDeployTarget() error = %v", err)
	}
	if got.Enabled {
		t.Error("Enabled = true, want false: the attached channel itself is disabled")
	}
}

func TestSendTestNotification_Slack_Success(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := sendTestNotification(context.Background(), nil, nil, NotifySlack, srv.URL); err != nil {
		t.Fatalf("sendTestNotification() error = %v", err)
	}
	text, _ := got["text"].(string)
	if !strings.Contains(text, "test notification") {
		t.Errorf("Slack payload = %+v, want a text field mentioning the test", got)
	}
}

func TestSendTestNotification_Mattermost_Success(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := sendTestNotification(context.Background(), nil, nil, NotifyMattermost, srv.URL); err != nil {
		t.Fatalf("sendTestNotification() error = %v", err)
	}
	text, _ := got["text"].(string)
	if !strings.Contains(text, "test notification") {
		t.Errorf("Mattermost payload = %+v, want a text field mentioning the test", got)
	}
}

func TestSendTestNotification_Lark_Success(t *testing.T) {
	var got larkPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := sendTestNotification(context.Background(), nil, nil, NotifyLark, srv.URL); err != nil {
		t.Fatalf("sendTestNotification() error = %v", err)
	}
	if got.MsgType != "text" || !strings.Contains(got.Content.Text, "test notification") {
		t.Errorf("Lark payload = %+v, want msg_type text mentioning the test", got)
	}
}

func TestSendTestNotification_Gotify_Success(t *testing.T) {
	var got gotifyPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := sendTestNotification(context.Background(), nil, nil, NotifyGotify, srv.URL+"/message?token=t"); err != nil {
		t.Fatalf("sendTestNotification() error = %v", err)
	}
	if !strings.Contains(got.Message, "test notification") {
		t.Errorf("Gotify payload = %+v, want a message field mentioning the test", got)
	}
}

func TestSendTestNotification_Ntfy_Success(t *testing.T) {
	var got ntfyPayload
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := sendTestNotification(context.Background(), nil, nil, NotifyNtfy, srv.URL+"/topic?auth=tk"); err != nil {
		t.Fatalf("sendTestNotification() error = %v", err)
	}
	if gotAuth != "Bearer tk" {
		t.Errorf("Authorization header = %q, want Bearer tk", gotAuth)
	}
	if !strings.Contains(got.Message, "test notification") {
		t.Errorf("ntfy payload = %+v, want a message field mentioning the test", got)
	}
}

func TestSendTestNotification_Resend_Success(t *testing.T) {
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

	notifyURL := "https://api.resend.com/emails?key=re_secret&to=ops%40example.com"
	if err := sendTestNotification(context.Background(), nil, nil, NotifyResend, notifyURL); err != nil {
		t.Fatalf("sendTestNotification() error = %v", err)
	}
	if gotAuth != "Bearer re_secret" {
		t.Errorf("Authorization header = %q, want Bearer re_secret", gotAuth)
	}
	if len(got.To) != 1 || got.To[0] != "ops@example.com" {
		t.Errorf("payload.To = %v, want [ops@example.com]", got.To)
	}
}

// 127.0.0.1:1 is this codebase's existing "deliberately unreachable"
// convention (internal/alerting/notify_test.go).
func TestSendTestNotification_UnreachableURL_Errors(t *testing.T) {
	err := sendTestNotification(context.Background(), nil, nil, NotifyGeneric, "http://127.0.0.1:1/hook")
	if err == nil {
		t.Error("sendTestNotification() error = nil, want an error for an unreachable URL")
	}
}

func TestDeployDispatcher_SendTest_UsesDispatcherClient(t *testing.T) {
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dispatcher := NewDeployDispatcher(nil, nil, nil, nil)
	if err := dispatcher.SendTest(context.Background(), NotifyGeneric, srv.URL); err != nil {
		t.Fatalf("SendTest() error = %v", err)
	}
	if received != 1 {
		t.Errorf("receiver got %d requests, want 1", received)
	}
}
