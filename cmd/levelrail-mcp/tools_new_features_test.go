package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListFeatureFlags(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/flags" {
			t.Errorf("path = %q, want /api/v1/apps/web/flags", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.FeatureFlagResource{{ID: "f1", Key: "new-ui", Name: "New UI", Enabled: true, RolloutPercentage: 50}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_feature_flags", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(list_feature_flags) error = %v", err)
	}
	var flags []apiclient.FeatureFlagResource
	decodeStructured(t, result, &flags)
	if len(flags) != 1 || flags[0].Key != "new-ui" {
		t.Errorf("flags = %+v, want one flag with key new-ui", flags)
	}
}

func TestGetFeatureFlag(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/flags/f1" {
			t.Errorf("path = %q, want /api/v1/apps/web/flags/f1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.FeatureFlagResource{ID: "f1", Key: "new-ui", Name: "New UI", Enabled: true, RolloutPercentage: 50})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_feature_flag", Arguments: map[string]any{"name": "web", "id": "f1"}})
	if err != nil {
		t.Fatalf("CallTool(get_feature_flag) error = %v", err)
	}
	var flag apiclient.FeatureFlagResource
	decodeStructured(t, result, &flag)
	if flag.ID != "f1" {
		t.Errorf("flag.ID = %q, want %q", flag.ID, "f1")
	}
}

func TestGetSystemDoctor(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/doctor" {
			t.Errorf("path = %q, want /api/v1/system/doctor", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.SystemDoctorResource{
			OK:     false,
			Checks: []apiclient.DoctorCheckResource{{Code: "docker", Name: "Docker daemon", Status: "fail", Message: "not reachable"}},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_system_doctor", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_system_doctor) error = %v", err)
	}
	var report apiclient.SystemDoctorResource
	decodeStructured(t, result, &report)
	if report.OK || len(report.Checks) != 1 || report.Checks[0].Status != "fail" {
		t.Errorf("report = %+v, want OK=false with one failing check", report)
	}
}

func TestGetOnboardingStatus(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/onboarding" {
			t.Errorf("path = %q, want /api/v1/onboarding", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.OnboardingStateResource{Completed: true})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_onboarding_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_onboarding_status) error = %v", err)
	}
	var state apiclient.OnboardingStateResource
	decodeStructured(t, result, &state)
	if !state.Completed {
		t.Errorf("state.Completed = false, want true")
	}
}

func TestListWebhookDeliveries(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/webhook-deliveries" {
			t.Errorf("path = %q, want /api/v1/apps/web/webhook-deliveries", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.WebhookDeliveryResource{{ID: "d1", Provider: "github", EventType: "push", Matched: true, StatusCode: 200}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_webhook_deliveries",
		Arguments: map[string]any{"name": "web", "limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool(list_webhook_deliveries) error = %v", err)
	}
	var deliveries []apiclient.WebhookDeliveryResource
	decodeStructured(t, result, &deliveries)
	if len(deliveries) != 1 || deliveries[0].ID != "d1" {
		t.Errorf("deliveries = %+v, want one delivery with id d1", deliveries)
	}
	if gotQuery != "10" {
		t.Errorf("limit query param = %q, want %q", gotQuery, "10")
	}
}

func TestListBackupVerifications(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/databases/main/backups/bkp1/verifications" {
			t.Errorf("path = %q, want /api/v1/databases/main/backups/bkp1/verifications", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.BackupVerificationResource{
			{ID: "v2", BackupHistoryID: "bkp1", Status: "passed", ChecksumMatch: true, SizeMatch: true, FormatValid: true},
			{ID: "v1", BackupHistoryID: "bkp1", Status: "failed", ChecksumMatch: false},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_backup_verifications",
		Arguments: map[string]any{"name": "main", "backup_id": "bkp1"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_backup_verifications) error = %v", err)
	}
	var verifications []apiclient.BackupVerificationResource
	decodeStructured(t, result, &verifications)
	if len(verifications) != 2 || verifications[0].ID != "v2" {
		t.Errorf("verifications = %+v, want two entries with v2 first", verifications)
	}
}

func TestGetLatestBackupVerification(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/databases/main/backups/bkp1/verifications" {
			t.Errorf("path = %q, want /api/v1/databases/main/backups/bkp1/verifications", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.BackupVerificationResource{
			{ID: "v2", BackupHistoryID: "bkp1", Status: "passed", ChecksumMatch: true},
			{ID: "v1", BackupHistoryID: "bkp1", Status: "failed"},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_latest_backup_verification",
		Arguments: map[string]any{"name": "main", "backup_id": "bkp1"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_latest_backup_verification) error = %v", err)
	}
	var verification apiclient.BackupVerificationResource
	decodeStructured(t, result, &verification)
	if verification.ID != "v2" {
		t.Errorf("verification.ID = %q, want %q (the newest entry)", verification.ID, "v2")
	}
}

func TestGetLatestBackupVerification_None(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.BackupVerificationResource{})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_latest_backup_verification",
		Arguments: map[string]any{"name": "main", "backup_id": "bkp1"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_latest_backup_verification) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("IsError = true, want false when no verification has ever run")
	}
	var verification apiclient.BackupVerificationResource
	decodeStructured(t, result, &verification)
	if verification.ID != "" {
		t.Errorf("verification.ID = %q, want empty when no verification exists", verification.ID)
	}
}

func TestCompareDeploys(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/deploys/compare" {
			t.Errorf("path = %q, want /api/v1/apps/web/deploys/compare", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("from")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DeployCompareResource{
			ServiceName: "web",
			From:        apiclient.DeployCompareSide{DeployID: "dep_1", Image: "nginx:1"},
			To:          apiclient.DeployCompareSide{IsCurrent: true, Image: "nginx:2"},
			Changes:     []apiclient.DeployCompareField{{Field: "image", From: "nginx:1", To: "nginx:2"}},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "compare_deploys",
		Arguments: map[string]any{"name": "web", "from": "dep_1"},
	})
	if err != nil {
		t.Fatalf("CallTool(compare_deploys) error = %v", err)
	}
	var cmp apiclient.DeployCompareResource
	decodeStructured(t, result, &cmp)
	if len(cmp.Changes) != 1 || cmp.Changes[0].Field != "image" {
		t.Errorf("cmp = %+v, want one image change", cmp)
	}
	if gotQuery != "dep_1" {
		t.Errorf("from query param = %q, want %q", gotQuery, "dep_1")
	}
}

func TestListNotificationDeliveries(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/notification-channels/c1/deliveries" {
			t.Errorf("path = %q, want /api/v1/notification-channels/c1/deliveries", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.NotificationDeliveryResource{{ID: "n1", ChannelID: "c1", Trigger: "deploy_failed", Success: false, Error: "timeout"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_notification_deliveries",
		Arguments: map[string]any{"id": "c1"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_notification_deliveries) error = %v", err)
	}
	var deliveries []apiclient.NotificationDeliveryResource
	decodeStructured(t, result, &deliveries)
	if len(deliveries) != 1 || deliveries[0].ID != "n1" {
		t.Errorf("deliveries = %+v, want one delivery with id n1", deliveries)
	}
}
