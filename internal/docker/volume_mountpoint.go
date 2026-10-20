package docker

import (
	"context"
	"fmt"
)

// VolumeMountpoint returns the host directory that backs a local volume,
// creating the volume first when it does not exist.
func (c *Client) VolumeMountpoint(ctx context.Context, name string) (string, error) {
	if err := c.EnsureVolume(ctx, name); err != nil {
		return "", err
	}
	v, err := c.cli.VolumeInspect(ctx, name)
	if err != nil {
		return "", fmt.Errorf("docker: inspect volume %q: %w", name, err)
	}
	if v.Mountpoint == "" {
		return "", fmt.Errorf("docker: volume %q has no host mountpoint", name)
	}
	return v.Mountpoint, nil
}
