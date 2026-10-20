package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
)

func buildScratchImage(ctx context.Context, t *testing.T, c *Client, tag string) string {
	t.Helper()
	files := map[string]string{"Dockerfile": "FROM scratch\nCOPY marker /marker\n", "marker": tag}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
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
	return img.ID
}

// The save, remove, load round trip an image move does, against one daemon.
func TestClient_LoadImage_Live_RoundTripKeepsImageID(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()
	tag := "lr-imagemove-live:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	wantID := buildScratchImage(ctx, t, c, tag)
	t.Cleanup(func() { _, _ = c.cli.ImageRemove(context.Background(), wantID, image.RemoveOptions{Force: true}) })

	rc, err := c.cli.ImageSave(ctx, []string{tag})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	archive, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("read save stream: %v", err)
	}
	if _, err := c.cli.ImageRemove(ctx, wantID, image.RemoveOptions{Force: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if id, _ := c.InspectImageID(ctx, tag); id != "" {
		t.Fatalf("image still present after remove: %s", id)
	}

	if err := c.LoadImage(ctx, bytes.NewReader(archive)); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, err := c.InspectImageID(ctx, tag)
	if err != nil || got != wantID {
		t.Fatalf("loaded ID = %q, %v, want %q", got, err, wantID)
	}

	if layers, err := c.InspectImageLayers(ctx, tag); err != nil || len(layers) == 0 {
		t.Fatalf("loaded layers = %v, %v", layers, err)
	}

	if err := c.LoadImage(ctx, strings.NewReader("not a tar archive")); err == nil {
		t.Fatal("loading garbage succeeded")
	}
}
