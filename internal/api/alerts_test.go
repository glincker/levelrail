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

// newTestRouterWithAlerting is newTestRouter plus a wired AlertRules and
// NotificationChannels, the same pairing
// newTestRouterWithDeployNotifyTargets uses: creating a rule with a
// channel_id needs the channel registry to validate against.
func newTestRouterWithAlerting(t *testing.T) (*Router, *store.DB, *alerting.DB) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAlertRules(adb), WithNotificationChannels(adb))
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

// TestHandleCreateAlertRule_CertExpirySuccess checks that a
// kind=cert_expiry rule needs none of the threshold/crashloop fields:
// EvaluateCertExpiry (internal/alerting/cert_expiry.go) watches every
// stored certificate platform-wide, not this rule's own ResourceID.
func TestHandleCreateAlertRule_CertExpirySuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"cert expiry watch","kind":"cert_expiry","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "cert_expiry" {
		t.Errorf("Kind = %q, want cert_expiry", got.Kind)
	}
}

// TestHandleCreateAlertRule_PatchStatusSuccess checks that a
// kind=patch_status rule needs none of the threshold/crashloop fields
// either: EvaluatePatchStatus (internal/alerting/patch_status.go) watches
// every node platform-wide, not this rule's own ResourceID, the same
// shape kind=cert_expiry already uses.
func TestHandleCreateAlertRule_PatchStatusSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"patch status watch","kind":"patch_status","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "patch_status" {
		t.Errorf("Kind = %q, want patch_status", got.Kind)
	}
}

// TestHandleCreateAlertRule_NodeDiskSpaceSuccess checks that a
// kind=node_disk_space rule needs none of the threshold/crashloop fields
// either: EvaluateNodeDiskSpace (internal/alerting/disk_space.go) watches
// every node's disk usage platform-wide, not this rule's own
// ResourceID, the same shape kind=cert_expiry/patch_status already use.
func TestHandleCreateAlertRule_NodeDiskSpaceSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"disk space watch","kind":"node_disk_space","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "node_disk_space" {
		t.Errorf("Kind = %q, want node_disk_space", got.Kind)
	}
}

// TestHandleCreateAlertRule_NodeResourceUsageSuccess checks that a
// kind=node_resource_usage rule needs none of the threshold/crashloop
// fields either: EvaluateNodeResourceUsage
// (internal/alerting/node_resource_usage.go) watches every node's
// summed CPU/memory usage platform-wide, not this rule's own
// ResourceID, the same shape kind=node_disk_space already uses.
func TestHandleCreateAlertRule_NodeResourceUsageSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"node load watch","kind":"node_resource_usage","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "node_resource_usage" {
		t.Errorf("Kind = %q, want node_resource_usage", got.Kind)
	}
}

// TestHandleCreateAlertRule_ScheduledTaskFailureSuccess checks that a
// kind=scheduled_task_failure rule requires and accepts a
// scheduled_task_id belonging to this app, reusing
// restart_count_threshold as its consecutive-failure threshold (see
// alerting.Rule's own doc comment).
func TestHandleCreateAlertRule_ScheduledTaskFailureSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	task := store.ScheduledTask{ID: "sct_1", ServiceName: "web", Command: []string{"./cleanup.sh"}, Schedule: "0 3 * * *"}
	if err := db.SaveScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("seed scheduled task: %v", err)
	}

	body := `{"name":"cleanup failing","kind":"scheduled_task_failure","scheduled_task_id":"sct_1","restart_count_threshold":3,"notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ScheduledTaskID != "sct_1" || got.RestartCountThreshold != 3 {
		t.Errorf("got = %+v, want ScheduledTaskID=sct_1 RestartCountThreshold=3", got)
	}
}

// TestHandleCreateAlertRule_ScheduledTaskFailure_MissingTaskID checks
// toRule's own validation: a scheduled_task_failure rule needs a
// scheduled_task_id, the same way a crashloop rule needs a
// restart_count_threshold.
func TestHandleCreateAlertRule_ScheduledTaskFailure_MissingTaskID(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"cleanup failing","kind":"scheduled_task_failure","restart_count_threshold":3,"enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandleCreateAlertRule_ScheduledTaskFailure_TaskBelongsToOtherApp
// checks the ownership guard: a scheduled_task_id from a different app
// must be rejected, the same information-hiding reasoning
// handleDeleteAlertRule's own doc comment gives for its ResourceID
// check.
func TestHandleCreateAlertRule_ScheduledTaskFailure_TaskBelongsToOtherApp(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "api")
	task := store.ScheduledTask{ID: "sct_1", ServiceName: "api", Command: []string{"./cleanup.sh"}, Schedule: "0 3 * * *"}
	if err := db.SaveScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("seed scheduled task: %v", err)
	}

	body := `{"name":"cleanup failing","kind":"scheduled_task_failure","scheduled_task_id":"sct_1","restart_count_threshold":3,"enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
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

// TestHandleCreateAlertRule_ChannelIDSuccess mirrors
// TestHandleCreateDeployNotifyTarget_Success: creating a rule with a
// channel_id resolves notify_url/notify_kind from that channel in the
// response.
func TestHandleCreateAlertRule_ChannelIDSuccess(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := adb.SaveNotificationChannel(context.Background(), alerting.NotificationChannel{
		ID: "chn_1", Name: "chn_1", Kind: alerting.NotifySlack, NotifyURL: "https://example.com/hook", Enabled: true,
	}); err != nil {
		t.Fatalf("seed notification channel: %v", err)
	}

	body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"channel_id":"chn_1","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ChannelID != "chn_1" {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, "chn_1")
	}
	if got.NotifyURL != "https://example.com/hook" || got.NotifyKind != "slack" {
		t.Errorf("got = %+v, want notify_url/notify_kind resolved from the attached channel", got)
	}
}

// TestHandleCreateAlertRule_UnknownChannelID mirrors
// TestHandleCreateDeployNotifyTarget_ValidationFailures' "unknown
// channel_id" case.
func TestHandleCreateAlertRule_UnknownChannelID(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"channel_id":"chn_ghost"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
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
