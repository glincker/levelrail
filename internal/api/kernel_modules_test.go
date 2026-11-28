package api

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/catalog"
	"github.com/glincker/levelrail/internal/failure"
)

func TestKernelModulePreflight(t *testing.T) {
	orig := kernelModuleLoaded
	t.Cleanup(func() { kernelModuleLoaded = orig })
	tpl := catalog.Template{ID: "wg-easy", Manifest: &catalog.Manifest{RequiresKernelModules: []string{"wireguard"}}}

	kernelModuleLoaded = func(string) (bool, bool) { return false, true }
	msg := kernelModulePreflight(tpl)
	if !strings.Contains(msg, "modprobe wireguard") {
		t.Fatalf("missing module must give the fix, got %q", msg)
	}
	if c, ok := failure.ClassifyCompat(msg); !ok || c.ID != failure.CompatKernelModuleMissing {
		t.Fatalf("refusal must classify as kernel_module_missing, got %v %v", c.ID, ok)
	}
	kernelModuleLoaded = func(string) (bool, bool) { return true, true }
	if msg := kernelModulePreflight(tpl); msg != "" {
		t.Fatalf("loaded module must pass, got %q", msg)
	}
	kernelModuleLoaded = func(string) (bool, bool) { return false, false }
	if msg := kernelModulePreflight(tpl); msg != "" {
		t.Fatalf("unknown host must skip, got %q", msg)
	}
}
