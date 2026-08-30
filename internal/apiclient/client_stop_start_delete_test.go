package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_AppLifecycleActions table-drives the wire shape of the app
// stop/start/delete methods added alongside the existing RestartApp:
// method, path, and (for stop/start) that the response decodes.
func TestClient_AppLifecycleActions(t *testing.T) {
	tests := []struct {
		name       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
	}{
		{
			name:       "StopApp",
			call:       func(c *Client) error { _, err := c.StopApp(context.Background(), "web"); return err },
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/apps/web/stop",
		},
		{
			name:       "StartApp",
			call:       func(c *Client) error { _, err := c.StartApp(context.Background(), "web"); return err },
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/apps/web/start",
		},
		{
			name:       "DeleteApp",
			call:       func(c *Client) error { return c.DeleteApp(context.Background(), "web") },
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/apps/web",
		},
		{
			name:       "DeleteDatabase",
			call:       func(c *Client) error { return c.DeleteDatabase(context.Background(), "main") },
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/databases/main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(AppResource{Name: "web"})
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "test-token")
			if err := tt.call(client); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("method = %q, want %q", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

func TestClient_SetSecret_OverwriteLocked(t *testing.T) {
	var gotBody SetSecretRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.SetSecret(context.Background(), "web", "API_KEY", "s3cr3t", true); err != nil {
		t.Fatalf("SetSecret() error = %v", err)
	}
	if gotBody.Value != "s3cr3t" || !gotBody.OverwriteLocked {
		t.Errorf("body = %+v, want value=s3cr3t overwrite_locked=true", gotBody)
	}
}

func TestClient_ListSecrets(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SecretKeyResource{{Key: "API_KEY", Locked: true}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListSecrets(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}
	if gotPath != "/api/v1/apps/web/secrets" {
		t.Errorf("path = %q, want /api/v1/apps/web/secrets", gotPath)
	}
	if len(got) != 1 || got[0].Key != "API_KEY" || !got[0].Locked {
		t.Errorf("ListSecrets() = %+v, want one locked API_KEY entry", got)
	}
}

func TestClient_SetSecretLock(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody SetSecretLockRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.SetSecretLock(context.Background(), "web", "API_KEY", true); err != nil {
		t.Fatalf("SetSecretLock() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/secrets/API_KEY/lock" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/secrets/API_KEY/lock", gotMethod, gotPath)
	}
	if !gotBody.Locked {
		t.Errorf("body.Locked = false, want true")
	}
}

func TestClient_GitSourceLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
	}{
		{
			name:       "GetGitSource",
			call:       func(c *Client) error { _, err := c.GetGitSource(context.Background(), "web"); return err },
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/apps/web/git-source",
		},
		{
			name: "SetGitSource",
			call: func(c *Client) error {
				_, err := c.SetGitSource(context.Background(), "web", SetGitSourceRequest{RepoURL: "https://github.com/acme/web"})
				return err
			},
			wantMethod: http.MethodPut,
			wantPath:   "/api/v1/apps/web/git-source",
		},
		{
			name:       "DeleteGitSource",
			call:       func(c *Client) error { return c.DeleteGitSource(context.Background(), "web") },
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/apps/web/git-source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(GitSourceResource{ServiceName: "web"})
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "test-token")
			if err := tt.call(client); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("method = %q, want %q", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}
