package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newNoiseRouter(t *testing.T) (*Router, *store.DB, *alerting.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAlertRules(adb), WithNotificationChannels(adb), WithAlertNoise(adb))
	return rt, db, adb, loginTestSession(t, rt, db)
}

func apiDo(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestAlertNoiseRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, target := range []string{"/api/v1/alert-silences", "/api/v1/alert-maintenance-windows", "/api/v1/alert-history"} {
		if rec := apiDo(t, rt, cookie, http.MethodGet, target, ""); rec.Code != http.StatusNotImplemented {
			t.Errorf("GET %s status = %d, want 501", target, rec.Code)
		}
	}
}

func TestSilenceLifecycle(t *testing.T) {
	rt, _, _, cookie := newNoiseRouter(t)

	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/alert-silences", `{"matchers":{"apps":["web"]},"duration":"1h","reason":"deploy window"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created silenceResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Status != alerting.SilenceActive || created.CreatedBy == "" || created.Reason != "deploy window" {
		t.Fatalf("created = %+v", created)
	}

	var list []silenceResource
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/alert-silences", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("active list = %d, want 1", len(list))
	}

	if rec = apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/alert-silences/"+created.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("expire status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/alert-silences", "")
	list = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("expired silence must leave the default list, got %d", len(list))
	}
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/alert-silences?include_expired=true", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Status != alerting.SilenceExpired {
		t.Errorf("history must keep the expired silence: %+v", list)
	}
	if rec = apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/alert-silences/sil_missing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing silence status = %d, want 404", rec.Code)
	}
}

func TestSilenceValidation(t *testing.T) {
	rt, _, _, cookie := newNoiseRouter(t)
	tests := map[string]string{
		"no matchers":     `{"matchers":{},"duration":"1h"}`,
		"no duration":     `{"matchers":{"apps":["web"]}}`,
		"bad duration":    `{"matchers":{"apps":["web"]},"duration":"soon"}`,
		"too long":        `{"matchers":{"apps":["web"]},"duration":"9999h"}`,
		"bad severity":    `{"matchers":{"severities":["loud"]},"duration":"1h"}`,
		"negative window": `{"matchers":{"apps":["web"]},"duration":"-1h"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/alert-silences", body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestQuickSilenceAndRuleListFlag(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")
	ctx := context.Background()
	_ = adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu", Comparator: alerting.GreaterThan, Enabled: true})
	_ = adb.SaveRule(ctx, alerting.Rule{ID: "r2", Name: "cpu", Kind: alerting.KindThreshold, ResourceID: "service:worker", Metric: "cpu", Comparator: alerting.GreaterThan, Enabled: true})

	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/alerts/r1/silence", `{"duration":"4h","reason":"noisy"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var s silenceResource
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if len(s.Matchers.RuleIDs) != 1 || s.Matchers.RuleIDs[0] != "r1" {
		t.Errorf("quick silence must target exactly the rule, got %+v", s.Matchers)
	}
	if got := s.EndsAt.Sub(s.StartsAt); got != 4*time.Hour {
		t.Errorf("duration = %s, want 4h", got)
	}

	if rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/alerts/r2/silence", `{"duration":"1h"}`); rec.Code != http.StatusNotFound {
		t.Errorf("another app's rule status = %d, want 404", rec.Code)
	}

	var rules []ruleResource
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/alerts", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 1 || !rules[0].Silenced || rules[0].SilencedBy != s.ID {
		t.Errorf("rule list must flag the silenced rule: %+v", rules)
	}
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/worker/alerts", "")
	rules = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 1 || rules[0].Silenced {
		t.Errorf("other app's rule must not be flagged: %+v", rules)
	}
}

func TestMaintenanceWindowCRUD(t *testing.T) {
	rt, _, _, cookie := newNoiseRouter(t)
	body := `{"name":"nightly","cron":"0 3 * * *","duration":"2h","timezone":"America/New_York","scope":"app","targets":["web"],"enabled":true}`
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/alert-maintenance-windows", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var w maintenanceWindowResource
	_ = json.Unmarshal(rec.Body.Bytes(), &w)
	if w.ID == "" || w.NextStart == nil || w.Timezone != "America/New_York" {
		t.Fatalf("created = %+v", w)
	}

	upd := `{"name":"nightly","cron":"0 4 * * *","duration":"1h","timezone":"UTC","scope":"all","enabled":false}`
	if rec = apiDo(t, rt, cookie, http.MethodPut, "/api/v1/alert-maintenance-windows/"+w.ID, upd); rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list []maintenanceWindowResource
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/alert-maintenance-windows", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Cron != "0 4 * * *" || list[0].Enabled || list[0].Active {
		t.Fatalf("list = %+v", list)
	}

	bad := map[string]string{
		"bad cron":     `{"name":"x","cron":"nope","duration":"1h","scope":"all"}`,
		"bad tz":       `{"name":"x","cron":"0 3 * * *","duration":"1h","timezone":"Mars/Base","scope":"all"}`,
		"no targets":   `{"name":"x","cron":"0 3 * * *","duration":"1h","scope":"node"}`,
		"bad duration": `{"name":"x","cron":"0 3 * * *","duration":"forever","scope":"all"}`,
	}
	for name, b := range bad {
		if rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/alert-maintenance-windows", b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", name, rec.Code)
		}
	}
	if rec = apiDo(t, rt, cookie, http.MethodPut, "/api/v1/alert-maintenance-windows/mw_missing", upd); rec.Code != http.StatusNotFound {
		t.Errorf("update missing status = %d, want 404", rec.Code)
	}
	if rec = apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/alert-maintenance-windows/"+w.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", rec.Code)
	}
}

func TestAlertHistoryEndpoints(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	seedApp(t, db, "web")
	ctx := context.Background()
	now := time.Now()
	_ = adb.RecordHistory(ctx, alerting.HistoryEntry{At: now.Add(-3 * time.Minute), RuleID: "r1", RuleName: "cpu", RuleKind: "threshold", App: "web", Event: alerting.EventFired, Outcome: alerting.OutcomeSent})
	_ = adb.RecordHistory(ctx, alerting.HistoryEntry{At: now.Add(-2 * time.Minute), RuleID: "r1", RuleName: "cpu", RuleKind: "threshold", App: "web", Event: alerting.EventResolved, Outcome: alerting.OutcomeSilenced, SilenceID: "sil_1"})
	_ = adb.RecordHistory(ctx, alerting.HistoryEntry{At: now.Add(-time.Minute), RuleID: "r2", RuleName: "disk", RuleKind: "node_disk_space", Event: alerting.EventFired, Outcome: alerting.OutcomeInhibited})

	count := func(target string) int {
		rec := apiDo(t, rt, cookie, http.MethodGet, target, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", target, rec.Code, rec.Body.String())
		}
		var got []alertHistoryResource
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return len(got)
	}
	tests := map[string]int{
		"/api/v1/alert-history":                            3,
		"/api/v1/alert-history?outcome=silenced":           1,
		"/api/v1/alert-history?app=web":                    2,
		"/api/v1/alert-history?rule_id=r2":                 1,
		"/api/v1/alert-history?event=fired&limit=1":        1,
		"/api/v1/apps/web/alert-history":                   2,
		"/api/v1/alert-history?since=2099-01-01T00:00:00Z": 0,
	}
	for target, want := range tests {
		if got := count(target); got != want {
			t.Errorf("%s returned %d, want %d", target, got, want)
		}
	}
	for _, bad := range []string{"/api/v1/alert-history?limit=abc", "/api/v1/alert-history?since=yesterday"} {
		if rec := apiDo(t, rt, cookie, http.MethodGet, bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", bad, rec.Code)
		}
	}
}

func TestAlertRuleNoiseFields(t *testing.T) {
	rt, db, _, cookie := newNoiseRouter(t)
	seedApp(t, db, "web")
	body := `{"name":"cpu","kind":"threshold","metric":"cpu","comparator":">","threshold":80,"for_duration":"3m","enabled":true,` +
		`"severity":"critical","labels":{"team":"core"},"consecutive_failures":3,"flap_threshold":4,"flap_window":"20m"}`
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got ruleResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Severity != "critical" || got.Labels["team"] != "core" || got.ConsecutiveFailures != 3 || got.FlapThreshold != 4 || got.FlapWindow != "20m0s" || got.ForDuration != "3m0s" {
		t.Errorf("got = %+v", got)
	}
	for name, b := range map[string]string{
		"severity": `{"name":"c","kind":"threshold","metric":"cpu","comparator":">","severity":"loud"}`,
		"negative": `{"name":"c","kind":"threshold","metric":"cpu","comparator":">","consecutive_failures":-1}`,
		"window":   `{"name":"c","kind":"threshold","metric":"cpu","comparator":">","flap_window":"x"}`,
	} {
		if rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/alerts", b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", name, rec.Code)
		}
	}
}
