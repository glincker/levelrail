// Package fakerender is a read-only stand-in for the Render REST API,
// shaped from the documented responses the importer consumes. Any request
// that is not a GET is rejected and recorded, so a test can assert the
// importer never writes to Render.
package fakerender

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Fixture identifiers other packages assert on.
const (
	Token     = "rnd_fake-render-token" //nolint:gosec // fixture value
	SvcWeb    = "srv-web"
	SvcWorker = "srv-worker"
	SvcCron   = "srv-cron"
	SvcStatic = "srv-static"
	SecretVal = "render-session-secret-value" //nolint:gosec // fixture value
)

// Server is the fake. Create it with New and serve Handler.
type Server struct {
	mu       sync.Mutex
	requests []string
	// Empty makes the workspace hold nothing.
	Empty bool
	// PageSize caps every list page, 0 means 2 so a test crosses a page boundary.
	PageSize int
}

// New returns a fake with the default fixture.
func New() *Server { return &Server{} }

// Requests returns "METHOD path" for every request received.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// Violations returns every request that was not a GET.
func (s *Server) Violations() []string {
	var out []string
	for _, r := range s.Requests() {
		if !strings.HasPrefix(r, http.MethodGet+" ") {
			out = append(out, r)
		}
	}
	return out
}

type obj = map[string]any

func service(id, name, typ string, extra obj, details obj) obj {
	o := obj{"id": id, "name": name, "type": typ, "rootDir": "", "autoDeploy": "no", "suspended": "not_suspended", "serviceDetails": details}
	for k, v := range extra {
		o[k] = v
	}
	return o
}

func (s *Server) services() []obj {
	if s.Empty {
		return nil
	}
	return []obj{
		service(SvcWeb, "api", "web_service", obj{"repo": "https://github.com/acme/api", "branch": "main", "rootDir": "server", "autoDeploy": "yes"},
			obj{"runtime": "node", "plan": "starter", "region": "oregon", "numInstances": 2, "healthCheckPath": "/healthz",
				"disk":               obj{"id": "dsk-1", "name": "data", "sizeGB": 10, "mountPath": "/var/data"},
				"envSpecificDetails": obj{"buildCommand": "npm ci", "startCommand": "npm start"}}),
		service(SvcWorker, "mailer", "background_worker", obj{"imagePath": "ghcr.io/acme/mailer:1.2"},
			obj{"runtime": "image", "plan": "starter", "region": "oregon", "numInstances": 1, "envSpecificDetails": obj{}}),
		service(SvcCron, "nightly", "cron_job", obj{"repo": "https://github.com/acme/api", "branch": "main"},
			obj{"runtime": "node", "schedule": "0 3 * * *", "envSpecificDetails": obj{"startCommand": "node jobs/nightly.js"}}),
		service(SvcStatic, "site", "static_site", obj{"repo": "https://github.com/acme/site", "branch": "trunk"},
			obj{"buildCommand": "npm run build", "publishPath": "dist"}),
	}
}

// Handler serves the fake API. Lists are cut into pages of PageSize items.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	// page returns the items after the cursor, at most one page.
	page := func(r *http.Request, key string, items []obj) []obj {
		size := s.PageSize
		if size == 0 {
			size = 2
		}
		start := 0
		if c := strings.TrimPrefix(r.URL.Query().Get("cursor"), "c"); c != "" {
			n, _ := strconv.Atoi(c)
			start = n + 1
		}
		if start > len(items) {
			start = len(items)
		}
		end := min(start+size, len(items))
		out := make([]obj, 0, end-start)
		for i := start; i < end; i++ {
			out = append(out, obj{key: items[i], "cursor": "c" + strconv.Itoa(i)})
		}
		return out
	}
	mux.HandleFunc("/services", func(w http.ResponseWriter, r *http.Request) {
		write(w, page(r, "service", s.services()))
	})
	mux.HandleFunc("/services/{id}/env-vars", func(w http.ResponseWriter, r *http.Request) {
		var items []obj
		if r.PathValue("id") == SvcWeb {
			items = []obj{
				{"key": "NODE_ENV", "value": "production"},
				{"key": "SESSION_SECRET", "value": SecretVal},
				{"key": "JWT_SALT", "generateValue": true},
			}
		}
		write(w, page(r, "envVar", items))
	})
	mux.HandleFunc("/services/{id}/custom-domains", func(w http.ResponseWriter, r *http.Request) {
		var items []obj
		if r.PathValue("id") == SvcWeb {
			items = []obj{
				{"id": "d1", "name": "API.Example.com", "domainType": "subdomain", "verificationStatus": "verified"},
				{"id": "d2", "name": "api.onrender.com", "domainType": "subdomain", "verificationStatus": "verified"},
				{"id": "d3", "name": "example.com", "domainType": "apex", "verificationStatus": "verified"},
			}
		}
		write(w, page(r, "customDomain", items))
	})
	mux.HandleFunc("/postgres", func(w http.ResponseWriter, r *http.Request) {
		var items []obj
		if !s.Empty {
			items = []obj{{"id": "dpg-1", "name": "main-db", "version": "16", "plan": "basic_256mb", "diskSizeGB": 15, "region": "oregon", "databaseName": "app", "databaseUser": "app_user"}}
		}
		write(w, page(r, "postgres", items))
	})
	mux.HandleFunc("/key-value", func(w http.ResponseWriter, r *http.Request) {
		var items []obj
		if !s.Empty {
			items = []obj{{"id": "red-1", "name": "cache", "plan": "starter", "region": "oregon", "version": "8.1"}}
		}
		write(w, page(r, "keyValue", items))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		if r.Method != http.MethodGet {
			http.Error(w, `{"message":"read-only fake: only GET is allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+Token {
			http.Error(w, `{"id":"unauthorized","message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
