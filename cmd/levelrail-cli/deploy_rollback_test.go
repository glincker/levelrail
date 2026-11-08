package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// rollbackServer serves a fresh app whose upload (or build trigger) fails, with the given
// deploy-attempts body, and records whether the app was deleted.
func rollbackServer(app, attempts string, failTrigger bool, deleted *bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+app:
			http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"` + app + `"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/"+app:
			*deleted = true
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/deploy-attempts"):
			_, _ = w.Write([]byte(attempts))
		case failTrigger && strings.HasSuffix(r.URL.Path, "/uploads"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"hash":"` + strings.Repeat("ab", 32) + `"}`))
		case failTrigger && strings.HasSuffix(r.URL.Path, "/builds"):
			http.Error(w, `{"error":"bad gateway"}`, http.StatusBadGateway)
		case strings.HasSuffix(r.URL.Path, "/uploads"):
			http.Error(w, `{"error":"disk full"}`, http.StatusInsufficientStorage)
		default:
			http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
		}
	}))
}

func TestDeployUpload_FailureRemovesTheAppItCreated(t *testing.T) {
	cases := []struct {
		name        string
		attempts    string
		failTrigger bool
		wantDeleted bool
		wantNote    string
	}{
		{"no attempts deletes", `[]`, false, true, "removed app"},
		{"attempts exist keeps the app", `[{"id":"dep_9"}]`, false, false, "apps delete"},
		{"attempts unknown keeps the app", `not json`, false, false, "apps delete"},
		{"build queued unknown keeps the app", `[]`, true, false, "apps delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeDeployDir(t, appDirFiles)
			var deleted bool
			srv := rollbackServer(filepath.Base(dir), tc.attempts, tc.failTrigger, &deleted)
			defer srv.Close()
			code, _, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok"}, nil)
			if code == exitOK || deleted != tc.wantDeleted || !strings.Contains(errOut, tc.wantNote) {
				t.Fatalf("exit = %d, deleted = %v, stderr = %s", code, deleted, errOut)
			}
		})
	}
}

func TestDeployStatic_FailureRollbackNeedsNoAttempts(t *testing.T) {
	cases := []struct {
		name        string
		attempts    string
		failTrigger bool
		wantDeleted bool
	}{
		{"no attempts deletes", `[]`, false, true},
		{"attempts exist keeps the app", `[{"id":"dep_9"}]`, false, false},
		{"build queued unknown keeps the app", `[]`, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site := writeDeployDir(t, map[string]string{"index.html": "hi"})
			var deleted bool
			srv := rollbackServer(filepath.Base(site), tc.attempts, tc.failTrigger, &deleted)
			defer srv.Close()
			code, _, errOut := runDeployCLI([]string{"deploy", site, "--static", "--yes", "--api-url", srv.URL, "--token", "tok"}, nil)
			if code == exitOK || deleted != tc.wantDeleted {
				t.Fatalf("exit = %d, deleted = %v, stderr = %s", code, deleted, errOut)
			}
		})
	}
}
