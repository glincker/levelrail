package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRailpackCacheIDPreview(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, mode string
		preview    bool
		want       string
	}{
		{"production unchanged", build.PreviewCacheSharedWithApp, false, "web-pr-7"},
		{"preview isolated", build.PreviewCacheIsolated, true, "web-pr-7"},
		{"preview shared", build.PreviewCacheSharedWithApp, true, "web"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(build.EnvPreviewCacheMode, tc.mode)
			got := railpackCacheID(ctx, Request{ServiceName: "web-pr-7", Preview: tc.preview}, "web-pr-7:abc")
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyNextCachePrefix(t *testing.T) {
	redis := &spec.Next{Cache: "redis"}
	cases := []struct {
		name    string
		preview bool
		next    *spec.Next
		env     map[string]spec.EnvVar
		want    string
	}{
		{"preview gets service name", true, redis, nil, "web-pr-7"},
		{"production untouched", false, redis, nil, ""},
		{"user value wins", true, redis, map[string]spec.EnvVar{nextCachePrefixEnvVar: {}}, ""},
		{"non redis untouched", true, nil, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desired := &store.DesiredService{}
			req := Request{ServiceName: "web-pr-7", Preview: tc.preview, Service: spec.Service{Next: tc.next, Env: tc.env}}
			applyNextCache(desired, req, nil)
			if got := desired.Env[nextCachePrefixEnvVar]; got != tc.want {
				t.Fatalf("prefix = %q, want %q", got, tc.want)
			}
		})
	}
}
