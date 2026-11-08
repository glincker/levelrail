package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNextImageCacheMountOnlyWhenEnabled(t *testing.T) {
	detected := build.NextProfile{Detected: true}
	on := Request{Service: spec.Service{Next: &spec.Next{ImageCache: spec.NextImageCachePersistent}}}
	off := Request{Service: spec.Service{Next: &spec.Next{ImageCache: spec.NextImageCacheNone}}}
	if got := nextImageCacheTarget(off, detected, false); got != "" {
		t.Fatalf("none mounted %q", got)
	}
	if got := nextImageCacheTarget(on, build.NextProfile{}, true); got != "" {
		t.Fatalf("non-next app mounted %q", got)
	}
	mount := nextImageCacheTarget(on, detected, false)
	if mount != "/app/.next/cache/images" {
		t.Fatalf("mount = %q", mount)
	}

	var d store.DesiredService
	applyNextImageCache(&d, "web", "")
	if len(d.Volumes) != 0 {
		t.Fatalf("empty path added %v", d.Volumes)
	}
	applyNextImageCache(&d, "web", mount)
	applyNextImageCache(&d, "web", mount)
	if len(d.Volumes) != 1 || d.Volumes[0].Name != "app-web-next-images" || d.Volumes[0].ContainerPath != mount {
		t.Fatalf("volumes = %+v", d.Volumes)
	}
}

func TestNextImageCachePathStandaloneLayout(t *testing.T) {
	on := Request{Service: spec.Service{Next: &spec.Next{ImageCache: spec.NextImageCachePersistent}}}
	detected := build.NextProfile{Detected: true}
	if got := nextImageCacheTarget(on, detected, true); got != "/app/.next/standalone/.next/cache/images" {
		t.Fatalf("standalone mount = %q", got)
	}
	if got := nextImageCacheTarget(on, detected, false); got != "/app/.next/cache/images" {
		t.Fatalf("plain mount = %q", got)
	}
}

func TestNextRunsStandaloneMatchesPlan(t *testing.T) {
	dir := t.TempDir()
	if build.NextRunsStandalone(dir) {
		t.Fatal("empty dir reported standalone")
	}
	if err := os.WriteFile(filepath.Join(dir, "next.config.js"), []byte("module.exports = { output: 'standalone' }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !build.NextRunsStandalone(dir) {
		t.Fatal("standalone config not detected")
	}
}
