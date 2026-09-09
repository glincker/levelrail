package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListDeployAttempts(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/deploy-attempts" {
			t.Errorf("path = %q, want /api/v1/apps/web/deploy-attempts", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DeployAttemptResource{
			{ID: "dep_1", ServiceName: "web", Image: "nginx:2", Status: "succeeded", StartedAt: time.Now()},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_deploy_attempts", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(list_deploy_attempts) error = %v", err)
	}
	var attempts []apiclient.DeployAttemptResource
	decodeStructured(t, result, &attempts)
	if len(attempts) != 1 || attempts[0].ID != "dep_1" {
		t.Errorf("attempts = %+v, want one attempt with id dep_1", attempts)
	}
}

func TestListDeployAttempts_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_deploy_attempts", Arguments: map[string]any{"name": "ghost"}})
	if err != nil {
		t.Fatalf("CallTool(list_deploy_attempts) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "app not found") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

func TestListDatabaseEngines(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/database-engines" {
			t.Errorf("path = %q, want /api/v1/database-engines", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DatabaseEngineResource{{ID: "postgres", Label: "PostgreSQL", DefaultVersion: "16"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_database_engines", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_database_engines) error = %v", err)
	}
	var engines []apiclient.DatabaseEngineResource
	decodeStructured(t, result, &engines)
	if len(engines) != 1 || engines[0].ID != "postgres" {
		t.Errorf("engines = %+v, want one engine with id postgres", engines)
	}
}

func TestGetSystemStatus(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/status" {
			t.Errorf("path = %q, want /api/v1/system/status", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.SystemStatusResource{SecretsConfigured: true, DockerConnected: true})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_system_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_system_status) error = %v", err)
	}
	var status apiclient.SystemStatusResource
	decodeStructured(t, result, &status)
	if !status.SecretsConfigured || !status.DockerConnected {
		t.Errorf("status = %+v, want secrets_configured and docker_connected true", status)
	}
}

func TestListScheduledTasks(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/scheduled-tasks" {
			t.Errorf("path = %q, want /api/v1/apps/web/scheduled-tasks", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ScheduledTaskResource{
			{ID: "task_1", ServiceName: "web", Command: []string{"php", "artisan", "cache:clear"}, Schedule: "0 * * * *", Enabled: true},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_scheduled_tasks", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(list_scheduled_tasks) error = %v", err)
	}
	var tasks []apiclient.ScheduledTaskResource
	decodeStructured(t, result, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "task_1" {
		t.Errorf("tasks = %+v, want one task with id task_1", tasks)
	}
}

func TestGetScheduledTask(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/scheduled-tasks/task_1" {
			t.Errorf("path = %q, want /api/v1/apps/web/scheduled-tasks/task_1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ScheduledTaskResource{
			ID: "task_1", ServiceName: "web", Command: []string{"php", "artisan", "cache:clear"},
			Schedule: "0 * * * *", Enabled: true, LastRunStatus: "succeeded",
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_scheduled_task",
		Arguments: map[string]any{"name": "web", "id": "task_1"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_scheduled_task) error = %v", err)
	}
	var task apiclient.ScheduledTaskResource
	decodeStructured(t, result, &task)
	if task.LastRunStatus != "succeeded" {
		t.Errorf("task.LastRunStatus = %q, want %q", task.LastRunStatus, "succeeded")
	}
}

func TestGetScheduledTask_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"scheduled task not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_scheduled_task",
		Arguments: map[string]any{"name": "web", "id": "ghost"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_scheduled_task) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "scheduled task not found") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

func TestListEnvironments(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects/proj_1/environments" {
			t.Errorf("path = %q, want /api/v1/projects/proj_1/environments", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.EnvironmentResource{{ID: "env_1", ProjectID: "proj_1", Name: "production", Protected: true}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_environments", Arguments: map[string]any{"project_id": "proj_1"}})
	if err != nil {
		t.Fatalf("CallTool(list_environments) error = %v", err)
	}
	var environments []apiclient.EnvironmentResource
	decodeStructured(t, result, &environments)
	if len(environments) != 1 || !environments[0].Protected {
		t.Errorf("environments = %+v, want one protected environment", environments)
	}
}

func TestListDomains(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/domains" {
			t.Errorf("path = %q, want /api/v1/domains", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DomainResource{{Domain: "app.example.com", ServiceName: "web"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_domains", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_domains) error = %v", err)
	}
	var domains []apiclient.DomainResource
	decodeStructured(t, result, &domains)
	if len(domains) != 1 || domains[0].ServiceName != "web" {
		t.Errorf("domains = %+v, want one domain owned by web", domains)
	}
}

func TestGetAppNetwork(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/network" {
			t.Errorf("path = %q, want /api/v1/apps/web/network", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.NetworkResource{ContainerPort: 3000, HostPort: 54321, Running: true})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_network", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_network) error = %v", err)
	}
	var network apiclient.NetworkResource
	decodeStructured(t, result, &network)
	if !network.Running || network.ContainerPort != 3000 {
		t.Errorf("network = %+v, want running=true container_port=3000", network)
	}
}

func TestGetDomainMaintenanceStatus(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/domains/app.example.com/maintenance" {
			t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/maintenance", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DomainMaintenanceResource{Domain: "app.example.com", Enabled: true})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_domain_maintenance_status",
		Arguments: map[string]any{"name": "web", "domain": "app.example.com"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_domain_maintenance_status) error = %v", err)
	}
	var status apiclient.DomainMaintenanceResource
	decodeStructured(t, result, &status)
	if !status.Enabled {
		t.Errorf("status.Enabled = false, want true")
	}
}

func TestGetCloudflareTunnelStatus(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/settings/cloudflare-tunnel" {
			t.Errorf("path = %q, want /api/v1/settings/cloudflare-tunnel", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.CloudflareTunnelResource{Enabled: true, HasToken: true, Status: "connected"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_cloudflare_tunnel_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_cloudflare_tunnel_status) error = %v", err)
	}
	var status apiclient.CloudflareTunnelResource
	decodeStructured(t, result, &status)
	if status.Status != "connected" || !status.HasToken {
		t.Errorf("status = %+v, want status=connected has_token=true", status)
	}
}

func TestListCertificates(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/certificates" {
			t.Errorf("path = %q, want /api/v1/certificates", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.CertificateResource{
			{Domain: "app.example.com", Status: "expiring_soon", NotAfter: time.Now().Add(48 * time.Hour)},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_certificates", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_certificates) error = %v", err)
	}
	var certs []apiclient.CertificateResource
	decodeStructured(t, result, &certs)
	if len(certs) != 1 || certs[0].Status != "expiring_soon" {
		t.Errorf("certs = %+v, want one expiring_soon certificate", certs)
	}
}

// TestPlatformVisibilityTools_Surface403 mirrors TestNewTools_Surface403
// (tools_test.go) for every tool added in this batch: each hits its own
// distinct API path and could regress independently of the others.
func TestPlatformVisibilityTools_Surface403(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"list_deploy_attempts", map[string]any{"name": "web"}},
		{"list_database_engines", map[string]any{}},
		{"get_system_status", map[string]any{}},
		{"list_scheduled_tasks", map[string]any{"name": "web"}},
		{"get_scheduled_task", map[string]any{"name": "web", "id": "task_1"}},
		{"list_environments", map[string]any{"project_id": "proj_1"}},
		{"list_domains", map[string]any{}},
		{"get_app_network", map[string]any{"name": "web"}},
		{"get_domain_maintenance_status", map[string]any{"name": "web", "domain": "app.example.com"}},
		{"get_cloudflare_tunnel_status", map[string]any{}},
		{"list_certificates", map[string]any{}},
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
