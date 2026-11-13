package docker

import (
	"context"
	"fmt"
)

// VolumeAccess is what a write check found for one mounted volume.
type VolumeAccess struct {
	Volume, Path string
	// User is the container's configured user ("nextjs", "1001:1001"); empty means root.
	User               string
	UID, GID           int
	OwnerUID, OwnerGID int
	Mode               int64
	Writable           bool
}

// VolumeAccessProber is an optional Runtime capability (type-asserted, like
// ImageDetailInspector): a write check of a container's volume roots without exec.
type VolumeAccessProber interface {
	ProbeVolumeAccess(ctx context.Context, containerID string, volumes []VolumeMount) ([]VolumeAccess, error)
}

// ProbeVolumeAccess implements VolumeAccessProber. It reads the container's
// effective user, resolves it from the image's /etc/passwd and compares it with
// each writable mount's root owner and mode, all through the archive API.
func (c *Client) ProbeVolumeAccess(ctx context.Context, containerID string, volumes []VolumeMount) ([]VolumeAccess, error) {
	if len(volumes) == 0 {
		return nil, nil
	}
	insp, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("docker: inspect container %q: %w", containerID, err)
	}
	user := ""
	if insp.Config != nil {
		user = insp.Config.User
	}
	uid, gid := 0, 0
	if !isRootUser(user) {
		if uid, gid, err = c.resolveImageUser(ctx, containerID, user); err != nil {
			return nil, fmt.Errorf("docker: user %q: %w", user, err)
		}
	}
	out := make([]VolumeAccess, 0, len(volumes))
	for _, v := range volumes {
		if v.ReadOnly {
			continue
		}
		a := VolumeAccess{Volume: v.Name, Path: v.ContainerPath, User: user, UID: uid, GID: gid, Writable: true}
		if uid != 0 {
			root, _, err := c.volumeRoot(ctx, containerID, v.ContainerPath)
			if err != nil {
				return nil, fmt.Errorf("docker: %w", err)
			}
			a.OwnerUID, a.OwnerGID, a.Mode = root.Uid, root.Gid, root.Mode
			a.Writable = dirWritableBy(uid, gid, root.Uid, root.Gid, root.Mode)
		}
		out = append(out, a)
	}
	return out, nil
}
