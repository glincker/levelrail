// Package fakevercel is a read-only stand-in for the Vercel REST API,
// shaped from the documented responses the importer consumes. Any request
// that is not a GET is rejected and recorded, so a test can assert the
// importer never writes to Vercel.
package fakevercel

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// Fixture identifiers other packages assert on.
const (
	Token     = "fake-vercel-token" //nolint:gosec // fixture value
	TeamID    = "team_acme"
	ProjWeb   = "prj_web"
	ProjDocs  = "prj_docs"
	ProjBare  = "prj_bare"
	SecretVal = "enc-blob-should-never-surface" //nolint:gosec // fixture value
)

// Server is the fake. Create it with New and serve Handler.
type Server struct {
	mu       sync.Mutex
	requests []string
	queries  []string
	// Empty makes the team hold no projects.
	Empty bool
}

// New returns a fake with the default fixture.
func New() *Server { return &Server{} }

// Requests returns "METHOD path" for every request received.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// Queries returns the raw query of every request received.
func (s *Server) Queries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.queries...)
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

func list(items ...obj) []obj {
	if items == nil {
		return []obj{}
	}
	return items
}

func env(key, typ, value string, targets ...string) obj {
	return obj{"key": key, "type": typ, "value": value, "target": targets}
}

// Handler serves the fake API. The project list is split over two pages.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	all := []string{"production", "preview", "development"}
	mux.HandleFunc("/v10/projects", func(w http.ResponseWriter, r *http.Request) {
		if s.Empty {
			write(w, obj{"projects": list(), "pagination": obj{"count": 0, "next": nil}})
			return
		}
		if r.URL.Query().Get("from") == "" {
			write(w, obj{"projects": list(
				obj{"id": ProjWeb, "name": "web", "framework": "nextjs", "buildCommand": "pnpm build", "installCommand": "pnpm i",
					"outputDirectory": ".next", "rootDirectory": "apps/web", "nodeVersion": "20.x",
					"link": obj{"type": "github", "org": "acme", "repo": "mono", "productionBranch": "main"}},
				obj{"id": ProjDocs, "name": "docs", "framework": "astro", "nodeVersion": "22.x",
					"link": obj{"type": "gitlab", "org": "acme", "repo": "docs", "productionBranch": "trunk"}},
			), "pagination": obj{"count": 2, "next": "tok-page-2"}})
			return
		}
		write(w, obj{"projects": list(obj{"id": ProjBare, "name": "bare", "link": nil}), "pagination": obj{"count": 1, "next": nil}})
	})
	mux.HandleFunc("/v10/projects/{id}/env", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("decrypt") == "true" {
			http.Error(w, `{"error":{"code":"forbidden","message":"decrypt not allowed in this test"}}`, http.StatusForbidden)
			return
		}
		if r.PathValue("id") == ProjWeb {
			write(w, obj{"envs": list(
				env("NODE_ENV", "plain", "production", all...),
				env("API_KEY", "sensitive", "", all...),
				env("DB_PASS", "encrypted", SecretVal, all...),
				env("PREVIEW_ONLY", "plain", "yes", "preview"),
				env("FEATURE", "plain", "prod-on", "production"),
				env("FEATURE", "plain", "dev-off", "development"),
			), "pagination": obj{"count": 6, "next": nil}})
			return
		}
		write(w, obj{"envs": list(), "pagination": obj{"count": 0, "next": nil}})
	})
	mux.HandleFunc("/v9/projects/{id}/domains", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != ProjWeb {
			write(w, obj{"domains": list(), "pagination": obj{"count": 0, "next": nil}})
			return
		}
		if r.URL.Query().Get("until") == "" {
			write(w, obj{"domains": list(obj{"name": "Web.Example.com"}, obj{"name": "web-abc.vercel.app"}),
				"pagination": obj{"count": 2, "next": 1700000000000}})
			return
		}
		write(w, obj{"domains": list(obj{"name": "www.example.com"}, obj{"name": "old.example.com", "redirect": "web.example.com"}),
			"pagination": obj{"count": 2, "next": nil}})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.queries = append(s.queries, r.URL.RawQuery)
		s.mu.Unlock()
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":{"message":"read-only fake: only GET is allowed"}}`, http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+Token {
			http.Error(w, `{"error":{"code":"forbidden","message":"Not authorized"}}`, http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
