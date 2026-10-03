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

func TestHandleGetAppHealthScore_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/health-score", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleGetAppHealthScore_MixedCategories seeds one app with a
// genuinely passing deploy/observability signal and a genuinely failing
// security signal (a required secret declared but never set), then
// asserts both compute correctly and the overall status is the worst
// of the two, the same "one failure outranks everything" rule
// overallHealthStatus applies.
func TestHandleGetAppHealthScore_MixedCategories(t *testing.T) {
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	setter := &fakeSecretSetter{existsValues: map[string]bool{}} // API_KEY deliberately absent
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAlertRules(adb), WithNotificationChannels(adb), WithSecretSetter(setter))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name:      "api",
		Image:     "img:v1",
		Port:      8080,
		SecretEnv: []store.SecretEnvRef{{Name: "API_KEY", Required: true}},
	}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "att_1", ServiceName: "api", Image: "img:v1",
		Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded,
		StartedAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed deploy attempt: %v", err)
	}

	ruleID, err := alerting.NewRuleID()
	if err != nil {
		t.Fatalf("generate rule id: %v", err)
	}
	if err := adb.SaveRule(ctx, alerting.Rule{
		ID: ruleID, Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: resourceIDForApp("api"),
		Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80,
		NotifyURL: "https://example.com/hook", NotifyKind: alerting.NotifyGeneric, Enabled: true,
	}); err != nil {
		t.Fatalf("seed alert rule: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/api/health-score", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appHealthScoreResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.AppName != "api" {
		t.Errorf("app_name = %q, want %q", got.AppName, "api")
	}

	byKey := make(map[string]healthScoreCategory, len(got.Categories))
	for _, c := range got.Categories {
		byKey[c.Key] = c
	}

	if cat := byKey["security"]; cat.Status != HealthScoreStatusFail {
		t.Errorf("security status = %q, want %q (reason: %q)", cat.Status, HealthScoreStatusFail, cat.Reason)
	}
	if cat := byKey["deploy"]; cat.Status != HealthScoreStatusPass {
		t.Errorf("deploy status = %q, want %q (reason: %q)", cat.Status, HealthScoreStatusPass, cat.Reason)
	}
	if cat := byKey["observability"]; cat.Status != HealthScoreStatusPass {
		t.Errorf("observability status = %q, want %q (reason: %q)", cat.Status, HealthScoreStatusPass, cat.Reason)
	}
	// No volumes and no health check configured: a real, reportable warn,
	// not a silently skipped category.
	if cat := byKey["resilience"]; cat.Status != HealthScoreStatusWarn {
		t.Errorf("resilience status = %q, want %q (reason: %q)", cat.Status, HealthScoreStatusWarn, cat.Reason)
	}

	if got.Status != HealthScoreStatusFail {
		t.Errorf("overall status = %q, want %q (security's fail must win)", got.Status, HealthScoreStatusFail)
	}
}

// TestHandleGetAppHealthScore_CrashloopFiringFailsDeployCategory asserts
// a firing crashloop rule outranks an otherwise-clean deploy history,
// reading alerting.Rule.Firing (persisted state, not the live in-memory
// RestartTracker).
func TestHandleGetAppHealthScore_CrashloopFiringFailsDeployCategory(t *testing.T) {
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAlertRules(adb), WithNotificationChannels(adb))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "api", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "att_1", ServiceName: "api", Image: "img:v1",
		Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded,
		StartedAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed deploy attempt: %v", err)
	}

	ruleID, err := alerting.NewRuleID()
	if err != nil {
		t.Fatalf("generate rule id: %v", err)
	}
	if err := adb.SaveRule(ctx, alerting.Rule{
		ID: ruleID, Name: "crashloop", Kind: alerting.KindCrashloop, ResourceID: resourceIDForApp("api"),
		RestartCountThreshold: 3, RestartWindow: 5 * time.Minute, Enabled: true,
	}); err != nil {
		t.Fatalf("seed crashloop rule: %v", err)
	}
	now := time.Now()
	if err := adb.UpdateState(ctx, ruleID, &now, &now, true, now, nil); err != nil {
		t.Fatalf("mark rule firing: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/api/health-score", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appHealthScoreResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	for _, cat := range got.Categories {
		if cat.Key == "deploy" {
			if cat.Status != HealthScoreStatusFail {
				t.Errorf("deploy status = %q, want %q (reason: %q)", cat.Status, HealthScoreStatusFail, cat.Reason)
			}
			return
		}
	}
	t.Fatal("no deploy category in response")
}
