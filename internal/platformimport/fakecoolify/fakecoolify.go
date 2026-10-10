// Package fakecoolify is a read-only stand-in for the Coolify v4 REST API,
// shaped from the documented responses the importer consumes. It exists
// for tests and local runs: any request that is not a GET is rejected and
// recorded, so a test can assert the importer never writes to the source.
package fakecoolify

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// Fixture identifiers other packages assert on.
const (
	Token         = "fake-coolify-token" //nolint:gosec // fixture value
	ProjectUUID   = "proj-shop"
	EnvName       = "production"
	PGUUID        = "pgmain1xuuid"
	RedisUUID     = "rdscache1uuid"
	AppWeb        = "app-web-uuid" //nolint:gosec // fixture value
	AppSearxng    = "app-searxng-uuid"
	AppCompose    = "app-collab-uuid"
	AppMemgraph   = "app-memgraph-uuid"
	AppAPI        = "app-api-uuid"
	AppReport     = "app-report-uuid"
	ServiceUUID   = "svc-plausible-uuid"
	WebSecret     = "web-session-secret-value" //nolint:gosec // fixture value
	WebDBPassword = "s3cr3t-db-pass"
)

// Server is the fake. Create it with New and serve Handler.
type Server struct {
	mu       sync.Mutex
	requests []string
	// RedactSensitive makes secret env values come back empty, like a token
	// without the read:sensitive ability.
	RedactSensitive bool
	// SearxngImage replaces the registry image of the searxng app, so a
	// local run can use a small image. Empty keeps the default.
	SearxngImage string
	// SearxngPort replaces the exposed port of the searxng app.
	SearxngPort string
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

func list(items ...obj) []obj {
	if items == nil {
		return []obj{}
	}
	return items
}

func envVar(key, value string, shownOnce bool) obj {
	return obj{"uuid": "env-" + key, "key": key, "value": value, "real_value": value, "is_preview": false, "is_shown_once": shownOnce}
}

func (s *Server) applications() []obj {
	return list(
		obj{"uuid": AppWeb, "name": "web", "fqdn": "https://app.example.com", "build_pack": "dockerfile", "git_repository": "acme/web",
			"git_branch": "main", "dockerfile_location": "/Dockerfile", "base_directory": "/", "ports_exposes": "3000",
			"health_check_enabled": true, "health_check_path": "/healthz", "health_check_type": "http", "health_check_interval": 10,
			"health_check_timeout": 3, "health_check_retries": 4, "limits_memory": "512m", "limits_cpus": "0.5",
			"destination": obj{"server": obj{"name": "hetzner-1"}}},
		obj{"uuid": AppSearxng, "name": "searxng", "fqdn": "https://search.example.com", "build_pack": "dockerimage",
			"docker_registry_image_name": s.searxngImage(), "docker_registry_image_tag": s.searxngTag(), "ports_exposes": s.searxngPort(),
			"health_check_enabled": true, "health_check_path": "/healthz", "health_check_type": "http", "health_check_interval": 5,
			"health_check_timeout": 3, "health_check_retries": 3, "limits_memory": "0", "limits_cpus": "0",
			"destination": obj{"server": obj{"name": "hetzner-1"}}},
		obj{"uuid": AppMemgraph, "name": "memgraph", "fqdn": "", "build_pack": "dockerimage",
			"docker_registry_image_name": "memgraph/memgraph", "docker_registry_image_tag": "2.20.0", "ports_exposes": "7687",
			"limits_memory": "2g", "limits_cpus": "1", "destination": obj{"server": obj{"name": "hetzner-1"}}},
		obj{"uuid": AppAPI, "name": "go-api", "fqdn": "https://api.example.com", "build_pack": "nixpacks",
			"git_repository": "git@github.com:acme/api.git", "git_branch": "main", "ports_exposes": "8080", "private_key_id": 3,
			"destination": obj{"server": obj{"name": "hetzner-1"}}},
		obj{"uuid": AppReport, "name": "report-worker", "fqdn": "", "build_pack": "dockerimage",
			"docker_registry_image_name": "ghcr.io/acme/report", "docker_registry_image_tag": "1.4", "ports_exposes": "9000",
			"destination": obj{"server": obj{"name": "hetzner-1"}}},
		obj{"uuid": AppCompose, "name": "collab-server", "fqdn": "https://collab.example.com", "build_pack": "dockercompose",
			"ports_exposes": "1234", "destination": obj{"server": obj{"name": "hetzner-1"}}},
	)
}

func (s *Server) searxngImage() string {
	if s.SearxngImage == "" {
		return "searxng/searxng"
	}
	name, _, _ := strings.Cut(s.SearxngImage, ":")
	return name
}

func (s *Server) searxngPort() string {
	if s.SearxngPort == "" {
		return "8080"
	}
	return s.SearxngPort
}

func (s *Server) searxngTag() string {
	if _, tag, ok := strings.Cut(s.SearxngImage, ":"); ok {
		return tag
	}
	return "latest"
}

func (s *Server) envs(uuid string) []obj {
	pw := WebDBPassword
	sec := WebSecret
	if s.RedactSensitive {
		pw, sec = "", ""
	}
	switch uuid {
	case AppWeb:
		return list(
			envVar("NODE_ENV", "production", false),
			envVar("NEXT_PUBLIC_URL", "https://app.example.com", false),
			envVar("DATABASE_URL", "postgres://app:"+pw+"@"+PGUUID+":5432/app", false),
			envVar("SESSION_SECRET", sec, true),
		)
	case AppSearxng:
		return list(envVar("SEARXNG_BASE_URL", "https://search.example.com/", false))
	case AppReport:
		return list(envVar("REDIS_URL", "redis://:"+pw+"@"+RedisUUID+":6379/0", false))
	case AppAPI:
		return list(envVar("API_TOKEN", sec, true))
	}
	return list()
}

func (s *Server) storages(uuid string) obj {
	switch uuid {
	case AppWeb:
		return obj{"persistent_storages": list(obj{"name": AppWeb + "-uploads", "mount_path": "/app/uploads", "host_path": nil}), "file_storages": list()}
	case AppMemgraph:
		return obj{"persistent_storages": list(obj{"name": AppMemgraph + "-data", "mount_path": "/var/lib/memgraph", "host_path": nil, "size_bytes": 5 << 30}), "file_storages": list()}
	}
	return obj{"persistent_storages": list(), "file_storages": list()}
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		write(w, list(obj{"uuid": ProjectUUID, "name": "Shop"}))
	})
	mux.HandleFunc("/api/v1/projects/{uuid}/environments", func(w http.ResponseWriter, _ *http.Request) {
		write(w, list(obj{"uuid": "env-prod", "name": EnvName}))
	})
	mux.HandleFunc("/api/v1/projects/{uuid}/{env}", func(w http.ResponseWriter, _ *http.Request) {
		write(w, obj{
			"name": EnvName, "applications": s.applications(),
			"postgresqls": list(obj{"uuid": PGUUID, "name": "main-db", "image": "postgres:16-alpine"}),
			"redis":       list(obj{"uuid": RedisUUID, "name": "cache", "image": "redis:7.2"}),
			"mongodbs":    list(), "mysqls": list(), "mariadbs": list(),
			"services": list(obj{"uuid": ServiceUUID, "name": "plausible", "service_type": "plausible", "destination": obj{"server": obj{"name": "hetzner-1"}}}),
		})
	})
	mux.HandleFunc("/api/v1/applications/{uuid}/envs", func(w http.ResponseWriter, r *http.Request) {
		write(w, s.envs(r.PathValue("uuid")))
	})
	mux.HandleFunc("/api/v1/applications/{uuid}/storages", func(w http.ResponseWriter, r *http.Request) {
		write(w, s.storages(r.PathValue("uuid")))
	})
	mux.HandleFunc("/api/v1/applications/{uuid}/scheduled-tasks", func(w http.ResponseWriter, _ *http.Request) {
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
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
