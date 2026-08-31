package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleDiagnoseApp_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/diagnose", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDiagnoseApp_NothingFailedYet(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Confidence != "none" {
		t.Errorf("Confidence = %q, want %q for an app with no attempts, no failing conditions", got.Confidence, "none")
	}
	if len(got.MatchedSignals) != 0 {
		t.Errorf("MatchedSignals = %+v, want none", got.MatchedSignals)
	}
}

func TestHandleDiagnoseApp_LatestFailedAttempt(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "web", Image: "levelrail/web:2",
		Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusRunning, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_1", store.DeployAttemptStatusFailed, base.Add(time.Minute),
		"deploy: service \"web\": create: Bind for 0.0.0.0:3000 failed: port is already allocated"); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q", got.Confidence, "high")
	}
	if got.DeployAttemptID != "dep_1" {
		t.Errorf("DeployAttemptID = %q, want %q", got.DeployAttemptID, "dep_1")
	}
	if len(got.MatchedSignals) != 1 || got.MatchedSignals[0].Source != "deploy_attempt.error" {
		t.Fatalf("MatchedSignals = %+v, want the attempt error surfaced", got.MatchedSignals)
	}
}

func TestHandleDiagnoseApp_PinnedDeployID(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_old", ServiceName: "web", Image: "levelrail/web:1",
		Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed old attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_old", store.DeployAttemptStatusFailed, base.Add(time.Minute), "no such image: levelrail/web:1"); err != nil {
		t.Fatalf("finish old attempt: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_new", ServiceName: "web", Image: "levelrail/web:2",
		Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed new attempt: %v", err)
	}

	// Pinning to the older, failed attempt must diagnose that one, not
	// the newer succeeded one that would otherwise be picked as latest.
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose?deploy_id=dep_old", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DeployAttemptID != "dep_old" {
		t.Errorf("DeployAttemptID = %q, want %q", got.DeployAttemptID, "dep_old")
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q (image pull failure signature)", got.Confidence, "high")
	}
}

func TestHandleDiagnoseApp_PinnedDeployID_WrongApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "other", Image: "levelrail/other:1", Port: 3001}); err != nil {
		t.Fatalf("seed app other: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "other", Image: "levelrail/other:1",
		Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose?deploy_id=dep_1", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDiagnoseApp_FailingCondition(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpsertConditions(ctx, applicationControllerName("web"), []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "ReadinessFailed", Message: "readiness probe: timed out waiting for a successful response"},
	}); err != nil {
		t.Fatalf("seed conditions: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q", got.Confidence, "high")
	}
	if got.DeployAttemptID != "" {
		t.Errorf("DeployAttemptID = %q, want empty: no attempt exists", got.DeployAttemptID)
	}
}

func TestHandleDiagnoseApp_CrashloopFiring(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedApp(t, db, "web")

	if err := adb.SaveRule(ctx, alerting.Rule{
		ID: "rule_1", Name: "crashloop", Kind: alerting.KindCrashloop,
		ResourceID: resourceIDForApp("web"), RestartCountThreshold: 3, RestartWindow: 5 * time.Minute, Enabled: true,
	}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	restartCount := 4.0
	now := time.Now()
	if err := adb.UpdateState(ctx, "rule_1", nil, &now, true, now, &restartCount); err != nil {
		t.Fatalf("seed firing state: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q", got.Confidence, "high")
	}
	if len(got.MatchedSignals) != 1 || got.MatchedSignals[0].Source != "crashloop_rule" {
		t.Fatalf("MatchedSignals = %+v, want the crashloop rule surfaced", got.MatchedSignals)
	}
}

// TestHandleDiagnoseApp_BuildPhaseFailure_UsesPersistedBuildLog covers the
// gap where a build-phase failure (no container ever started) used to be
// diagnosed against an empty runtime-log query; it should instead pull
// the real build output from the persisted deploy log store, giving
// internal/diagnose's build_dependency_failure signature real text to
// match against.
func TestHandleDiagnoseApp_BuildPhaseFailure_UsesPersistedBuildLog(t *testing.T) {
	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithDeployLogQuerier(tdb))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_build_fail", ServiceName: "web", Image: "levelrail/web:2",
		Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusRunning, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_build_fail", store.DeployAttemptStatusFailed, base.Add(time.Minute),
		`deploy: service "web": build: exit code 1`); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	if err := tdb.WriteDeployLogBatch(ctx, []telemetry.DeployLogEntry{
		{AttemptID: "dep_build_fail", Stream: "stdout", Timestamp: base, Message: "Step 3/5: RUN npm ci"},
		{AttemptID: "dep_build_fail", Stream: "stderr", Timestamp: base.Add(time.Second), Message: "npm ERR! code ERESOLVE"},
	}); err != nil {
		t.Fatalf("seed build log: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Confidence != "medium" {
		t.Errorf("Confidence = %q, want %q (build_dependency_failure signature)", got.Confidence, "medium")
	}
	if len(got.MatchedSignals) != 1 || got.MatchedSignals[0].Source != "logs" {
		t.Fatalf("MatchedSignals = %+v, want one signal sourced from the persisted build log", got.MatchedSignals)
	}
	if !strings.Contains(strings.ToLower(got.MatchedSignals[0].Excerpt), "npm err!") {
		t.Errorf("Excerpt = %q, want it to contain the actual npm error line from the build log", got.MatchedSignals[0].Excerpt)
	}
}

// TestHandleDiagnoseApp_RuntimeFailure_IgnoresBuildLog confirms a
// non-build failure (a container started and later failed) still uses
// the runtime telemetry log path, not the persisted build log, even when
// a build log happens to exist for some other attempt.
func TestHandleDiagnoseApp_RuntimeFailure_IgnoresBuildLog(t *testing.T) {
	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithDeployLogQuerier(tdb))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_runtime_fail", ServiceName: "web", Image: "levelrail/web:2",
		Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusRunning, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_runtime_fail", store.DeployAttemptStatusFailed, base.Add(time.Minute),
		"port is already allocated"); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	if err := tdb.WriteDeployLogBatch(ctx, []telemetry.DeployLogEntry{
		{AttemptID: "dep_runtime_fail", Stream: "stdout", Timestamp: base, Message: "npm ERR! code ERESOLVE"},
	}); err != nil {
		t.Fatalf("seed build log: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q (port_conflict signature from the attempt error, not the build log)", got.Confidence, "high")
	}
	if len(got.MatchedSignals) != 1 || got.MatchedSignals[0].Source != "deploy_attempt.error" {
		t.Fatalf("MatchedSignals = %+v, want the attempt error surfaced, not the unrelated build log", got.MatchedSignals)
	}
}

func TestHandleDiagnoseApp_RequiresAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/diagnose", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
