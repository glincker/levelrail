package build

import (
	"context"
	"errors"
	"testing"

	dockerclient "github.com/docker/docker/client"
)

// TestNewClient_CacheValidation exercises NewClient's cache-option
// validation path without needing a live, reachable BuildKit
// connection: bkclient.New (client.go) dials lazily, so a
// *dockerclient.Client pointed at a host that is never actually
// contacted is enough to reach the cache.validate() check below it.
// This is what actually exercises WithCacheRegistry and
// WithCacheRegistryInsecure as pure option-setting logic, distinct from
// the live build tests in build_test.go that also happen to pass them.
func TestNewClient_CacheValidation(t *testing.T) {
	docker, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("tcp://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("new docker client: %v", err)
	}
	t.Cleanup(func() { _ = docker.Close() })

	tests := []struct {
		name    string
		opts    []Option
		wantErr error
	}{
		{
			name: "no cache options: valid",
		},
		{
			name: "registry ref only: valid",
			opts: []Option{WithCacheRegistry("example.com/cache:tag")},
		},
		{
			name: "registry ref with insecure: valid",
			opts: []Option{WithCacheRegistry("example.com/cache:tag"), WithCacheRegistryInsecure()},
		},
		{
			name:    "insecure without a ref: rejected",
			opts:    []Option{WithCacheRegistryInsecure()},
			wantErr: ErrCacheRegistryRefRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(context.Background(), docker, tt.opts...)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewClient() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient() unexpected error: %v", err)
			}
			t.Cleanup(func() { _ = c.Close() })
		})
	}
}

func TestNewClient_NilDocker(t *testing.T) {
	_, err := NewClient(context.Background(), nil)
	if err == nil {
		t.Fatal("NewClient(nil docker) error = nil, want a non-nil error")
	}
}
