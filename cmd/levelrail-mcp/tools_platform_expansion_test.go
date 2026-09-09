package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListIAMPolicies(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/iam/policies" {
			t.Errorf("path = %q, want /api/v1/iam/policies", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.PolicyResource{{ID: "pol_1", Name: "deny-prod"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_iam_policies", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_iam_policies) error = %v", err)
	}
	var policies []apiclient.PolicyResource
	decodeStructured(t, result, &policies)
	if len(policies) != 1 || policies[0].ID != "pol_1" {
		t.Errorf("policies = %+v, want one policy with id pol_1", policies)
	}
}

func TestGetIAMPolicy(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/iam/policies/pol_1" {
			t.Errorf("path = %q, want /api/v1/iam/policies/pol_1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.PolicyResource{ID: "pol_1", Name: "deny-prod"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_iam_policy", Arguments: map[string]any{"id": "pol_1"}})
	if err != nil {
		t.Fatalf("CallTool(get_iam_policy) error = %v", err)
	}
	var policy apiclient.PolicyResource
	decodeStructured(t, result, &policy)
	if policy.Name != "deny-prod" {
		t.Errorf("policy.Name = %q, want %q", policy.Name, "deny-prod")
	}
}

func TestGetIAMPolicy_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"policy not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_iam_policy", Arguments: map[string]any{"id": "ghost"}})
	if err != nil {
		t.Fatalf("CallTool(get_iam_policy) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "policy not found") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

func TestListNotificationChannels(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/notification-channels" {
			t.Errorf("path = %q, want /api/v1/notification-channels", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.NotificationChannelResource{{ID: "c1", Name: "ops-slack", Kind: "slack", Enabled: true}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_notification_channels", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_notification_channels) error = %v", err)
	}
	var channels []apiclient.NotificationChannelResource
	decodeStructured(t, result, &channels)
	if len(channels) != 1 || channels[0].ID != "c1" {
		t.Errorf("channels = %+v, want one channel with id c1", channels)
	}
}

func TestGetAppGitSource(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/git-source" {
			t.Errorf("path = %q, want /api/v1/apps/web/git-source", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.GitSourceResource{ServiceName: "web", RepoURL: "https://github.com/org/web", Branch: "main", HasToken: true})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_git_source", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_git_source) error = %v", err)
	}
	var source apiclient.GitSourceResource
	decodeStructured(t, result, &source)
	if source.RepoURL != "https://github.com/org/web" || !source.HasToken {
		t.Errorf("source = %+v, want repo_url set and has_token true", source)
	}
}

func TestGetAppGitSource_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no git source connected for this app"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_git_source", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_git_source) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "no git source connected") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

func TestGetAppHookRuns(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/hook-runs" {
			t.Errorf("path = %q, want /api/v1/apps/web/hook-runs", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppHookRunsResource{
			PreDeploy: &apiclient.HookRunResource{HookType: "pre_deploy", Command: "migrate", ExitCode: 0, Success: true},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_hook_runs", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_hook_runs) error = %v", err)
	}
	var runs apiclient.AppHookRunsResource
	decodeStructured(t, result, &runs)
	if runs.PreDeploy == nil || !runs.PreDeploy.Success {
		t.Errorf("runs = %+v, want a successful pre_deploy run", runs)
	}
	if runs.PostDeploy != nil {
		t.Errorf("runs.PostDeploy = %+v, want nil (never run)", runs.PostDeploy)
	}
}

func TestGetAppHookRuns_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_hook_runs", Arguments: map[string]any{"name": "ghost"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_hook_runs) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "app not found") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

func TestGetDomainTLSCertStatus(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/domains/app.example.com/tls-cert" {
			t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/tls-cert", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DomainTLSCertResource{Domain: "app.example.com", Enabled: true, ExpiresAt: "2027-01-01T00:00:00Z"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_domain_tls_cert_status",
		Arguments: map[string]any{"name": "web", "domain": "app.example.com"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_domain_tls_cert_status) error = %v", err)
	}
	var status apiclient.DomainTLSCertResource
	decodeStructured(t, result, &status)
	if !status.Enabled || status.Domain != "app.example.com" {
		t.Errorf("status = %+v, want enabled=true for app.example.com", status)
	}
}

func TestListOrganizations(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/organizations" {
			t.Errorf("path = %q, want /api/v1/organizations", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.OrganizationResource{{ID: "org_1", Name: "acme"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_organizations", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_organizations) error = %v", err)
	}
	var orgs []apiclient.OrganizationResource
	decodeStructured(t, result, &orgs)
	if len(orgs) != 1 || orgs[0].Name != "acme" {
		t.Errorf("orgs = %+v, want one organization named acme", orgs)
	}
}

func TestListProjects(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			t.Errorf("path = %q, want /api/v1/projects", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ProjectResource{{ID: "proj_1", Name: "core"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_projects) error = %v", err)
	}
	var projects []apiclient.ProjectResource
	decodeStructured(t, result, &projects)
	if len(projects) != 1 || projects[0].Name != "core" {
		t.Errorf("projects = %+v, want one project named core", projects)
	}
}

func TestListRegistryCredentials(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/registry-credentials" {
			t.Errorf("path = %q, want /api/v1/registry-credentials", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.RegistryCredentialResource{{ID: "rc1", Name: "ghcr", RegistryHost: "ghcr.io", Username: "deploy", ExpiryStatus: "healthy"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_registry_credentials", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_registry_credentials) error = %v", err)
	}
	var creds []apiclient.RegistryCredentialResource
	decodeStructured(t, result, &creds)
	if len(creds) != 1 || creds[0].ExpiryStatus != "healthy" {
		t.Errorf("creds = %+v, want one credential with expiry_status healthy", creds)
	}
}

func TestListBackupTargets(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/backup-targets" {
			t.Errorf("path = %q, want /api/v1/backup-targets", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.BackupTargetResource{{ID: "bt1", Name: "primary-s3", Provider: "s3", Bucket: "backups"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_backup_targets", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_backup_targets) error = %v", err)
	}
	var targets []apiclient.BackupTargetResource
	decodeStructured(t, result, &targets)
	if len(targets) != 1 || targets[0].Bucket != "backups" {
		t.Errorf("targets = %+v, want one target with bucket backups", targets)
	}
}

func TestTestBackupTargetConnection(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/backup-targets/bt1/test" {
			t.Errorf("request = %s %s, want POST /api/v1/backup-targets/bt1/test", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "test_backup_target_connection",
		Arguments: map[string]any{"id": "bt1"},
	})
	if err != nil {
		t.Fatalf("CallTool(test_backup_target_connection) error = %v", err)
	}
	var out backupTargetTestResult
	decodeStructured(t, result, &out)
	if !out.OK {
		t.Errorf("out.OK = false, want true on a successful connection test")
	}
}

func TestTestBackupTargetConnection_Failure(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"credential rejected by target"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "test_backup_target_connection",
		Arguments: map[string]any{"id": "bt1"},
	})
	if err != nil {
		t.Fatalf("CallTool(test_backup_target_connection) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a failed connection test")
	}
	if !strings.Contains(toolResultText(result), "credential rejected by target") {
		t.Errorf("error text = %q, want it to contain the server's own error message", toolResultText(result))
	}
}

func TestListAppVolumeBackups(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/volumes/data/backups" {
			t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/backups", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.BackupHistoryResource{{ID: "vb1", ServiceName: "web", VolumeName: "data", Status: "succeeded"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_app_volume_backups",
		Arguments: map[string]any{"name": "web", "volume": "data", "limit": 5},
	})
	if err != nil {
		t.Fatalf("CallTool(list_app_volume_backups) error = %v", err)
	}
	var history []apiclient.BackupHistoryResource
	decodeStructured(t, result, &history)
	if len(history) != 1 || history[0].ID != "vb1" {
		t.Errorf("history = %+v, want one backup with id vb1", history)
	}
	if gotQuery != "5" {
		t.Errorf("limit query param = %q, want %q", gotQuery, "5")
	}
}

// TestPlatformExpansionTools_Surface403 mirrors TestNewTools_Surface403
// (tools_test.go) for every tool added in this batch: each hits its own
// distinct API path and could regress independently of the others.
func TestPlatformExpansionTools_Surface403(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"list_iam_policies", map[string]any{}},
		{"get_iam_policy", map[string]any{"id": "pol_1"}},
		{"list_notification_channels", map[string]any{}},
		{"get_app_git_source", map[string]any{"name": "web"}},
		{"get_app_hook_runs", map[string]any{"name": "web"}},
		{"get_domain_tls_cert_status", map[string]any{"name": "web", "domain": "app.example.com"}},
		{"list_organizations", map[string]any{}},
		{"list_projects", map[string]any{}},
		{"list_registry_credentials", map[string]any{}},
		{"list_backup_targets", map[string]any{}},
		{"test_backup_target_connection", map[string]any{"id": "bt1"}},
		{"list_app_volume_backups", map[string]any{"name": "web", "volume": "data"}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"token lacks the required ability"}`))
			})

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error = %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("IsError = false, want true for a 403 response")
			}
			text := toolResultText(result)
			if !strings.Contains(text, "token lacks the required ability") {
				t.Errorf("error text = %q, want it to contain the server's own 403 message", text)
			}
			if !strings.Contains(text, "403") {
				t.Errorf("error text = %q, want it to contain the status code 403", text)
			}
		})
	}
}
