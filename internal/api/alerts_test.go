package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// newTestRouterWithAlerting wires AlertRules and NotificationChannels, since channel_id validation needs the registry.
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
	t.Parallel()
	rt, db := newTestRouter(t) // no WithAlertRules
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	tests := []struct {
		method, target, body string
	}{
		{http.MethodPost, "/api/v1/apps/web/alerts", `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`},
		{http.MethodGet, "/api/v1/apps/web/alerts", ""},
		{http.MethodPut, "/api/v1/apps/web/alerts/whatever", `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`},
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
	t.Parallel()
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

func TestHandleCreateAlertRule_WarnsWithoutDestination(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		extra    string
		wantWarn bool
	}{
		{name: "no channel and no url warns", extra: "", wantWarn: true},
		{name: "notify url does not warn", extra: `,"notify_url":"https://hooks.glincker.com/x","notify_kind":"slack"`, wantWarn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, _ := newTestRouterWithAlerting(t)
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"enabled":true` + tt.extra + `}`
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var got ruleResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if gotWarn := len(got.Warnings) > 0; gotWarn != tt.wantWarn {
				t.Errorf("Warnings = %v, want warning = %v", got.Warnings, tt.wantWarn)
			}
		})
	}
}

func TestHandleCreateAlertRule_CrashloopSuccess(t *testing.T) {
	t.Parallel()
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

// Platform-wide kinds like cert_expiry need no threshold or crashloop fields.
func TestHandleCreateAlertRule_CertExpirySuccess(t *testing.T) {
	t.Parallel()
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

// Platform-wide kinds like patch_status need no threshold or crashloop fields.
func TestHandleCreateAlertRule_PatchStatusSuccess(t *testing.T) {
	t.Parallel()
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

// Platform-wide kinds like node_disk_space need no threshold or crashloop fields.
func TestHandleCreateAlertRule_NodeDiskSpaceSuccess(t *testing.T) {
	t.Parallel()
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

// Platform-wide kinds like node_resource_usage need no threshold or crashloop fields.
func TestHandleCreateAlertRule_NodeResourceUsageSuccess(t *testing.T) {
	t.Parallel()
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

func TestHandleCreateAlertRule_NodeOfflineSuccess(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"node offline","kind":"node_offline","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

// scheduled_task_failure reuses restart_count_threshold as its consecutive-failure threshold.
func TestHandleCreateAlertRule_ScheduledTaskFailureSuccess(t *testing.T) {
	t.Parallel()
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

func TestHandleCreateAlertRule_ScheduledTaskFailure_MissingTaskID(t *testing.T) {
	t.Parallel()
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

// A scheduled_task_id from another app must be rejected, so ownership cannot leak.
func TestHandleCreateAlertRule_ScheduledTaskFailure_TaskBelongsToOtherApp(t *testing.T) {
	t.Parallel()
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

// backup_missing reuses for_duration as its overdue grace period.
func TestHandleCreateAlertRule_BackupMissingDatabaseSuccess(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	body := `{"name":"main backup missing","kind":"backup_missing","backup_resource_kind":"database","backup_database_name":"main","for_duration":"6h","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BackupResourceKind != "database" || got.BackupDatabaseName != "main" || got.ForDuration != "6h0m0s" {
		t.Errorf("got = %+v, want BackupResourceKind=database BackupDatabaseName=main ForDuration=6h0m0s", got)
	}
}

// TestHandleCreateAlertRule_BackupMissingVolumeSuccess checks the service
// volume variant: backup_service_name/backup_volume_name must resolve to
// a real, already-configured service_volume_backups row.
func TestHandleCreateAlertRule_BackupMissingVolumeSuccess(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := db.SaveBackupTarget(context.Background(), store.BackupTarget{ID: "tgt_1", Name: "s3", Provider: "aws", Bucket: "backups"}); err != nil {
		t.Fatalf("seed backup target: %v", err)
	}
	if err := db.SetServiceVolumeBackupSchedule(context.Background(), "web", "uploads", "tgt_1", "0 3 * * *", 7, 0, ""); err != nil {
		t.Fatalf("seed service volume backup schedule: %v", err)
	}

	body := `{"name":"uploads backup missing","kind":"backup_missing","backup_resource_kind":"volume","backup_service_name":"web","backup_volume_name":"uploads","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BackupResourceKind != "volume" || got.BackupServiceName != "web" || got.BackupVolumeName != "uploads" {
		t.Errorf("got = %+v, want BackupResourceKind=volume BackupServiceName=web BackupVolumeName=uploads", got)
	}
}

// TestHandleCreateAlertRule_BackupMissing_UnknownDatabase checks
// validateAlertRuleReferences' own existence check: a backup_database_name
// naming a database that doesn't exist must be rejected.
func TestHandleCreateAlertRule_BackupMissing_UnknownDatabase(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"missing db","kind":"backup_missing","backup_resource_kind":"database","backup_database_name":"gone","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// An unconfigured volume backup target must be rejected, as the database branch does.
func TestHandleCreateAlertRule_BackupMissing_UnconfiguredVolume(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"missing volume","kind":"backup_missing","backup_resource_kind":"volume","backup_service_name":"web","backup_volume_name":"never-configured","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandleCreateAlertRule_BackupMissing_MissingResourceKind checks
// toRule's own validation: an empty/unknown backup_resource_kind is
// rejected rather than silently defaulting to database.
func TestHandleCreateAlertRule_BackupMissing_MissingResourceKind(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"no kind","kind":"backup_missing","backup_database_name":"main","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// A caller-supplied resource_id is ignored in favor of resourceIDForApp(name).
func TestHandleCreateAlertRule_ResourceIDNotCallerSuppliable(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// channel_id resolves notify_url and notify_kind from the saved channel.
func TestHandleCreateAlertRule_ChannelIDSuccess(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/alerts", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateAlertRule_Success(t *testing.T) {
	t.Parallel()
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	body := `{"name":"even higher cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":95,"enabled":false}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/alerts/r1", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "r1" || got.Name != "even higher cpu" || got.Threshold != 95 || got.Enabled {
		t.Errorf("got = %+v, want the updated fields with the same id", got)
	}
	if got.ResourceID != "service:web" {
		t.Errorf("ResourceID = %q, want %q", got.ResourceID, "service:web")
	}
}

// Editing a rule's config must not reset its firing state; only the evaluator changes that.
func TestHandleUpdateAlertRule_PreservesEvaluationState(t *testing.T) {
	t.Parallel()
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	firingSince := time.Now().UTC()
	if err := adb.UpdateState(ctx, "r1", nil, &firingSince, true, firingSince, nil); err != nil {
		t.Fatalf("seed evaluation state: %v", err)
	}

	body := `{"name":"high cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":90,"enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/alerts/r1", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: updating a rule's config must not reset its evaluation state")
	}
}

func TestHandleUpdateAlertRule_NotFound(t *testing.T) {
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"x","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/alerts/nonexistent", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// A rule of service:worker cannot be edited through service:web's URL.
func TestHandleUpdateAlertRule_OwnershipMismatch(t *testing.T) {
	t.Parallel()
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "worker high cpu", Kind: alerting.KindThreshold, ResourceID: "service:worker", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	body := `{"name":"hijacked","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":1}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/alerts/r1", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: updating another app's rule through this app's URL must fail as if the rule didn't exist", rec.Code, http.StatusNotFound)
	}

	unchanged, err := adb.GetRule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if unchanged.Name != "worker high cpu" {
		t.Errorf("rule was modified despite the ownership mismatch: Name = %q", unchanged.Name)
	}
}

func TestHandleUpdateAlertRule_ValidationFailures(t *testing.T) {
	t.Parallel()
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	tests := []struct {
		name string
		body string
	}{
		{"missing name", `{"kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`},
		{"bad kind", `{"name":"x","kind":"bogus","metric":"cpu_percent","comparator":">","threshold":80}`},
		{"threshold missing metric", `{"name":"x","kind":"threshold","comparator":">","threshold":80}`},
		{"malformed body", `{not json`},
		{"unknown channel_id", `{"name":"x","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"channel_id":"chn_ghost"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/alerts/r1", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

// Read-only tokens are rejected, since the route is gated by AbilityWrite.
func TestHandleUpdateAlertRule_RequiresWriteAbility(t *testing.T) {
	t.Parallel()
	rt, db, adb := newTestRouterWithAlerting(t)
	seedApp(t, db, "web")
	ctx := context.Background()
	if err := adb.SaveRule(ctx, alerting.Rule{ID: "r1", Name: "high cpu", Kind: alerting.KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 80, Enabled: true}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	const plaintext = "read-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_ro", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"name":"x","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/alerts/r1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestHandleDeleteAlertRule_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/alerts/nonexistent", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// A rule of service:worker cannot be deleted through service:web's URL, even with write access.
func TestHandleDeleteAlertRule_OwnershipMismatch(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	rt, db, _ := newTestRouterWithAlerting(t)
	seedApp(t, db, "web")

	tests := []struct {
		method, target string
	}{
		{http.MethodPost, "/api/v1/apps/web/alerts"},
		{http.MethodGet, "/api/v1/apps/web/alerts"},
		{http.MethodPut, "/api/v1/apps/web/alerts/r1"},
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
