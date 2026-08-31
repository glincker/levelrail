package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithNotificationChannels wires a real (not faked) tester:
// the test-send endpoint's whole point is proving a connection works.
func newTestRouterWithNotificationChannels(t *testing.T) (*Router, *store.DB, *alerting.DB) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	tester := alerting.NewDeployDispatcher(adb, nil, nil, nil)
	rt := NewRouter(nil, testBrand(), db,
		WithNotificationChannels(adb), WithNotificationChannelTester(tester), WithNotificationDeliveries(adb))
	return rt, db, adb
}

func TestNotificationChannelRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithNotificationChannels
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		method, target, body string
	}{
		{http.MethodGet, "/api/v1/notification-channels", ""},
		{http.MethodPost, "/api/v1/notification-channels", `{"name":"x","kind":"slack","notify_url":"https://example.com"}`},
		{http.MethodDelete, "/api/v1/notification-channels/whatever", ""},
		{http.MethodPost, "/api/v1/notification-channels/test", `{"kind":"slack","notify_url":"https://example.com"}`},
		{http.MethodPost, "/api/v1/notification-channels/whatever/test", ""},
		{http.MethodGet, "/api/v1/notification-channels/whatever/deliveries", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, tt.target, tt.body))
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateNotificationChannel_Success(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"Team Slack","kind":"slack","notify_url":"https://hooks.slack.com/x"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got notificationChannelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a server-generated ID")
	}
	if got.Name != "Team Slack" || got.Kind != "slack" || got.NotifyURL != "https://hooks.slack.com/x" || !got.Enabled {
		t.Errorf("got = %+v, want matching fields from the request body, Enabled defaulted true", got)
	}
}

func TestHandleCreateNotificationChannel_PushoverKindAccepted(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"My Phone","kind":"pushover","notify_url":"https://api.pushover.net/1/messages.json?token=app-token&user=user-key"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got notificationChannelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "pushover" {
		t.Errorf("Kind = %q, want pushover", got.Kind)
	}
}

func TestHandleCreateNotificationChannel_EnabledFalseRespected(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"Paused","kind":"generic","notify_url":"https://example.com","enabled":false}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got notificationChannelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Error("Enabled = true, want false: enabled:false was explicit in the request")
	}
}

func TestHandleCreateNotificationChannel_ValidationFailures(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"missing name", `{"kind":"slack","notify_url":"https://example.com"}`},
		{"missing notify_url", `{"name":"x","kind":"slack"}`},
		{"bad kind", `{"name":"x","kind":"bogus","notify_url":"https://example.com"}`},
		{"malformed body", `{not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleListNotificationChannels(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://a.example.com")
	seedNotificationChannel(t, adb, "chn_2", alerting.NotifyDiscord, "https://b.example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/notification-channels", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []notificationChannelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d channels, want 2", len(got))
	}
}

func TestHandleDeleteNotificationChannel_Success(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/notification-channels/chn_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleDeleteNotificationChannel_NotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/notification-channels/nonexistent", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleDeleteNotificationChannel_ClearsAttachedAppTarget is the
// HTTP-level counterpart to the alerting package's own FK-clearing test.
func TestHandleDeleteNotificationChannel_ClearsAttachedAppTarget(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	rt.deployNotifyTargets = adb
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://example.com")

	createBody := `{"channel_id":"chn_1","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploy-notify-targets", createBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create target status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created deployTargetResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/notification-channels/chn_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete channel status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	got, err := adb.GetDeployTarget(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetDeployTarget() after channel delete error = %v, want the app's target row to still exist", err)
	}
	if got.ChannelID != "" {
		t.Errorf("ChannelID = %q, want empty after the channel it attached to was deleted", got.ChannelID)
	}
}

func TestHandleTestNotificationChannel_Success(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body := `{"kind":"generic","notify_url":"` + srv.URL + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/test", body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if received != 1 {
		t.Errorf("receiver got %d requests, want 1", received)
	}
}

// 127.0.0.1:1 is this codebase's existing "deliberately unreachable"
// convention (internal/alerting/notify_test.go).
func TestHandleTestNotificationChannel_UnreachableURL_Fails(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"kind":"generic","notify_url":"http://127.0.0.1:1/hook"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/test", body))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestHandleTestNotificationChannel_ValidationFailures(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"missing notify_url", `{"kind":"slack"}`},
		{"bad kind", `{"kind":"bogus","notify_url":"https://example.com"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/test", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleTestExistingNotificationChannel_Success(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifyGeneric, srv.URL)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/chn_1/test", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if received != 1 {
		t.Errorf("receiver got %d requests, want 1", received)
	}
}

func TestHandleTestExistingNotificationChannel_NotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/ghost/test", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleTestExistingNotificationChannel_RecordsDelivery proves a
// test-send against an already-saved channel writes a delivery-history
// row, not just a log line: the CLI/UI's own "when did this last work"
// question depends on that write actually happening.
func TestHandleTestExistingNotificationChannel_RecordsDelivery(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifyGeneric, srv.URL)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/chn_1/test", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	deliveries, err := adb.ListNotificationDeliveries(t.Context(), "chn_1", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("got %d deliveries, want 1", len(deliveries))
	}
	if deliveries[0].Trigger != "test" || !deliveries[0].Success {
		t.Errorf("delivery = %+v, want a successful test-trigger row", deliveries[0])
	}
}

// TestHandleTestExistingNotificationChannel_RecordsFailedDelivery proves
// a failed test-send still records a delivery row, with its error detail
// preserved: the whole point of delivery history is surfacing failures,
// not just successes.
func TestHandleTestExistingNotificationChannel_RecordsFailedDelivery(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifyGeneric, "http://127.0.0.1:1/hook")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels/chn_1/test", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	deliveries, err := adb.ListNotificationDeliveries(t.Context(), "chn_1", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("got %d deliveries, want 1", len(deliveries))
	}
	if deliveries[0].Success || deliveries[0].Error == "" {
		t.Errorf("delivery = %+v, want a failed row with a non-empty error", deliveries[0])
	}
}

func TestHandleListNotificationDeliveries_NewestFirst(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifyGeneric, "https://example.com")
	if err := adb.RecordNotificationDelivery(t.Context(), alerting.NotificationDelivery{ID: "ndl_1", ChannelID: "chn_1", Trigger: "deploy-succeeded", Success: true}); err != nil {
		t.Fatalf("seed delivery 1: %v", err)
	}
	if err := adb.RecordNotificationDelivery(t.Context(), alerting.NotificationDelivery{ID: "ndl_2", ChannelID: "chn_1", Trigger: "deploy-failed", Success: false, Error: "boom"}); err != nil {
		t.Fatalf("seed delivery 2: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/notification-channels/chn_1/deliveries", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []notificationDeliveryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d deliveries, want 2", len(got))
	}
	if got[0].ID != "ndl_2" || got[1].ID != "ndl_1" {
		t.Errorf("got IDs [%s, %s], want [ndl_2, ndl_1] (newest first)", got[0].ID, got[1].ID)
	}
}

func TestHandleListNotificationDeliveries_ChannelNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/notification-channels/ghost/deliveries", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListNotificationDeliveries_InvalidLimit(t *testing.T) {
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	cookie := loginTestSession(t, rt, db)
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifyGeneric, "https://example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/notification-channels/chn_1/deliveries?limit=0", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestNotificationChannelRoutes_RequireAuth(t *testing.T) {
	rt, db, _ := newTestRouterWithNotificationChannels(t)
	_ = db

	tests := []struct {
		method, target string
	}{
		{http.MethodGet, "/api/v1/notification-channels"},
		{http.MethodPost, "/api/v1/notification-channels"},
		{http.MethodDelete, "/api/v1/notification-channels/chn_1"},
		{http.MethodPost, "/api/v1/notification-channels/test"},
		{http.MethodPost, "/api/v1/notification-channels/chn_1/test"},
		{http.MethodGet, "/api/v1/notification-channels/chn_1/deliveries"},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}
