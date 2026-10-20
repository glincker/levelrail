package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetSystemStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SystemStatusResource{
			DockerConnected: false,
			DockerError:     "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/system/status" {
		t.Errorf("request = %s %s, want GET /api/v1/system/status", gotMethod, gotPath)
	}
	if got.DockerConnected {
		t.Error("DockerConnected = true, want false")
	}
	if got.DockerError == "" {
		t.Error("DockerError = \"\", want the daemon error message")
	}
}
