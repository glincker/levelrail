package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
)

func TestRemoveNextImageCacheVolume(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	vols := newFakeVolumeManager(deploy.NextImageCacheVolume("web"), "app-web-data")
	rt.orphanedVolumes = vols
	rt.removeNextImageCacheVolume(context.Background(), "web", "")
	if len(vols.removed) != 1 || vols.removed[0] != "app-web-next-images" {
		t.Fatalf("removed = %v", vols.removed)
	}
	rt.removeNextImageCacheVolume(context.Background(), "web", "remote-node")
	if len(vols.removed) != 1 {
		t.Fatalf("remote node volume touched: %v", vols.removed)
	}
}
