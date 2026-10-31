package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestNotificationChannels_SealedSecretsNeverLeaveTheServer(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true") // receiver is an httptest server on loopback
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	adb.SetChannelSecrets(secrets.NewManager(db, mk))
	cookie := loginTestSession(t, rt, db)

	const token = "T0-B0-sealedtoken" //nolint:gosec // test fixture, not a credential
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Seeded directly: the save-time shape check only accepts vendor webhook hosts, never a local receiver.
	if err := adb.SaveNotificationChannel(context.Background(), alerting.NotificationChannel{
		ID: "chn_sealed", Name: "ops", Kind: alerting.NotifySlack, NotifyURL: srv.URL + "/hook/" + token,
		Enabled: true, InteractiveApprovals: true, InteractiveSecret: "signing-sealed",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/notification-channels", ""))
	var listed []notificationChannelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed) != 1 {
		t.Fatalf("list: %v %s", err, rec.Body.String())
	}
	created := listed[0]
	if !created.HasInteractiveSecret || !strings.HasSuffix(created.NotifyURL, "/(hidden)") {
		t.Errorf("created = %+v, want a masked target and has_interactive_secret", created)
	}

	var rawURL, rawSecret string
	if err := adb.QueryRowContext(context.Background(), `SELECT notify_url, interactive_secret FROM notification_channels WHERE id = ?`, created.ID).Scan(&rawURL, &rawSecret); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rawURL, token) || strings.Contains(rawSecret, "signing-sealed") {
		t.Fatalf("alerting.db row holds plaintext: %q / %q", rawURL, rawSecret)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/notification-channels", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), token) {
		t.Fatalf("root list leaked the credential: %d %s", rec.Code, rec.Body.String())
	}

	update := `{"name":"renamed","kind":"slack","notify_url":"` + created.NotifyURL + `","interactive_approvals":true}`
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/notification-channels/"+created.ID, update))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), token) {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/"+created.ID+"/test", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("test-send status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(paths) != 1 || paths[0] != "/hook/"+token {
		t.Fatalf("receiver paths = %v, want the decrypted URL after a masked round-trip edit", paths)
	}
}

func TestDoctorCheckNotificationChannelSecrets(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	ctx := context.Background()
	if got := rt.doctorCheckNotificationChannelSecrets(ctx); got.Status != doctorStatusOK {
		t.Fatalf("empty store: %+v, want ok", got)
	}
	if err := adb.SaveNotificationChannel(ctx, alerting.NotificationChannel{
		ID: "chn_plain", Name: "ops", Kind: alerting.NotifyTelegram, NotifyURL: "https://api.telegram.org/botX:Y/sendMessage?chat_id=1", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	got := rt.doctorCheckNotificationChannelSecrets(ctx)
	if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "no master key") {
		t.Fatalf("plaintext row without a master key: %+v, want a warning", got)
	}

	mk, _ := secrets.GenerateMasterKey()
	adb.SetChannelSecrets(secrets.NewManager(db, mk))
	if _, err := adb.SealNotificationChannelSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	if got := rt.doctorCheckNotificationChannelSecrets(ctx); got.Status != doctorStatusOK {
		t.Fatalf("after sealing: %+v, want ok", got)
	}
}
