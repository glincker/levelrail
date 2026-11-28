package docker

import (
	"context"
	"fmt"

	dockerclient "github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// ReadVolumeFile reads one small file from a named volume through a created,
// never started helper, so the daemon reads it regardless of file owner or mode.
func (c *Client) ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error) {
	if err := c.ensureImage(ctx, chownHelperImage, nil, false, nil); err != nil {
		return nil, err
	}
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{Image: chownHelperImage, Labels: c.withInstanceLabel(nil)},
		&container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: volumeName, Target: "/v", ReadOnly: true}}},
		nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("create reader for volume %q: %w", volumeName, err)
	}
	defer func() {
		_ = c.cli.ContainerRemove(context.Background(), resp.ID, dockerclient.ContainerRemoveOptions{Force: true})
	}()
	return c.readContainerFile(ctx, resp.ID, "/v/"+relPath)
}
