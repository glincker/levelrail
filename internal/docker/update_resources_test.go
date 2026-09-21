package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// TestUpdateResources_MemorySwapDefaultsWhenUnset proves the actual
// ContainerUpdate call site (not just updateMemorySwap in isolation)
// sends the defaulted MemorySwap value: a memory-only update must not
// regress back to sending 0, which is the exact silent-failure bug this
// was fixed for.
func TestUpdateResources_MemorySwapDefaultsWhenUnset(t *testing.T) {
	var gotUpdate container.UpdateConfig
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&gotUpdate)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Warnings":[]}`))
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

	err = c.UpdateResources(context.Background(), "container-id", Resources{MemoryBytes: 512 << 20})
	if err != nil {
		t.Fatalf("UpdateResources() error = %v", err)
	}
	if gotUpdate.Memory != 512<<20 {
		t.Errorf("sent Memory = %d, want %d", gotUpdate.Memory, 512<<20)
	}
	if gotUpdate.MemorySwap != 1024<<20 {
		t.Errorf("sent MemorySwap = %d, want %d (2x memory, the ContainerCreate-matching default), got the old silently-failing 0 if this regresses", gotUpdate.MemorySwap, 1024<<20)
	}
}
