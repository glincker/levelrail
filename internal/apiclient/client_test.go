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
