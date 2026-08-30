package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "structured error", data: []byte(`{"error":"name is required"}`), want: "name is required"},
		{name: "empty body", data: []byte(``), want: "(empty response body)"},
		{name: "non-json body falls back to raw text", data: []byte(`<html>502 Bad Gateway</html>`), want: "<html>502 Bad Gateway</html>"},
		{name: "json without error field falls back to raw text", data: []byte(`{"other":"x"}`), want: `{"other":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractErrorMessage(tt.data); got != tt.want {
				t.Errorf("ExtractErrorMessage(%s) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

func TestPathEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "web", want: "web"},
		{in: "a/b", want: "a%2Fb"},
		{in: "50%", want: "50%25"},
	}
	for _, tt := range tests {
		if got := PathEscape(tt.in); got != tt.want {
			t.Errorf("PathEscape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestClient_CreateApp(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody AppResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	req := AppResource{Name: "web", Image: "levelrail/web:1", Port: 3000}
	got, err := client.CreateApp(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Errorf("CreateApp() = %+v, want %+v", got, req)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps" {
		t.Errorf("path = %q, want /api/v1/apps", gotPath)
	}
}

func TestClient_APIErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"an app with this name already exists"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.CreateApp(context.Background(), AppResource{Name: "web", Image: "x:1", Port: 3000})
	if err == nil {
		t.Fatalf("CreateApp() error = nil, want an error for a 409 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusConflict)
	}
	if apiErr.Message != "an app with this name already exists" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "an app with this name already exists")
	}
}

func TestClient_NetworkError(t *testing.T) {
	// A server that's immediately closed guarantees connection refused,
	// a real transport-level failure rather than an HTTP error status.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	client := NewClient(url, "")
	_, err := client.ListApps(context.Background())
	if err == nil {
		t.Fatalf("ListApps() error = nil, want a network error")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("error type = *APIError, want a plain network error, got %v", err)
	}
}

func TestClient_RestartApp(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(AppResource{Name: "web", Image: "levelrail/web:1", Port: 3000})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.RestartApp(context.Background(), "web")
	if err != nil {
		t.Fatalf("RestartApp() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/restart" {
		t.Errorf("path = %q, want /api/v1/apps/web/restart", gotPath)
	}
	if gotBody != "" {
		t.Errorf("body = %q, want empty (no request body)", gotBody)
	}
	if got.Name != "web" {
		t.Errorf("Name = %q, want %q", got.Name, "web")
	}
}

func TestClient_RestartApp_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.RestartApp(context.Background(), "ghost")
	if err == nil {
		t.Fatalf("RestartApp() error = nil, want an error for a 404 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
}

func TestClient_DeployCompose(t *testing.T) {
	composeYAML := []byte("services:\n  web:\n    image: nginx:latest\n")
	var gotAuth, gotMethod, gotPath, gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ComposeDeployResult{
			AppID:    "myapp",
			Services: []AppResource{{Name: "web", Image: "nginx:latest", Port: 80}},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DeployCompose(context.Background(), "myapp", composeYAML)
	if err != nil {
		t.Fatalf("DeployCompose() error = %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/myapp/compose" {
		t.Errorf("path = %q, want /api/v1/apps/myapp/compose", gotPath)
	}
	if gotContentType != "text/yaml" {
		t.Errorf("Content-Type = %q, want text/yaml", gotContentType)
	}
	if !reflect.DeepEqual(gotBody, composeYAML) {
		t.Errorf("body = %q, want raw YAML %q, not JSON-encoded", gotBody, composeYAML)
	}
	want := ComposeDeployResult{AppID: "myapp", Services: []AppResource{{Name: "web", Image: "nginx:latest", Port: 80}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DeployCompose() = %+v, want %+v", got, want)
	}
}

func TestClient_DeployCompose_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid compose file"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.DeployCompose(context.Background(), "myapp", []byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatalf("DeployCompose() error = nil, want an error for a 400 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if apiErr.Message != "invalid compose file" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "invalid compose file")
	}
}

func TestClient_TriggerBuild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/builds" {
			t.Errorf("path = %q, want /api/v1/apps/web/builds", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(BuildTriggerResponse{
			Image: "levelrail/web:abc123",
			App:   AppResource{Name: "web", Image: "levelrail/web:abc123"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.TriggerBuild(context.Background(), "web", BuildTriggerRequest{RepoURL: "https://example.com/x.git", Ref: "main", ImageRepo: "levelrail/web"})
	if err != nil {
		t.Fatalf("TriggerBuild() error = %v", err)
	}
	if got.Image != "levelrail/web:abc123" {
		t.Errorf("Image = %q, want %q", got.Image, "levelrail/web:abc123")
	}
}

func TestClient_ListServiceTemplates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/service-templates" {
			t.Errorf("path = %q, want /api/v1/service-templates", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]ServiceTemplateListItem{{ID: "postgres", Name: "Postgres", Category: "database"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListServiceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListServiceTemplates() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "postgres" {
		t.Errorf("ListServiceTemplates() = %+v, want one entry with ID postgres", got)
	}
}

func TestClient_GetServiceTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/service-templates/postgres" {
			t.Errorf("path = %q, want /api/v1/service-templates/postgres", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ServiceTemplateDetail{ID: "postgres", Name: "Postgres", Compose: "services:\n  postgres:\n    image: postgres:16\n"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetServiceTemplate(context.Background(), "postgres")
	if err != nil {
		t.Fatalf("GetServiceTemplate() error = %v", err)
	}
	if got.Compose == "" {
		t.Errorf("GetServiceTemplate() Compose is empty, want the full compose body")
	}
}

func TestClient_GetServiceTemplate_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"service template not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetServiceTemplate(context.Background(), "ghost")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
}

func TestClient_CreateOrganization(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody CreateOrganizationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(OrganizationResource{ID: "org_1", Name: gotBody.Name, CreatedAt: "2026-08-16T00:00:00Z"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.CreateOrganization(context.Background(), CreateOrganizationRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/organizations" {
		t.Errorf("method/path = %s %s, want POST /api/v1/organizations", gotMethod, gotPath)
	}
	if got.ID != "org_1" || got.Name != "Acme" {
		t.Errorf("CreateOrganization() = %+v, want id org_1 name Acme", got)
	}
}

func TestClient_ListOrganizations(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]OrganizationResource{{ID: "org_1", Name: "Acme"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListOrganizations(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizations() error = %v", err)
	}
	if gotPath != "/api/v1/organizations" {
		t.Errorf("path = %q, want /api/v1/organizations", gotPath)
	}
	if len(got) != 1 || got[0].ID != "org_1" {
		t.Errorf("ListOrganizations() = %+v, want one org_1", got)
	}
}

func TestClient_GetOrganization(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OrganizationResource{ID: "org_1", Name: "Acme"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetOrganization(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("GetOrganization() error = %v", err)
	}
	if gotPath != "/api/v1/organizations/org_1" {
		t.Errorf("path = %q, want /api/v1/organizations/org_1", gotPath)
	}
	if got.Name != "Acme" {
		t.Errorf("Name = %q, want Acme", got.Name)
	}
}

func TestClient_DeleteOrganization(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DeleteOrganization(context.Background(), "org_1"); err != nil {
		t.Fatalf("DeleteOrganization() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/organizations/org_1" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/organizations/org_1", gotMethod, gotPath)
	}
}

func TestClient_CreateProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody CreateProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ProjectResource{ID: "proj_1", Name: gotBody.Name, CreatedAt: "2026-08-16T00:00:00Z"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.CreateProject(context.Background(), CreateProjectRequest{Name: "my-saas"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/projects" {
		t.Errorf("method/path = %s %s, want POST /api/v1/projects", gotMethod, gotPath)
	}
	if got.ID != "proj_1" || got.Name != "my-saas" {
		t.Errorf("CreateProject() = %+v, want id proj_1 name my-saas", got)
	}
}

func TestClient_ListProjects(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ProjectResource{{ID: "proj_1", Name: "my-saas"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if gotPath != "/api/v1/projects" {
		t.Errorf("path = %q, want /api/v1/projects", gotPath)
	}
	if len(got) != 1 || got[0].ID != "proj_1" {
		t.Errorf("ListProjects() = %+v, want one proj_1", got)
	}
}

func TestClient_GetProject(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ProjectResource{ID: "proj_1", Name: "my-saas"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetProject(context.Background(), "proj_1")
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if gotPath != "/api/v1/projects/proj_1" {
		t.Errorf("path = %q, want /api/v1/projects/proj_1", gotPath)
	}
	if got.Name != "my-saas" {
		t.Errorf("Name = %q, want my-saas", got.Name)
	}
}

func TestClient_DeleteProject(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DeleteProject(context.Background(), "proj_1"); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/projects/proj_1" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/projects/proj_1", gotMethod, gotPath)
	}
}

func TestClient_GetProjectEnv(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"LOG_LEVEL": "info"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetProjectEnv(context.Background(), "proj_1")
	if err != nil {
		t.Fatalf("GetProjectEnv() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/projects/proj_1/env" {
		t.Errorf("method/path = %s %s, want GET /api/v1/projects/proj_1/env", gotMethod, gotPath)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("GetProjectEnv() = %+v, want LOG_LEVEL=info", got)
	}
}

func TestClient_SetProjectEnv(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetProjectEnv(context.Background(), "proj_1", map[string]string{"LOG_LEVEL": "info"})
	if err != nil {
		t.Fatalf("SetProjectEnv() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/projects/proj_1/env" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/projects/proj_1/env", gotMethod, gotPath)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("SetProjectEnv() = %+v, want LOG_LEVEL=info", got)
	}
}

func TestClient_SetProjectOrganization(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetProjectOrganizationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ProjectResource{ID: "proj_1", Name: "web", OrgID: gotBody.OrgID})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetProjectOrganization(context.Background(), "proj_1", "org_1")
	if err != nil {
		t.Fatalf("SetProjectOrganization() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/projects/proj_1/organization" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/projects/proj_1/organization", gotMethod, gotPath)
	}
	if got.OrgID != "org_1" {
		t.Errorf("OrgID = %q, want org_1", got.OrgID)
	}
}

func TestClient_GetOrganizationEnv(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"LOG_LEVEL": "info"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetOrganizationEnv(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("GetOrganizationEnv() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/organizations/org_1/env" {
		t.Errorf("method/path = %s %s, want GET /api/v1/organizations/org_1/env", gotMethod, gotPath)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("GetOrganizationEnv() = %+v, want LOG_LEVEL=info", got)
	}
}

func TestClient_SetOrganizationEnv(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetOrganizationEnv(context.Background(), "org_1", map[string]string{"LOG_LEVEL": "info"})
	if err != nil {
		t.Fatalf("SetOrganizationEnv() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/organizations/org_1/env" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/organizations/org_1/env", gotMethod, gotPath)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("SetOrganizationEnv() = %+v, want LOG_LEVEL=info", got)
	}
}

func TestClient_GetEnvironmentEnv(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"LOG_LEVEL": "info"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetEnvironmentEnv(context.Background(), "env_1")
	if err != nil {
		t.Fatalf("GetEnvironmentEnv() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/environments/env_1/env" {
		t.Errorf("method/path = %s %s, want GET /api/v1/environments/env_1/env", gotMethod, gotPath)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("GetEnvironmentEnv() = %+v, want LOG_LEVEL=info", got)
	}
}

func TestClient_SetEnvironmentEnv(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetEnvironmentEnv(context.Background(), "env_1", map[string]string{"LOG_LEVEL": "info"})
	if err != nil {
		t.Fatalf("SetEnvironmentEnv() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/environments/env_1/env" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/environments/env_1/env", gotMethod, gotPath)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("SetEnvironmentEnv() = %+v, want LOG_LEVEL=info", got)
	}
}

func TestClient_CreateEnvironment(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody CreateEnvironmentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(EnvironmentResource{ID: "env_1", ProjectID: "proj_1", Name: gotBody.Name})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.CreateEnvironment(context.Background(), "proj_1", CreateEnvironmentRequest{Name: "staging"})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/projects/proj_1/environments" {
		t.Errorf("method/path = %s %s, want POST /api/v1/projects/proj_1/environments", gotMethod, gotPath)
	}
	if got.ID != "env_1" || got.Name != "staging" {
		t.Errorf("CreateEnvironment() = %+v, want id env_1 name staging", got)
	}
}

func TestClient_ListEnvironments(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]EnvironmentResource{{ID: "env_1", ProjectID: "proj_1", Name: "staging"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListEnvironments(context.Background(), "proj_1")
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if gotPath != "/api/v1/projects/proj_1/environments" {
		t.Errorf("path = %q, want /api/v1/projects/proj_1/environments", gotPath)
	}
	if len(got) != 1 || got[0].ID != "env_1" {
		t.Errorf("ListEnvironments() = %+v, want one env_1", got)
	}
}

func TestClient_DeleteEnvironment(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DeleteEnvironment(context.Background(), "env_1"); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/environments/env_1" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/environments/env_1", gotMethod, gotPath)
	}
}

func TestClient_SetAppEnvironment(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetAppEnvironmentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AppResource{Name: "web", EnvironmentID: gotBody.EnvironmentID})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetAppEnvironment(context.Background(), "web", "env_1")
	if err != nil {
		t.Fatalf("SetAppEnvironment() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/environment" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/environment", gotMethod, gotPath)
	}
	if got.EnvironmentID != "env_1" {
		t.Errorf("EnvironmentID = %q, want env_1", got.EnvironmentID)
	}
}

func TestClient_SetAppProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetAppProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AppResource{Name: "web", ProjectID: gotBody.ProjectID})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetAppProject(context.Background(), "web", "proj_1")
	if err != nil {
		t.Fatalf("SetAppProject() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/project" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/project", gotMethod, gotPath)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1", got.ProjectID)
	}
}

func TestClient_SetDatabaseProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetDatabaseProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DatabaseResource{Name: "db", ProjectID: gotBody.ProjectID})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetDatabaseProject(context.Background(), "db", "proj_1")
	if err != nil {
		t.Fatalf("SetDatabaseProject() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/db/project" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/databases/db/project", gotMethod, gotPath)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1", got.ProjectID)
	}
}

func TestClient_GetCloudflareTunnel(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CloudflareTunnelResource{Enabled: true, HasToken: true, Status: "connected"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetCloudflareTunnel(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareTunnel() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/cloudflare-tunnel" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/cloudflare-tunnel", gotMethod, gotPath)
	}
	if !got.Enabled || got.Status != "connected" {
		t.Errorf("GetCloudflareTunnel() = %+v, want Enabled=true Status=connected", got)
	}
}

func TestClient_SetCloudflareTunnel(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody UpdateCloudflareTunnelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CloudflareTunnelResource{Enabled: gotBody.Enabled, HasToken: true})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetCloudflareTunnel(context.Background(), UpdateCloudflareTunnelRequest{Enabled: true, Token: "cf-tunnel-token"})
	if err != nil {
		t.Fatalf("SetCloudflareTunnel() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/cloudflare-tunnel" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/cloudflare-tunnel", gotMethod, gotPath)
	}
	if gotBody.Token != "cf-tunnel-token" || !gotBody.Enabled {
		t.Errorf("request body = %+v, want Enabled=true Token=cf-tunnel-token", gotBody)
	}
	if !got.Enabled {
		t.Errorf("SetCloudflareTunnel() = %+v, want Enabled=true", got)
	}
}

func TestClient_DisconnectCloudflareTunnel(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CloudflareTunnelResource{Enabled: false, HasToken: false, Status: "disconnected"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DisconnectCloudflareTunnel(context.Background())
	if err != nil {
		t.Fatalf("DisconnectCloudflareTunnel() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/cloudflare-tunnel" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/cloudflare-tunnel", gotMethod, gotPath)
	}
	if got.Enabled || got.HasToken {
		t.Errorf("DisconnectCloudflareTunnel() = %+v, want Enabled=false HasToken=false", got)
	}
}
