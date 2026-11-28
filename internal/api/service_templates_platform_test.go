package api

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/catalog"
	"github.com/glincker/levelrail/internal/store"
)

func TestPlatformPreflightTemplate(t *testing.T) {
	rt := &Router{}
	host := "linux/" + runtime.GOARCH
	if reason, _ := rt.platformPreflight(context.Background(), catalog.Template{ID: "x", Platforms: []string{host}}); reason != "" {
		t.Errorf("matching platform refused: %s", reason)
	}
	if reason, _ := rt.platformPreflight(context.Background(), catalog.Template{ID: "x"}); reason != "" {
		t.Errorf("unchecked platforms refused: %s", reason)
	}
	reason, overridable := rt.platformPreflight(context.Background(), catalog.Template{ID: "x", Platforms: []string{"plan9/mips"}})
	if !overridable || !strings.Contains(reason, "plan9/mips") || !strings.Contains(reason, host) {
		t.Errorf("mismatch reason = %q overridable=%v", reason, overridable)
	}
}

func TestGPUTemplatePreflight(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	ctx := context.Background()
	tpl := catalog.Template{ID: "runner", RequiresGPU: true}
	if got := rt.gpuTemplatePreflight(ctx, catalog.Template{ID: "plain"}); got != "" {
		t.Fatalf("non-GPU template refused: %s", got)
	}
	if got := rt.gpuTemplatePreflight(ctx, tpl); got != "" {
		t.Fatalf("unknown local GPU state should not refuse: %s", got)
	}
	if err := db.SetNodeGPU(ctx, store.LocalNodeGPUKey, gpuInfoN(0, false)); err != nil {
		t.Fatal(err)
	}
	seedGPUNode(t, db, "gpubox", gpuInfoN(1, true))
	got := rt.gpuTemplatePreflight(ctx, tpl)
	if !strings.Contains(got, "device_missing") || !strings.Contains(got, "gpubox") {
		t.Fatalf("want class and free node named, got %q", got)
	}
	if err := db.SetNodeGPU(ctx, store.LocalNodeGPUKey, gpuInfoN(1, true)); err != nil {
		t.Fatal(err)
	}
	if got := rt.gpuTemplatePreflight(ctx, tpl); got != "" {
		t.Fatalf("local free GPU refused: %s", got)
	}
}

type archPinger struct{ arch string }

func (archPinger) Ping(context.Context) error          { return nil }
func (p archPinger) DaemonArch(context.Context) string { return p.arch }

// A darwin control plane runs linux containers, so the daemon arch decides.
func TestPlatformPreflightDaemonPlatform(t *testing.T) {
	tests := []struct {
		name      string
		daemon    string
		platforms []string
		refused   bool
	}{
		{"arm64 daemon, arm64 template", "arm64", []string{"linux/arm64"}, false},
		{"aarch64 daemon, arm64 template", "aarch64", []string{"linux/arm64"}, false},
		{"amd64 daemon, amd64 template", "x86_64", []string{"linux/amd64", "linux/arm64"}, false},
		{"arm64 daemon, amd64 only", "arm64", []string{"linux/amd64"}, true},
		{"darwin never matches", "arm64", []string{"darwin/arm64"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{dockerPinger: archPinger{arch: tt.daemon}}
			reason, _ := rt.platformPreflight(context.Background(), catalog.Template{ID: "x", Platforms: tt.platforms})
			if (reason != "") != tt.refused {
				t.Errorf("reason = %q, refused want %v", reason, tt.refused)
			}
		})
	}
}
