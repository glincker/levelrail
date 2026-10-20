package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewClient_NegotiatesAPIVersion proves NewClient adapts to whatever
// API version a connected daemon actually reports rather than assuming a
// fixed one: a fake daemon advertises an unusual version on /_ping, and
// the first real request must be sent against that negotiated version,
// not the client library's own compiled default.
//
// This is the exact failure mode CapRover hit (a hardcoded API version
// constant that breaks the whole dashboard whenever the host's Docker
// daemon is upgraded past it): NewClient already calls
// dockerclient.WithAPIVersionNegotiation(), so this test is a regression
// guard against that ever being lost, not a fix for a bug found here.
func TestNewClient_NegotiatesAPIVersion(t *testing.T) {
	tests := []struct {
		name          string
		daemonVersion string
		wantPrefix    string
	}{
		{
			name:          "older daemon, client downgrades",
			daemonVersion: "1.41",
			wantPrefix:    "/v1.41/",
		},
		{
			name:          "newer daemon, client caps at its own max",
			daemonVersion: "1.90",
			wantPrefix:    "/v1.51/", // api.DefaultVersion for docker/docker v28.5.2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/_ping") {
					w.Header().Set("Api-Version", tt.daemonVersion)
					w.WriteHeader(http.StatusOK)
					return
				}
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("[]"))
			}))
			defer srv.Close()

			t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))

			c, err := NewClient()
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			defer func() {
				if err := c.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}()

			if _, err := c.ListByPrefix(context.Background(), "test"); err != nil {
				t.Fatalf("ListByPrefix() error = %v", err)
			}

			if !strings.HasPrefix(gotPath, tt.wantPrefix) {
				t.Errorf("request path = %q, want prefix %q (daemon advertised %q)", gotPath, tt.wantPrefix, tt.daemonVersion)
			}
		})
	}
}
