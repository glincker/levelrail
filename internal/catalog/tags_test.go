package catalog

import (
	"reflect"
	"testing"
)

func TestNewerTags(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		candidates []string
		limit      int
		want       []string
	}{
		{"semver", "2.10.2", []string{"2.10.1", "2.10.2", "2.10.3", "2.11.0", "3.0.0", "latest"}, 5, []string{"3.0.0", "2.11.0", "2.10.3"}},
		{"v prefix keeps shape", "v1.2.3", []string{"v1.2.4", "1.9.0", "v2.0.0-rc1"}, 5, []string{"v1.2.4"}},
		{"suffix must match", "16-alpine", []string{"17-alpine", "17", "18-alpine", "17-bookworm"}, 5, []string{"18-alpine", "17-alpine"}},
		{"limit applies", "1.0", []string{"1.1", "1.2", "1.3"}, 2, []string{"1.3", "1.2"}},
		{"floating tag has no version", "latest", []string{"1.0"}, 5, nil},
		{"shorter version compares as padded", "7", []string{"7.4", "8", "6"}, 5, []string{"8", "7.4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewerTags(tt.current, tt.candidates, tt.limit); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewerTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		ref  string
		want ImageRef
	}{
		{"postgres:16-alpine", ImageRef{Registry: "docker.io", Repository: "library/postgres", Tag: "16-alpine"}},
		{"grafana/grafana-oss", ImageRef{Registry: "docker.io", Repository: "grafana/grafana-oss", Tag: ""}},
		{"ghcr.io/miniflux/miniflux:2.2.1", ImageRef{Registry: "ghcr.io", Repository: "miniflux/miniflux", Tag: "2.2.1"}},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := ParseImageRef(tt.ref)
			if err != nil {
				t.Fatalf("ParseImageRef() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseImageRef() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIsFloatingTag(t *testing.T) {
	for tag, want := range map[string]bool{"latest": true, "": true, "stable": true, "16-alpine": false, "2.7.3": false} {
		if got := IsFloatingTag(tag); got != want {
			t.Errorf("IsFloatingTag(%q) = %v, want %v", tag, got, want)
		}
	}
}
