package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
)

func buildMarkerImage(ctx context.Context, t *testing.T, c *Client, tag, marker string) string {
	t.Helper()
	dockerfile := "FROM nginx:alpine\nRUN echo " + marker + " > /marker\n"
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "Dockerfile", Mode: 0o600, Size: int64(len(dockerfile))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := c.cli.ImageBuild(ctx, &buf, build.ImageBuildOptions{Tags: []string{tag}, Remove: true, ForceRemove: true})
	if err != nil {
		t.Fatalf("build %s: %v", tag, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	img, err := c.cli.ImageInspect(ctx, tag)
	if err != nil {
		t.Fatalf("inspect built %s: %v", tag, err)
	}
	t.Cleanup(func() { _, _ = c.cli.ImageRemove(context.Background(), img.ID, image.RemoveOptions{Force: true}) })
	return img.ID
}

// TestClient_Prune_Live_KeepsPinnedAndInUseImages is the disk-pressure
// scenario: a prune must never take a pinned rollback tag, nor an image a
// running container still uses even after its tag moved on, but must free a
// truly orphaned dangling image. Run it against an isolated daemon, a prune
// is daemon-wide.
func TestClient_Prune_Live_KeepsPinnedAndInUseImages(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	c := liveClient(t)
	ctx := context.Background()
	const name = "levelrail-test-prune-img-running"
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(ctx, t, c, name) })

	pinnedID := buildMarkerImage(ctx, t, c, "levelrail-test-prune/app:rollback-pin", "pinned")
	inUseID := buildMarkerImage(ctx, t, c, "levelrail-test-prune/app:live", "inuse")
	id, err := c.Create(ctx, ContainerSpec{Name: name, Image: "levelrail-test-prune/app:live"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	// The :live tag moves to new content, leaving the running container on an untagged image.
	newID := buildMarkerImage(ctx, t, c, "levelrail-test-prune/app:live", "newer")
	orphanID := buildMarkerImage(ctx, t, c, "levelrail-test-prune/orphan:x", "orphan")
	if _, err := c.cli.ImageRemove(ctx, "levelrail-test-prune/orphan:x", image.RemoveOptions{}); err != nil {
		t.Fatalf("untag orphan image: %v", err)
	}
	_ = newID

	result := c.Prune(ctx, []string{name})
	if len(result.Errors) != 0 {
		t.Fatalf("Prune() stage errors: %v", result.Errors)
	}

	for label, imgID := range map[string]string{"pinned rollback tag": pinnedID, "image used by a running container": inUseID} {
		if _, err := c.cli.ImageInspect(ctx, imgID); err != nil {
			t.Errorf("%s was removed by Prune: %v", label, err)
		}
	}
	if _, err := c.cli.ImageInspect(ctx, orphanID); err == nil {
		t.Error("unreferenced dangling image survived Prune under disk pressure, want it freed")
	}
	if state, err := c.InspectByName(ctx, name); err != nil || state == nil || !state.Running {
		t.Errorf("running container disturbed by Prune: state=%+v err=%v", state, err)
	}
}
