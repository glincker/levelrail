package docker

import (
	"context"
	"fmt"
	"strings"

	dockerclient "github.com/moby/moby/client"

	dockertypes "github.com/moby/moby/api/types"
)

// ListVolumesByPrefix returns this instance's volumes whose name starts
// with prefix, with on-disk sizes from the daemon's disk usage report
// (SizeBytes is -1 when the driver reports none).
func (c *Client) ListVolumesByPrefix(ctx context.Context, prefix string) ([]NamedVolume, error) {
	du, err := c.cli.DiskUsage(ctx, dockerclient.DiskUsageOptions{Volumes: true})
	if err != nil {
		return nil, fmt.Errorf("docker: volume disk usage: %w", err)
	}
	inUse, err := c.mountedVolumeNames(ctx)
	if err != nil {
		return nil, err
	}
	var out []NamedVolume
	for i := range du.Volumes.Items {
		v := &du.Volumes.Items[i]
		if !strings.HasPrefix(v.Name, prefix) {
			continue
		}
		if c.hasInstanceLabel() && v.Labels[c.instanceLabelKey] != c.instanceLabelValue {
			continue
		}
		out = append(out, NamedVolume{Name: v.Name, SizeBytes: volumeSize(v), CreatedAt: v.CreatedAt, Mounted: inUse[v.Name]})
	}
	return out, nil
}
