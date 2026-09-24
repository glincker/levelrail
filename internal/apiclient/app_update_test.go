package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_UpdateApp(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody AppResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UpdateApp(context.Background(), "web", AppResource{Name: "web", Env: map[string]string{"A": "1"}})
	if err != nil {
		t.Fatalf("UpdateApp() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web", gotMethod, gotPath)
	}
	if got.Env["A"] != "1" {
		t.Errorf("Env = %v, want A=1", got.Env)
	}
}
