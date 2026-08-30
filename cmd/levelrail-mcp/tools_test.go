package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestSession starts a fake control-plane API server driven by
// apiHandler, wires a real *apiclient.Client to it, registers every tool
// against that client on a fresh *mcp.Server, and connects an in-process
// MCP client to it over mcp.NewInMemoryTransports: the SDK's own
// in-process test harness, so this exercises the real tool-call wire
// path (schema validation, JSON marshaling, error mapping) without a
// subprocess or real stdio.
func newTestSession(t *testing.T, apiHandler http.HandlerFunc) *mcp.ClientSession {
	t.Helper()

	apiSrv := httptest.NewServer(apiHandler)
	t.Cleanup(apiSrv.Close)

	client := apiclient.NewClient(apiSrv.URL, "test-token")
	server := newServer(client)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()
	go func() { _ = server.Run(ctx, serverTransport) }()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// decodeStructured unmarshals a successful CallToolResult's
// StructuredContent into out, failing the test on any error or on an
// unexpected IsError result.
func decodeStructured(t *testing.T, result *mcp.CallToolResult, out any) {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool call returned an error result: %s", toolResultText(result))
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal StructuredContent into %T: %v", out, err)
	}
}

func toolResultText(result *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestListApps(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps" {
			t.Errorf("request = %s %s, want GET /api/v1/apps", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.AppResource{{Name: "web", Image: "nginx:1", Port: 80}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_apps", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_apps) error = %v", err)
	}
	var apps []apiclient.AppResource
	decodeStructured(t, result, &apps)
	if len(apps) != 1 || apps[0].Name != "web" {
		t.Errorf("apps = %+v, want one app named web", apps)
	}
}

func TestGetApp(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web" {
			t.Errorf("path = %q, want /api/v1/apps/web", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Image: "nginx:1", Port: 80})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app) error = %v", err)
	}
	var app apiclient.AppResource
	decodeStructured(t, result, &app)
	if app.Name != "web" {
		t.Errorf("app.Name = %q, want %q", app.Name, "web")
	}
}

func TestDeployApp(t *testing.T) {
	var gotBody map[string]string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/deploys" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/deploys", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Image: "nginx:2", Port: 80})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "deploy_app",
		Arguments: map[string]any{"name": "web", "image": "nginx:2"},
	})
	if err != nil {
		t.Fatalf("CallTool(deploy_app) error = %v", err)
	}
	var app apiclient.AppResource
	decodeStructured(t, result, &app)
	if app.Image != "nginx:2" {
		t.Errorf("app.Image = %q, want %q", app.Image, "nginx:2")
	}
	if gotBody["image"] != "nginx:2" {
		t.Errorf("request body image = %q, want %q", gotBody["image"], "nginx:2")
	}
}

func TestDeployCompose(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/myapp/compose" {
			t.Errorf("path = %q, want /api/v1/apps/myapp/compose", r.URL.Path)
		}
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ComposeDeployResult{
			AppID:    "myapp",
			Services: []apiclient.AppResource{{Name: "web", Image: "nginx:latest", Port: 80}},
		})
	})

	compose := "services:\n  web:\n    image: nginx:latest\n"
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "deploy_compose",
		Arguments: map[string]any{"name": "myapp", "compose": compose},
	})
	if err != nil {
		t.Fatalf("CallTool(deploy_compose) error = %v", err)
	}
	var out apiclient.ComposeDeployResult
	decodeStructured(t, result, &out)
	if out.AppID != "myapp" || len(out.Services) != 1 {
		t.Errorf("ComposeDeployResult = %+v, want AppID=myapp with one service", out)
	}
	if gotContentType != "text/yaml" {
		t.Errorf("Content-Type = %q, want text/yaml", gotContentType)
	}
	if string(gotBody) != compose {
		t.Errorf("request body = %q, want raw compose YAML %q", gotBody, compose)
	}
}

func TestRestartApp(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/restart" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/restart", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Image: "nginx:1", Port: 80})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "restart_app", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(restart_app) error = %v", err)
	}
	var app apiclient.AppResource
	decodeStructured(t, result, &app)
	if app.Name != "web" {
		t.Errorf("app.Name = %q, want %q", app.Name, "web")
	}
}

func TestGetAppStatus(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/deploys" {
			t.Errorf("path = %q, want /api/v1/apps/web/deploys", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ConditionResource{{Type: "Ready", Status: "True", Reason: "Reconciled"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_status", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_status) error = %v", err)
	}
	var conditions []apiclient.ConditionResource
	decodeStructured(t, result, &conditions)
	if len(conditions) != 1 || conditions[0].Type != "Ready" {
		t.Errorf("conditions = %+v, want one Ready condition", conditions)
	}
}

func TestListDeploys(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/deploys" {
			t.Errorf("path = %q, want /api/v1/apps/web/deploys", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ConditionResource{{Type: "Ready", Status: "True"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_deploys", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(list_deploys) error = %v", err)
	}
	var conditions []apiclient.ConditionResource
	decodeStructured(t, result, &conditions)
	if len(conditions) != 1 {
		t.Errorf("conditions = %+v, want one entry", conditions)
	}
}

func TestGetAppLogs(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/logs" {
			t.Errorf("path = %q, want /api/v1/apps/web/logs", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		entries := make([]apiclient.LogEntryResource, 0, 250)
		for i := 0; i < 250; i++ {
			entries = append(entries, apiclient.LogEntryResource{Message: "line"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": entries})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_app_logs",
		Arguments: map[string]any{"name": "web", "query": "panic"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_app_logs) error = %v", err)
	}
	var entries []apiclient.LogEntryResource
	decodeStructured(t, result, &entries)
	if len(entries) != maxLogTail {
		t.Errorf("len(entries) = %d, want the hard cap %d even though the server returned more", len(entries), maxLogTail)
	}
	if gotQuery != "panic" {
		t.Errorf("q query param = %q, want %q", gotQuery, "panic")
	}
}

func TestGetAppLogs_InvalidSince(t *testing.T) {
	session := newTestSession(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("API server should not be called for an invalid --since")
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_app_logs",
		Arguments: map[string]any{"name": "web", "since": "not-a-duration"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_app_logs) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for an invalid since value")
	}
	if !strings.Contains(toolResultText(result), "invalid since") {
		t.Errorf("error text = %q, want it to mention the invalid since value", toolResultText(result))
	}
}

func TestListDatabases(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/databases" {
			t.Errorf("path = %q, want /api/v1/databases", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DatabaseResource{{Name: "main", Engine: "postgres", Version: "16"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_databases", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_databases) error = %v", err)
	}
	var databases []apiclient.DatabaseResource
	decodeStructured(t, result, &databases)
	if len(databases) != 1 || databases[0].Engine != "postgres" {
		t.Errorf("databases = %+v, want one postgres database", databases)
	}
}

func TestGetDatabase(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/databases/main" {
			t.Errorf("path = %q, want /api/v1/databases/main", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DatabaseResource{Name: "main", Engine: "postgres", Version: "16"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_database", Arguments: map[string]any{"name": "main"}})
	if err != nil {
		t.Fatalf("CallTool(get_database) error = %v", err)
	}
	var database apiclient.DatabaseResource
	decodeStructured(t, result, &database)
	if database.Name != "main" {
		t.Errorf("database.Name = %q, want %q", database.Name, "main")
	}
}

func TestListServiceTemplates(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/service-templates" {
			t.Errorf("path = %q, want /api/v1/service-templates", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ServiceTemplateListItem{{ID: "postgres", Name: "Postgres", Category: "database"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_service_templates", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_service_templates) error = %v", err)
	}
	var templates []apiclient.ServiceTemplateListItem
	decodeStructured(t, result, &templates)
	if len(templates) != 1 || templates[0].ID != "postgres" {
		t.Errorf("templates = %+v, want one postgres entry", templates)
	}
}

func TestGetServiceTemplate(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/service-templates/postgres" {
			t.Errorf("path = %q, want /api/v1/service-templates/postgres", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ServiceTemplateDetail{ID: "postgres", Name: "Postgres", Compose: "services:\n  postgres:\n    image: postgres:16\n"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_service_template", Arguments: map[string]any{"id": "postgres"}})
	if err != nil {
		t.Fatalf("CallTool(get_service_template) error = %v", err)
	}
	var template apiclient.ServiceTemplateDetail
	decodeStructured(t, result, &template)
	if template.Compose == "" {
		t.Errorf("template.Compose is empty, want the full compose body")
	}
}

// TestToolError_SurfacesAPIMessage is the "403 should clearly say the
// token lacks the required ability" contract: a REST-level error must
// come back as a real MCP tool error whose text is the control plane's
// own message, not a generic "request failed".
func TestToolError_SurfacesAPIMessage(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"token lacks the required ability"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_apps", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_apps) transport error = %v", err)
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
}

func TestRollbackApp(t *testing.T) {
	var gotBody map[string]string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/deploys" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/deploys", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Image: "nginx:1", Port: 80})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "rollback_app",
		Arguments: map[string]any{"name": "web", "image": "nginx:1"},
	})
	if err != nil {
		t.Fatalf("CallTool(rollback_app) error = %v", err)
	}
	var app apiclient.AppResource
	decodeStructured(t, result, &app)
	if app.Image != "nginx:1" {
		t.Errorf("app.Image = %q, want %q", app.Image, "nginx:1")
	}
	if gotBody["image"] != "nginx:1" {
		t.Errorf("request body image = %q, want %q", gotBody["image"], "nginx:1")
	}
}

func TestListNodes(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/nodes" {
			t.Errorf("request = %s %s, want GET /api/v1/nodes", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.NodeResource{{ID: "n1", Name: "node-1", Status: "ready"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_nodes", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_nodes) error = %v", err)
	}
	var nodes []apiclient.NodeResource
	decodeStructured(t, result, &nodes)
	if len(nodes) != 1 || nodes[0].ID != "n1" {
		t.Errorf("nodes = %+v, want one node with id n1", nodes)
	}
}

func TestGetNode(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/n1" {
			t.Errorf("path = %q, want /api/v1/nodes/n1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.NodeResource{ID: "n1", Name: "node-1", Status: "ready"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_node", Arguments: map[string]any{"id": "n1"}})
	if err != nil {
		t.Fatalf("CallTool(get_node) error = %v", err)
	}
	var node apiclient.NodeResource
	decodeStructured(t, result, &node)
	if node.ID != "n1" {
		t.Errorf("node.ID = %q, want %q", node.ID, "n1")
	}
}

func TestGetNodeHealth(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/n1/health" {
			t.Errorf("path = %q, want /api/v1/nodes/n1/health", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ConditionResource{{Type: "Ready", Status: "True"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_node_health", Arguments: map[string]any{"id": "n1"}})
	if err != nil {
		t.Fatalf("CallTool(get_node_health) error = %v", err)
	}
	var conditions []apiclient.ConditionResource
	decodeStructured(t, result, &conditions)
	if len(conditions) != 1 || conditions[0].Type != "Ready" {
		t.Errorf("conditions = %+v, want one Ready condition", conditions)
	}
}

func TestListPreviewEnvironments(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/previews" {
			t.Errorf("path = %q, want /api/v1/apps/web/previews", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.PreviewEnvironmentResource{{PRNumber: 42, Branch: "feature-x", Status: "ready"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_preview_environments", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(list_preview_environments) error = %v", err)
	}
	var previews []apiclient.PreviewEnvironmentResource
	decodeStructured(t, result, &previews)
	if len(previews) != 1 || previews[0].PRNumber != 42 {
		t.Errorf("previews = %+v, want one preview for PR 42", previews)
	}
}

func TestListAlertRules(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/alerts" {
			t.Errorf("path = %q, want /api/v1/apps/web/alerts", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.AlertRuleResource{{ID: "r1", Name: "high-cpu", Kind: "threshold", Enabled: true}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_alert_rules", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(list_alert_rules) error = %v", err)
	}
	var rules []apiclient.AlertRuleResource
	decodeStructured(t, result, &rules)
	if len(rules) != 1 || rules[0].ID != "r1" {
		t.Errorf("rules = %+v, want one rule with id r1", rules)
	}
}

func TestGetAppMetrics(t *testing.T) {
	var gotQuery url.Values
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/metrics" {
			t.Errorf("path = %q, want /api/v1/apps/web/metrics", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppMetricsResource{
			Metric: "cpu_percent",
			Points: []apiclient.MetricPointResource{{Value: 12.5, Count: 4}},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_app_metrics",
		Arguments: map[string]any{"name": "web", "metric": "cpu_percent", "since": "30m", "step": "60s"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_app_metrics) error = %v", err)
	}
	var metrics apiclient.AppMetricsResource
	decodeStructured(t, result, &metrics)
	if metrics.Metric != "cpu_percent" || len(metrics.Points) != 1 {
		t.Errorf("metrics = %+v, want one cpu_percent point", metrics)
	}
	if gotQuery.Get("metric") != "cpu_percent" {
		t.Errorf("metric query param = %q, want %q", gotQuery.Get("metric"), "cpu_percent")
	}
	if gotQuery.Get("step") != "1m0s" {
		t.Errorf("step query param = %q, want %q", gotQuery.Get("step"), "1m0s")
	}
}

func TestGetAppMetrics_InvalidSince(t *testing.T) {
	session := newTestSession(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("API server should not be called for an invalid since")
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_app_metrics",
		Arguments: map[string]any{"name": "web", "metric": "cpu_percent", "since": "not-a-duration"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_app_metrics) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for an invalid since value")
	}
	if !strings.Contains(toolResultText(result), "invalid since") {
		t.Errorf("error text = %q, want it to mention the invalid since value", toolResultText(result))
	}
}

func TestGetAppMetrics_InvalidStep(t *testing.T) {
	session := newTestSession(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("API server should not be called for an invalid step")
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_app_metrics",
		Arguments: map[string]any{"name": "web", "metric": "cpu_percent", "step": "not-a-duration"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_app_metrics) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for an invalid step value")
	}
	if !strings.Contains(toolResultText(result), "invalid step") {
		t.Errorf("error text = %q, want it to mention the invalid step value", toolResultText(result))
	}
}

// TestNewTools_Surface403 is the same "ability-check 403 surfaces as a
// real MCP tool error with the server's own message" contract
// TestToolError_SurfacesAPIMessage already covers for list_apps, checked
// individually for every tool added alongside rollback/nodes/previews/
// alerts/metrics, since each hits its own distinct API path and any of
// them could regress independently of the others.
func TestNewTools_Surface403(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"rollback_app", map[string]any{"name": "web", "image": "nginx:1"}},
		{"list_nodes", map[string]any{}},
		{"get_node", map[string]any{"id": "n1"}},
		{"get_node_health", map[string]any{"id": "n1"}},
		{"list_preview_environments", map[string]any{"name": "web"}},
		{"list_alert_rules", map[string]any{"name": "web"}},
		{"get_app_metrics", map[string]any{"name": "web", "metric": "cpu_percent"}},
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

func TestToolError_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app", Arguments: map[string]any{"name": "ghost"}})
	if err != nil {
		t.Fatalf("CallTool(get_app) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "app not found") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}
