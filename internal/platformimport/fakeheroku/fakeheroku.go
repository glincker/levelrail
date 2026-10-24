// Package fakeheroku is a read-only stand-in for the Heroku Platform API,
// shaped from the documented responses the importer consumes. Any request
// that is not a GET is rejected and recorded, so a test can assert the
// importer never writes to Heroku.
package fakeheroku

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// Fixture identifiers other packages assert on.
const (
	Token       = "fake-heroku-token" //nolint:gosec // fixture value
	AppShop     = "app-shop-id"
	AppBare     = "app-bare-id"
	SessionKey  = "shop-session-secret-value"                            //nolint:gosec // fixture value
	DatabaseURL = "postgres://u:pw-from-addon@ec2.example.com:5432/shop" //nolint:gosec // fixture value
)

// Server is the fake. Create it with New and serve Handler.
type Server struct {
	mu       sync.Mutex
	requests []string
	ranges   []string
	// Empty makes the account hold no apps.
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

// Ranges returns the Range header of every /apps request.
func (s *Server) Ranges() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...)
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

func (s *Server) apps() []obj {
	if s.Empty {
		return list()
	}
	return list(
		obj{"id": AppShop, "name": "shop", "region": obj{"name": "us"}, "stack": obj{"name": "heroku-24"}, "team": obj{"name": "acme"}},
		obj{"id": AppBare, "name": "bare", "region": obj{"name": "eu"}, "stack": obj{"name": "heroku-24"}, "team": nil},
	)
}

func addon(id, name, service, plan string, vars ...string) obj {
	return obj{"id": id, "name": name, "addon_service": obj{"name": service}, "plan": obj{"name": plan}, "config_vars": vars}
}

// Handler serves the fake API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/apps", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.ranges = append(s.ranges, r.Header.Get("Range"))
		s.mu.Unlock()
		write(w, s.apps())
	})
	mux.HandleFunc("/apps/{id}/config-vars", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == AppShop {
			write(w, obj{"NODE_ENV": "production", "SESSION_SECRET": SessionKey, "DATABASE_URL": DatabaseURL,
				"REDIS_URL": "redis://:rpw@redis.example.com:6379", "SENDGRID_URL": "https://sg.example.com/x"})
			return
		}
		write(w, obj{})
	})
	mux.HandleFunc("/apps/{id}/formation", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == AppShop {
			write(w, list(obj{"type": "web", "quantity": 2, "size": "Standard-1X"},
				obj{"type": "worker", "quantity": 1, "size": "Standard-2X"},
				obj{"type": "clock", "quantity": 0, "size": "Standard-1X"}))
			return
		}
		write(w, list())
	})
	mux.HandleFunc("/apps/{id}/domains", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == AppShop {
			write(w, list(obj{"hostname": "Shop.Example.com", "kind": "custom"}, obj{"hostname": "shop-123.herokuapp.com", "kind": "heroku"}))
			return
		}
		write(w, list())
	})
	mux.HandleFunc("/apps/{id}/addons", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == AppShop {
			write(w, list(
				addon("ad-pg", "postgresql-round-1", "heroku-postgresql", "heroku-postgresql:standard-0", "DATABASE_URL"),
				addon("ad-rd", "redis-deep-2", "heroku-redis", "heroku-redis:premium-0", "REDIS_URL"),
				addon("ad-sg", "sendgrid-flat-3", "sendgrid", "sendgrid:starter", "SENDGRID_URL")))
			return
		}
		write(w, list())
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
			http.Error(w, `{"id":"unauthorized","message":"Invalid credentials provided."}`, http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "version=3") {
			http.Error(w, `{"id":"bad_request","message":"missing API version"}`, http.StatusBadRequest)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
