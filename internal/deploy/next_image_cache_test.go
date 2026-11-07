package deploy

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNextImageCacheMountOnlyWhenEnabled(t *testing.T) {
	detected := build.NextProfile{Detected: true}
	on := Request{Service: spec.Service{Next: &spec.Next{ImageCache: spec.NextImageCachePersistent}}}
	off := Request{Service: spec.Service{Next: &spec.Next{ImageCache: spec.NextImageCacheNone}}}
	if got := nextImageCacheTarget(off, detected); got != "" {
		t.Fatalf("none mounted %q", got)
	}
	if got := nextImageCacheTarget(on, build.NextProfile{}); got != "" {
		t.Fatalf("non-next app mounted %q", got)
	}
	mount := nextImageCacheTarget(on, detected)
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
