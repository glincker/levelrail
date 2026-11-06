package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestBuildCacheSettings_EnvSeedsAndResolverFollows(t *testing.T) {
	_, db := newTestRouter(t)
	ctx := t.Context()
	cases := []struct {
		name    string
		env     map[string]string
		perApp  bool
		minFree int
	}{
		{"defaults", map[string]string{}, true, 2048},
		{"cache off", map[string]string{build.EnvBuildCachePerApp: "false"}, false, 2048},
		{"threshold seeded", map[string]string{build.EnvBuildNextMinFreeMemoryMiB: "4096"}, true, 4096},
		{"warning off", map[string]string{build.EnvBuildNextMinFreeMemoryMiB: "0"}, true, 0},
	}
	t.Cleanup(func() { build.SetNextMinFreeMemoryResolver(nil); build.SetCachePerAppResolver(nil) })
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := runtimesettings.New(db, func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok })
			wireBuildCacheSettings(svc)
			if got := svc.Bool(ctx, build.KeyBuildCachePerApp); got != tc.perApp {
				t.Fatalf("per app = %v, want %v", got, tc.perApp)
			}
			if got := svc.Int(ctx, build.KeyBuildNextMinFreeMemoryMiB); got != tc.minFree {
				t.Fatalf("min free = %d, want %d", got, tc.minFree)
			}
		})
	}
}
