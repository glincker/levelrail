package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

func fetchFeed(t *testing.T, rt *Router, req *http.Request) []attention.Item {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out attentionFeedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	return out.Items
}

func mintExpiringToken(t *testing.T, rt *Router, cookie *http.Cookie, name string, days int) {
	t.Helper()
	body := `{"name":"` + name + `","abilities":["read"],"expires_in_days":` + strconv.Itoa(days) + `}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint %q = %d, body = %s", name, rec.Code, rec.Body.String())
	}
}

func subjectsOfKind(items []attention.Item, kind string) []string {
	var out []string
	for _, it := range items {
		if it.Kind == kind {
			out = append(out, it.Subject)
		}
	}
	return out
}

func TestAttentionFeed_TokenVisibility(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead, AbilityWrite})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)

	mintExpiringToken(t, rt, admin, "admin-ci", 2)
	mintExpiringToken(t, rt, readerCookie, "reader-ci", 3)
	mintExpiringToken(t, rt, admin, "admin-far", 9)
	readOnly := mintTokenAs(t, rt, admin, "read-only")

	tests := []struct {
		name string
		req  *http.Request
		want []string
	}{
		{"root session sees every owner's tokens", authedRequest(t, admin, http.MethodGet, "/api/v1/attention/feed", ""), []string{"admin-ci", "reader-ci"}},
		{"user session sees only its own", authedRequest(t, readerCookie, http.MethodGet, "/api/v1/attention/feed", ""), []string{"reader-ci"}},
		{"read-only token gets the feed without token items", bearerRequest(http.MethodGet, "/api/v1/attention/feed", "", readOnly.Token), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subjectsOfKind(fetchFeed(t, rt, tt.req), attention.KindTokenExpiring)
			if len(got) != len(tt.want) {
				t.Fatalf("expiring tokens = %v, want %v", got, tt.want)
			}
			for _, w := range tt.want {
				found := false
				for _, g := range got {
					found = found || g == w
				}
				if !found {
					t.Errorf("missing %q in %v", w, got)
				}
			}
		})
	}

	anon := httptest.NewRecorder()
	rt.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/v1/attention/feed", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Errorf("anonymous feed = %d, want 401", anon.Code)
	}
}

func TestAttentionFeed_ItemShape(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mintExpiringToken(t, rt, admin, "ci", 2)
	items := fetchFeed(t, rt, authedRequest(t, admin, http.MethodGet, "/api/v1/attention/feed", ""))
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one", items)
	}
	it := items[0]
	if it.ID == "" || it.Severity != attention.Warning || it.Title == "" || it.Action == "" || it.Link != "/settings/tokens" {
		t.Errorf("item = %+v, want id, warning, title, action and a tokens link", it)
	}
	for _, field := range []string{it.ID, it.Title, it.Detail, it.Action, it.Link} {
		if strings.ContainsAny(field, "\u2014\u2013") {
			t.Errorf("dash in copy %q", field)
		}
	}
}

func TestDataCopyItem(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		imp  store.DatabaseDataImport
		want bool
		kind string
	}{
		{"failed", store.DatabaseDataImport{Status: store.DataImportFailed, Reason: "auth failed"}, true, "failed"},
		{"verified", store.DatabaseDataImport{Status: store.DataImportVerified}, false, ""},
		{"copying fresh", store.DatabaseDataImport{Status: store.DataImportCopying, StartedAt: now.Add(-time.Minute)}, false, ""},
		{"copying stalled", store.DatabaseDataImport{Status: store.DataImportCopying, StartedAt: now.Add(-48 * time.Hour)}, true, "stalled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it, ok := dataCopyItem("main", tt.imp, now)
			if ok != tt.want {
				t.Fatalf("ok = %v, want %v", ok, tt.want)
			}
			if ok && (it.Params["state"] != tt.kind || it.Kind != attention.KindDataCopy || it.Link != "/databases/main") {
				t.Errorf("item = %+v", it)
			}
		})
	}
}

func TestDeviceExpiry_NotifiesOptedInChannelsWithoutCode(t *testing.T) {
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	notifier := &recordingNotifier{done: make(chan struct{}, 4)}
	rt := NewRouter(nil, testBrand(), db,
		WithNotificationChannels(adb), WithDeviceLoginNotifier(notifier), WithDashboardURL("https://panel.example.com"))

	ctx := context.Background()
	for i, ch := range []alerting.NotificationChannel{
		{NotifyDeviceLoginExpired: true},
		{NotifyDeviceLogin: true},
		{},
	} {
		id, err := alerting.NewNotificationChannelID()
		if err != nil {
			t.Fatal(err)
		}
		ch.ID, ch.Name, ch.Kind = id, "c"+string(rune('a'+i)), alerting.NotifyGeneric
		ch.NotifyURL, ch.Enabled = "https://example.com/hook", true
		if err := adb.SaveNotificationChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}

	started := startDeviceAuth(t, rt, "laptop")
	select {
	case <-notifier.done:
	case <-time.After(3 * time.Second):
		t.Fatal("start did not notify the waiting-login channel")
	}
	expireDeviceRowForTest(t, db, started.UserCode, time.Now().Add(-time.Minute))
	if n, err := rt.SweepDeviceLoginExpiry(ctx, time.Now()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	select {
	case <-notifier.done:
	case <-time.After(3 * time.Second):
		t.Fatal("expiry did not notify the opted-in channel")
	}
	time.Sleep(100 * time.Millisecond)
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.texts) != 2 {
		t.Fatalf("notices = %d, want 2 (one waiting, one expired)", len(notifier.texts))
	}
	expired := notifier.texts[1]
	if strings.Contains(expired, started.UserCode) || strings.Contains(expired, started.DeviceCode) {
		t.Errorf("expiry notice leaked a code: %q", expired)
	}
	if !strings.Contains(expired, "expired") || !strings.Contains(expired, "https://panel.example.com/settings/cli-access") {
		t.Errorf("expiry notice = %q, want the expiry text and dashboard link", expired)
	}
}
