package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestAlertingDB opens a fresh temp-file alerting store, the same
// real-not-mocked pattern newTestTelemetryDB (metrics_test.go) already
// establishes for internal/telemetry in this package's tests.
func newTestAlertingDB(t *testing.T) *alerting.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alerting.db")
	db, err := alerting.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("alerting.Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test alerting db: %v", err)
		}
	})
	return db
}

// newTestRouterWithAlerting is newTestRouter plus a wired AlertRules,
// for the alert-rule handlers specifically.
func newTestRouterWithAlerting(t *testing.T) (*Router, *store.DB, *alerting.DB) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAlertRules(adb))
	return rt, db, adb
}

func seedApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "img:v1", Port: 3000}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

func TestAlertRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithAlertRules
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	tests := []struct {
		method, target, body string
	}{
		{http.MethodPost, "/api/v1/apps/web/alerts", `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`},
		{http.MethodGet, "/api/v1/apps/web/alerts", ""},
		{http.MethodDelete, "/api/v1/apps/web/alerts/whatever", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, tt.target, tt.body))
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateAlertRule_ThresholdSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"for_duration":"2m","notify_url":"https://example.com/hook","notify_kind":"slack","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a server-generated ID")
	}
	if got.ResourceID != "service:web" {
		t.Errorf("ResourceID = %q, want %q", got.ResourceID, "service:web")
	}
	if got.Name != "high cpu" || got.Kind != "threshold" || got.Metric != "cpu_percent" || got.Comparator != ">" || got.Threshold != 80 {
		t.Errorf("got = %+v, want matching fields from the request body", got)
	}
	if got.ForDuration != "2m0s" {
		t.Errorf("ForDuration = %q, want %q", got.ForDuration, "2m0s")
	}
}

func TestHandleCreateAlertRule_CrashloopSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"web crashlooping","kind":"crashloop","restart_count_threshold":3,"restart_window":"5m","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RestartCountThreshold != 3 || got.RestartWindow != "5m0s" {
		t.Errorf("got = %+v, want RestartCountThreshold=3 RestartWindow=5m0s", got)
	}
}

// TestHandleCreateAlertRule_ResourceIDNotCallerSuppliable checks that a
// caller-supplied resource_id in the request body is discarded in favor
// of resourceIDForApp(name): a rule created through /apps/web/alerts is
// always scoped to service:web, regardless of what the body claims.
func TestHandleCreateAlertRule_ResourceIDNotCallerSuppliable(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"resource_id":"service:someone-elses-app"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ResourceID != "service:web" {
		t.Errorf("ResourceID = %q, want %q (caller-supplied resource_id must be ignored)", got.ResourceID, "service:web")
	}
}

// TestHandleCreateAlertRule_IDNotCallerSuppliable checks that a
// caller-supplied id is ignored: NewRuleID always mints a fresh one.
func TestHandleCreateAlertRule_IDNotCallerSuppliable(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"id":"attacker-chosen-id","name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "attacker-chosen-id" {
		t.Error("ID = attacker-chosen-id, want a server-generated ID, caller-supplied id must be ignored")
	}
}

func TestHandleCreateAlertRule_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/alerts", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCreateAlertRule_ValidationFailures(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	tests := []struct {
		name string
		body string
	}{
		{"missing name", `{"kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`},
		{"bad kind", `{"name":"x","kind":"bogus","metric":"cpu_percent","comparator":">","threshold":80}`},
		{"bad comparator", `{"name":"x","kind":"threshold","metric":"cpu_percent","comparator":"~=","threshold":80}`},
		{"threshold missing metric", `{"name":"x","kind":"threshold","comparator":">","threshold":80}`},
		{"crashloop missing restart_count_threshold", `{"name":"x","kind":"crashloop","restart_window":"5m"}`},
		{"crashloop missing restart_window", `{"name":"x","kind":"crashloop","restart_count_threshold":3}`},
		{"malformed body", `{not json`},
		{"bad for_duration", `{"name":"x","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"for_duration":"not-a-duration"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleListAlertRules(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "web high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r2", Name: "web crashlooping", Kind: alerting.KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 3, RestartWindow: 0, Enabled: false}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r3", Name: "worker high cpu", Kind: alerting.KindThreshold, ResourceID: "service:worker", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/alerts", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rules, want 2 (only service:web's rules, including the disabled one)", len(got))
	}
	for _, r := range got {
		if r.ResourceID != "service:web" {
			t.Errorf("got rule %+v scoped to a different resource", r)
		}
	}
}

func TestHandleListAlertRules_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/alerts", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteAlertRule_Success(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/alerts/r1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, err := adb.GetRule(ctx, "r1"); err == nil {
		t.Error("rule still exists after delete")
	}
}

func TestHandleDeleteAlertRule_NotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/alerts/nonexistent", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleDeleteAlertRule_OwnershipMismatch is the key security case:
// a rule belonging to service:worker must not be deletable by guessing
// its ID through service:web's URL, even though both apps exist and the
// caller is authenticated with write access.
func TestHandleDeleteAlertRule_OwnershipMismatch(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "worker high cpu", Kind: alerting.KindThreshold, ResourceID: "service:worker", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/alerts/r1", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: deleting another app's rule through this app's URL must fail as if the rule didn't exist", rec.Code, http.StatusNotFound)
	}

	if _, err := adb.GetRule(ctx, "r1"); err != nil {
		t.Errorf("rule was deleted despite the ownership mismatch: GetRule() error = %v", err)
	}
}

func TestAlertRoutes_RequireAuth(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	seedApp(t, db, "web")

	tests := []struct {
		method, target string
	}{
		{http.MethodPost, "/api/v1/apps/web/alerts"},
		{http.MethodGet, "/api/v1/apps/web/alerts"},
		{http.MethodDelete, "/api/v1/apps/web/alerts/r1"},
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
