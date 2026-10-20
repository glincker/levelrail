package registrycatalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListRepositories(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantRepos  []string
		wantErr    bool
		wantNotFnd bool
	}{
		{
			name: "success sorted",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/_catalog" {
					t.Errorf("path = %q, want /v2/_catalog", r.URL.Path)
				}
				user, pass, ok := r.BasicAuth()
				if !ok || user != "levelrail" || pass != "hunter2" {
					t.Errorf("basic auth = %q/%q/%v, want levelrail/hunter2/true", user, pass, ok)
				}
				_, _ = w.Write([]byte(`{"repositories":["zeta","alpha","beta"]}`))
			},
			wantRepos: []string{"alpha", "beta", "zeta"},
		},
		{
			name: "empty catalog",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"repositories":[]}`))
			},
			wantRepos: nil,
		},
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: true,
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`not json`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			c := NewClient()
			repos, err := c.ListRepositories(context.Background(), srv.URL, "levelrail", "hunter2")
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if !equalStrings(repos, tt.wantRepos) {
				t.Errorf("repos = %v, want %v", repos, tt.wantRepos)
			}
		})
	}
}

func TestClient_ListTags(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		handler    http.HandlerFunc
		wantTags   []string
		wantErr    error
	}{
		{
			name:       "success sorted",
			repository: "myapp",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/myapp/tags/list" {
					t.Errorf("path = %q, want /v2/myapp/tags/list", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"name":"myapp","tags":["v2","v1","latest"]}`))
			},
			wantTags: []string{"latest", "v1", "v2"},
		},
		{
			name:       "namespaced repository keeps its path segments",
			repository: "org/myapp",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/org/myapp/tags/list" {
					t.Errorf("path = %q, want /v2/org/myapp/tags/list", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"name":"org/myapp","tags":["latest"]}`))
			},
			wantTags: []string{"latest"},
		},
		{
			name:       "not found repository",
			repository: "ghost",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: ErrNotFound,
		},
		{
			name:       "server error",
			repository: "myapp",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: errors.New("unexpected status"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			c := NewClient()
			tags, err := c.ListTags(context.Background(), srv.URL, "levelrail", "hunter2", tt.repository)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("error = nil, want an error")
				}
				if errors.Is(tt.wantErr, ErrNotFound) && !errors.Is(err, ErrNotFound) {
					t.Errorf("error = %v, want wrapping ErrNotFound", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if !equalStrings(tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", tags, tt.wantTags)
			}
		})
	}
}

func TestClient_UnreachableServer(t *testing.T) {
	c := NewClient()
	_, err := c.ListRepositories(context.Background(), "http://127.0.0.1:1", "u", "p")
	if err == nil {
		t.Fatal("error = nil, want an error dialing an unreachable server")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
