package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	dockerclient "github.com/moby/moby/client"

	dockertypes "github.com/moby/moby/api/types"
)

// InventoryVolume is one Docker volume of any name, unlike NamedVolume which
// only keeps this project's own app-/db- names.
type InventoryVolume struct {
	Name      string
	SizeBytes int64 // -1 when the driver does not report a size
	CreatedAt string
	InUse     bool
	Labels    map[string]string
}

// InventoryContainer is one stopped container with its writable-layer size.
type InventoryContainer struct {
	ID        string
	Name      string
	SizeBytes int64 // -1 when Docker did not report a size
	Labels    map[string]string
	ExitedAt  time.Time // zero when unknown
}

// ListAllVolumes returns every volume on the daemon with its size and
// whether any container mounts it. A failed size query leaves sizes unknown.
func (c *Client) ListAllVolumes(ctx context.Context) ([]InventoryVolume, error) {
	vols, err := c.cli.VolumeList(ctx, dockerclient.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: list volumes: %w", err)
	}
	inUse, err := c.mountedVolumeNames(ctx)
	if err != nil {
		return nil, err
	}
	sizes := map[string]int64{}
	if du, duErr := c.cli.DiskUsage(ctx, dockerclient.DiskUsageOptions{Volumes: true}); duErr == nil {
		for i := range du.Volumes.Items {
			v := &du.Volumes.Items[i]
			sizes[v.Name] = volumeSize(v)
		}
	}
	out := make([]InventoryVolume, 0, len(vols.Volumes))
	for _, v := range vols.Volumes {
		if v == nil {
			continue
		}
		out = append(out, InventoryVolume{
			Name: v.Name, SizeBytes: sizeOr(sizes, v), CreatedAt: v.CreatedAt,
			InUse: inUse[v.Name], Labels: v.Labels,
		})
	}
	return out, nil
}

// ListStoppedContainers returns exited and created containers of this
// instance with their size and, from an inspect, when they stopped.
func (c *Client) ListStoppedContainers(ctx context.Context) ([]InventoryContainer, error) {
	f := dockerclient.Filters{}
	f.Add("status", "exited")
	f.Add("status", "created")
	c.instanceLabelFilter(f)
	summaries, err := c.cli.ContainerList(ctx, dockerclient.ContainerListOptions{All: true, Size: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list stopped containers: %w", err)
	}
	out := make([]InventoryContainer, 0, len(summaries))
	for _, cs := range summaries {
		ic := InventoryContainer{ID: cs.ID, Name: strings.TrimPrefix(containerSummaryName(cs), "/"), SizeBytes: cs.SizeRw, Labels: cs.Labels}
		if resp, ierr := c.cli.ContainerInspect(ctx, cs.ID); ierr == nil && resp.State != nil {
			if t, perr := time.Parse(time.RFC3339Nano, resp.State.FinishedAt); perr == nil && t.Year() > 1 {
				ic.ExitedAt = t
			}
		}
		out = append(out, ic)
	}
	return out, nil
}
