// Package exportfixture serves a read-only imitation of a Coolify API export
// for tests. The responses are written from the documented shapes, not
// captured from a real instance, and every credential in them is fake.
package exportfixture

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

//go:embed data/*.json
var data embed.FS

// Token is the bearer token the fixture accepts.
const Token = "fake-coolify-export-token" //nolint:gosec // fixture value

var routes = map[string]string{
	"/api/v1/projects":                               "projects.json",
	"/api/v1/projects/proj1/environments":            "environments.json",
	"/api/v1/projects/proj1/production":              "env_resources.json",
	"/api/v1/servers":                                "servers.json",
	"/api/v1/databases":                              "databases.json",
	"/api/v1/databases/db-pg/backups":                "backups_pg.json",
	"/api/v1/applications/app-web/envs":              "envs_web.json",
	"/api/v1/applications/app-web/storages":          "storages_web.json",
	"/api/v1/applications/app-web/scheduled-tasks":   "tasks_web.json",
	"/api/v1/applications/app-api/envs":              "empty_list.json",
	"/api/v1/applications/app-api/storages":          "empty_storages.json",
	"/api/v1/applications/app-api/scheduled-tasks":   "empty_list.json",
	"/api/v1/applications/app-docs/envs":             "empty_list.json",
	"/api/v1/applications/app-docs/storages":         "empty_storages.json",
	"/api/v1/applications/app-docs/scheduled-tasks":  "empty_list.json",
	"/api/v1/applications/app-umami/envs":            "empty_list.json",
	"/api/v1/applications/app-umami/storages":        "empty_storages.json",
	"/api/v1/applications/app-umami/scheduled-tasks": "empty_list.json",
}

// Server records every request so a test can assert the source was only read.
type Server struct {
	*httptest.Server
	mu   sync.Mutex
	seen []string
}

// Violations returns every request that was not a GET.
func (s *Server) Violations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, r := range s.seen {
		if len(r) < 4 || r[:4] != "GET " {
			out = append(out, r)
		}
	}
	return out
}

// New starts the fixture server and closes it when the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.seen = append(s.seen, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		if r.Method != http.MethodGet {
			http.Error(w, `{"message":"read-only fixture"}`, http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+Token {
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		file, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := data.ReadFile("data/" + file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(s.Close)
	return s
}
