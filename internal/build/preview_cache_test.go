package build

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewCacheID(t *testing.T) {
	long := strings.Repeat("feature-branch-name-", 4)
	cases := []struct {
		name, mode, svc, unique, want string
	}{
		{"isolated keeps the pr", PreviewCacheIsolated, "web-pr-12", "", "web-pr-12"},
		{"shared strips the pr", PreviewCacheSharedWithApp, "web-pr-12", "", "web"},
		{"shared strips multi service key", PreviewCacheSharedWithApp, "web-pr-12-api", "", "web"},
		{"special characters", PreviewCacheIsolated, "My_App.v2-pr-3", "", "my-app-v2-pr-3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreviewCacheID(tc.mode, tc.svc, tc.unique); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	a := PreviewCacheID(PreviewCacheIsolated, long+"-pr-1", "")
	b := PreviewCacheID(PreviewCacheIsolated, long+"-pr-2", "")
	if a == b || len(a) > maxCacheIDLen || len(b) > maxCacheIDLen {
		t.Fatalf("long names must differ and fit: %q %q", a, b)
	}
	o1 := PreviewCacheID(PreviewCacheOff, "web-pr-1", "t1")
	o2 := PreviewCacheID(PreviewCacheOff, "web-pr-1", "t2")
	if o1 == o2 || !strings.HasPrefix(o1, "web-pr-1-") {
		t.Fatalf("off mode must be unique per build: %q %q", o1, o2)
	}
	if got := PreviewCacheID(PreviewCacheOff, long+"-pr-1", "t"); len(got) > maxCacheIDLen {
		t.Fatalf("off id too long: %q", got)
	}
}

func TestPreviewCacheModeDefaults(t *testing.T) {
	t.Setenv(EnvPreviewCacheMode, "")
	if got := PreviewCacheMode(context.Background()); got != PreviewCacheIsolated {
		t.Fatalf("default = %q", got)
	}
	t.Setenv(EnvPreviewCacheMode, "OFF")
	if got := PreviewCacheMode(context.Background()); got != PreviewCacheOff {
		t.Fatalf("env = %q", got)
	}
	SetPreviewCacheModeResolver(func(context.Context) string { return "bogus" })
	defer SetPreviewCacheModeResolver(nil)
	if got := PreviewCacheMode(context.Background()); got != PreviewCacheIsolated {
		t.Fatalf("bogus = %q", got)
	}
}
