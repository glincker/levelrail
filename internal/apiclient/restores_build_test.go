package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_RestoresAndBuildHelpers(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBody = body["repo_url"] + "|" + body["ref"]
		w.Header().Set("Content-Type", "application/json")
		switch gotPath {
		case "/api/v1/build/detect":
			_, _ = w.Write([]byte(`{"provider":"node","framework_name":"Next.js","detected":true}`))
		case "/api/v1/git/branches":
			_, _ = w.Write([]byte(`{"branches":["main","dev"]}`))
		default:
			_, _ = w.Write([]byte(`[{"id":"x1","status":"succeeded"}]`))
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "t")
	ctx := context.Background()

	tests := []struct {
		name       string
		call       func() error
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"detect", func() error {
			r, err := c.DetectFramework(ctx, "https://x/y", "main")
			if err == nil && (!r.Detected || r.FrameworkName != "Next.js") {
				t.Errorf("detect = %+v", r)
			}
			return err
		}, http.MethodPost, "/api/v1/build/detect", "https://x/y|main"},
		{"branches", func() error {
			r, err := c.ListRepoBranches(ctx, "https://x/y")
			if err == nil && len(r.Branches) != 2 {
				t.Errorf("branches = %+v", r)
			}
			return err
		}, http.MethodPost, "/api/v1/git/branches", "https://x/y|"},
		{"restores", func() error {
			r, err := c.ListRestores(ctx, "db one")
			if err == nil && len(r) != 1 {
				t.Errorf("restores = %+v", r)
			}
			return err
		}, http.MethodGet, "/api/v1/databases/db%20one/restores", "|"},
		{"volume restores", func() error {
			_, err := c.ListVolumeRestores(ctx, "web", "data")
			return err
		}, http.MethodGet, "/api/v1/apps/web/volumes/data/restores", "|"},
		{"pitr restores", func() error {
			_, err := c.ListPITRRestores(ctx, "main")
			return err
		}, http.MethodGet, "/api/v1/databases/main/pitr-restores", "|"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("error = %v", err)
			}
			if gotMethod != tt.wantMethod || gotPath != tt.wantPath || gotBody != tt.wantBody {
				t.Errorf("request = %s %s body=%q, want %s %s body=%q", gotMethod, gotPath, gotBody, tt.wantMethod, tt.wantPath, tt.wantBody)
			}
		})
	}
}
