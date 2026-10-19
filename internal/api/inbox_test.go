package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

func newInboxRouter(t *testing.T) (*Router, *alerting.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAlertRules(adb), WithNotificationChannels(adb), WithAlertNoise(adb), WithInbox(adb))
	return rt, adb, loginTestSession(t, rt, db)
}

func TestInboxRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/inbox", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestInboxLifecycle(t *testing.T) {
	rt, adb, cookie := newInboxRouter(t)
	ctx := context.Background()
	if err := adb.RecordHistory(ctx, alerting.HistoryEntry{At: time.Now().Add(-time.Minute), RuleID: "r1", RuleName: "cpu", Severity: alerting.SeverityCritical,
		Event: alerting.EventFired, Outcome: alerting.OutcomeSent}); err != nil {
		t.Fatal(err)
	}

	var list []inboxItemResource
	rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/inbox", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list) != 1 || list[0].Read {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	var count map[string]int
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/inbox/unread-count", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count["unread"] != 1 {
		t.Fatalf("unread = %v", count)
	}

	if rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/read", `{"ids":["`+list[0].ID+`"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("mark read = %d %s", rec.Code, rec.Body.String())
	}
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/inbox/unread-count", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count["unread"] != 0 {
		t.Fatalf("unread after read = %v", count)
	}
	if rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/read", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty mark read = %d, want 400", rec.Code)
	}
}

func TestInboxPreferencesValidation(t *testing.T) {
	rt, _, cookie := newInboxRouter(t)
	rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/inbox/preferences", "")
	var prefs inboxPrefsResource
	_ = json.Unmarshal(rec.Body.Bytes(), &prefs)
	if prefs.MinSeverity != alerting.SeverityWarning || prefs.DigestEnabled {
		t.Fatalf("defaults = %+v", prefs)
	}
	if rec = apiDo(t, rt, cookie, http.MethodPut, "/api/v1/inbox/preferences", `{"min_severity":"info","digest_enabled":true,"digest_hour":8}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("digest without channel = %d, want 400", rec.Code)
	}
	rec = apiDo(t, rt, cookie, http.MethodPut, "/api/v1/inbox/preferences", `{"min_severity":"critical","muted_apps":["noisy"],"digest_hour":6}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &prefs)
	if rec.Code != http.StatusOK || prefs.MinSeverity != alerting.SeverityCritical || len(prefs.MutedApps) != 1 || prefs.DigestHour != 6 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}
}
