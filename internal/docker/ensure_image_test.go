package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEnsureImage_ForcePull proves forcePull actually skips the
// local-presence check (ImageInspect) rather than merely accepting the
// flag and ignoring it: without this, a mutable tag like ":latest"
// would never be re-pulled once present locally, defeating the whole
// point of pull_policy: always.
func TestEnsureImage_ForcePull(t *testing.T) {
	tests := []struct {
		name           string
		forcePull      bool
		wantInspectHit bool
		wantPullHit    bool
	}{
		{name: "forcePull skips inspect, always pulls", forcePull: true, wantInspectHit: false, wantPullHit: true},
		{name: "no forcePull inspects first, image already present, no pull", forcePull: false, wantInspectHit: true, wantPullHit: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inspectHit, pullHit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/_ping"):
					w.WriteHeader(http.StatusOK)
				case strings.HasSuffix(r.URL.Path, "/images/create"):
					pullHit = true
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"done"}` + "\n"))
				case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
					inspectHit = true
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"Id":"sha256:abc"}`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
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

			if err := c.ensureImage(context.Background(), "img:tag", nil, tt.forcePull); err != nil {
				t.Fatalf("ensureImage() error = %v", err)
			}
			if inspectHit != tt.wantInspectHit {
				t.Errorf("ImageInspect called = %v, want %v", inspectHit, tt.wantInspectHit)
			}
			if pullHit != tt.wantPullHit {
				t.Errorf("ImagePull called = %v, want %v", pullHit, tt.wantPullHit)
			}
		})
	}
}
