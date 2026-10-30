package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckVolumeOwner(t *testing.T) {
	want := VolumeOwner{App: "web"}
	if err := checkVolumeOwner("v", map[string]string{VolumeOwnerAppLabel: "other"}, want); !errors.Is(err, ErrVolumeOwnerMismatch) {
		t.Errorf("different owner: err = %v, want mismatch", err)
	}
	if err := checkVolumeOwner("v", map[string]string{VolumeOwnerAppLabel: "web"}, want); err != nil {
		t.Errorf("same owner: err = %v", err)
	}
	if err := checkVolumeOwner("v", nil, want); err != nil {
		t.Errorf("unlabeled: err = %v, want accepted", err)
	}
	if err := checkVolumeOwner("v", map[string]string{VolumeOwnerAppLabel: "other"}, VolumeOwner{}); err != nil {
		t.Errorf("unknown owner: err = %v, want accepted", err)
	}
}

func TestEnsureOwnedVolumeUnlabelledUsers(t *testing.T) {
	tests := []struct {
		name       string
		containers string
		wantErr    bool
	}{
		{name: "used by another app is refused", containers: `[{"Id":"a","Labels":{"platform-reserved.app":"other"}}]`, wantErr: true},
		{name: "unused is accepted", containers: `[]`},
		{name: "used by the same app is accepted", containers: `[{"Id":"a","Labels":{"platform-reserved.app":"web"}}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/_ping"):
					w.WriteHeader(http.StatusOK)
				case strings.HasSuffix(r.URL.Path, "/volumes/create"):
					_, _ = w.Write([]byte(`{"Name":"legacy","Labels":null}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					if !strings.Contains(r.URL.Query().Get("filters"), "legacy") || r.URL.Query().Get("all") == "" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = w.Write([]byte(tt.containers))
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
			defer func() { _ = c.Close() }()

			err = c.EnsureOwnedVolume(context.Background(), "legacy", VolumeOwner{App: "web"})
			if tt.wantErr != errors.Is(err, ErrVolumeOwnerMismatch) || (!tt.wantErr && err != nil) {
				t.Errorf("EnsureOwnedVolume() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
