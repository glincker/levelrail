package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dockernetwork "github.com/docker/docker/api/types/network"
)

// jsonError writes a Docker Engine API style error body, the shape
// request.go's checkResponseErr needs to surface the message verbatim
// through err.Error(), so a fake server can exercise the exact
// strings.Contains fallbacks client.go's NetworkConnect/NetworkDisconnect
// rely on without a real daemon.
func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func jsonOK(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// fakeDockerClient spins up an httptest server standing in for the
// Docker daemon (the same DOCKER_HOST-swap pattern update_resources_test.go
// already uses), so the real Client methods run their real HTTP-calling
// code without needing an actual daemon.
func fakeDockerClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return c
}

func TestClient_NetworkConnect(t *testing.T) {
	tests := []struct {
		name        string
		inspect     func(w http.ResponseWriter)
		connect     func(w http.ResponseWriter)
		wantErr     string
		wantConnect bool
	}{
		{
			name: "not yet connected, connects",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			connect:     func(w http.ResponseWriter) { jsonOK(w, nil) },
			wantConnect: true,
		},
		{
			name: "already connected by container id",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{
					"db-main": {Name: "db-main"},
				}})
			},
			wantConnect: false,
		},
		{
			name: "already connected by name, keyed by a different container id",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{
					"abc123": {Name: "db-main"},
				}})
			},
			wantConnect: false,
		},
		{
			name: "connect conflict treated as already connected",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			connect:     func(w http.ResponseWriter) { jsonError(w, http.StatusConflict, "endpoint already exists") },
			wantConnect: true,
		},
		{
			name: "connect already-exists wording with a non-conflict status still tolerated",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			connect: func(w http.ResponseWriter) {
				jsonError(w, http.StatusForbidden, "endpoint with name db-main already exists in network levelrail-app-web")
			},
			wantConnect: true,
		},
		{
			name: "inspect error propagates",
			inspect: func(w http.ResponseWriter) {
				jsonError(w, http.StatusInternalServerError, "daemon unreachable")
			},
			wantErr: "daemon unreachable",
		},
		{
			name: "connect real error propagates",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			connect:     func(w http.ResponseWriter) { jsonError(w, http.StatusInternalServerError, "disk full") },
			wantConnect: true,
			wantErr:     "disk full",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			connected := false
			c := fakeDockerClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/networks/"):
					tc.inspect(w)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/connect"):
					connected = true
					if tc.connect == nil {
						t.Fatal("unexpected connect call")
						return
					}
					tc.connect(w)
				default:
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			err := c.NetworkConnect(context.Background(), "levelrail-app-web", "db-main")

			if connected != tc.wantConnect {
				t.Errorf("connect called = %v, want %v", connected, tc.wantConnect)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NetworkConnect() error = %v", err)
			}
		})
	}
}

func TestClient_NetworkDisconnect(t *testing.T) {
	tests := []struct {
		name       string
		disconnect func(w http.ResponseWriter)
		wantErr    string
	}{
		{
			name:       "success",
			disconnect: func(w http.ResponseWriter) { jsonOK(w, nil) },
		},
		{
			name: "not connected tolerated",
			disconnect: func(w http.ResponseWriter) {
				jsonError(w, http.StatusForbidden, "container db-main is not connected to network levelrail-app-old")
			},
		},
		{
			name: "not found tolerated",
			disconnect: func(w http.ResponseWriter) {
				jsonError(w, http.StatusNotFound, "network levelrail-app-old not found")
			},
		},
		{
			name:       "other error propagates",
			disconnect: func(w http.ResponseWriter) { jsonError(w, http.StatusInternalServerError, "daemon unreachable") },
			wantErr:    "daemon unreachable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeDockerClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/disconnect") {
					tc.disconnect(w)
					return
				}
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			})

			err := c.NetworkDisconnect(context.Background(), "levelrail-app-old", "db-main", true)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NetworkDisconnect() error = %v", err)
			}
		})
	}
}

func TestClient_RemoveNetwork(t *testing.T) {
	tests := []struct {
		name            string
		inspect         func(w http.ResponseWriter)
		remove          func(w http.ResponseWriter)
		disconnect      func(w http.ResponseWriter)
		wantErr         string
		wantDisconnects int
	}{
		{
			name: "no containers attached, removed directly",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			remove: func(w http.ResponseWriter) { jsonOK(w, nil) },
		},
		{
			name: "disconnects attached containers before removing",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{
					"c1": {Name: "db-main"},
					"c2": {Name: "web-1"},
				}})
			},
			disconnect:      func(w http.ResponseWriter) { jsonOK(w, nil) },
			remove:          func(w http.ResponseWriter) { jsonOK(w, nil) },
			wantDisconnects: 2,
		},
		{
			name: "network already gone, remove still attempted",
			inspect: func(w http.ResponseWriter) {
				jsonError(w, http.StatusNotFound, "network levelrail-app-old not found")
			},
			remove: func(w http.ResponseWriter) { jsonOK(w, nil) },
		},
		{
			name: "remove not found tolerated",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			remove: func(w http.ResponseWriter) { jsonError(w, http.StatusNotFound, "network not found") },
		},
		{
			name: "disconnect error during removal propagates",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{
					"c1": {Name: "db-main"},
				}})
			},
			disconnect:      func(w http.ResponseWriter) { jsonError(w, http.StatusInternalServerError, "disk full") },
			wantDisconnects: 1,
			wantErr:         "disk full",
		},
		{
			name: "remove real error propagates",
			inspect: func(w http.ResponseWriter) {
				jsonOK(w, dockernetwork.Inspect{Containers: map[string]dockernetwork.EndpointResource{}})
			},
			remove:  func(w http.ResponseWriter) { jsonError(w, http.StatusInternalServerError, "daemon unreachable") },
			wantErr: "daemon unreachable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disconnects := 0
			c := fakeDockerClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/networks/"):
					tc.inspect(w)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/disconnect"):
					disconnects++
					if tc.disconnect == nil {
						t.Fatal("unexpected disconnect call")
						return
					}
					tc.disconnect(w)
				case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/networks/"):
					if tc.remove == nil {
						t.Fatal("unexpected remove call")
						return
					}
					tc.remove(w)
				default:
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			err := c.RemoveNetwork(context.Background(), "levelrail-app-old")

			if disconnects != tc.wantDisconnects {
				t.Errorf("disconnect calls = %d, want %d", disconnects, tc.wantDisconnects)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RemoveNetwork() error = %v", err)
			}
		})
	}
}

func TestAlreadyConnected(t *testing.T) {
	containers := map[string]dockernetwork.EndpointResource{
		"c1": {Name: "db-main"},
	}

	tests := []struct {
		name       string
		containers map[string]dockernetwork.EndpointResource
		id         string
		want       bool
	}{
		{name: "matches by map key", containers: containers, id: "c1", want: true},
		{name: "matches by endpoint name", containers: containers, id: "db-main", want: true},
		{name: "no match", containers: containers, id: "web-1", want: false},
		{name: "empty map never matches", containers: map[string]dockernetwork.EndpointResource{}, id: "c1", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := alreadyConnected(tc.containers, tc.id); got != tc.want {
				t.Errorf("alreadyConnected(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}
