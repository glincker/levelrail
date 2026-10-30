package docker

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/docker/api/types/volume"
)

// Label keys stamped on a volume when it is created for an owner.
const (
	VolumeOwnerAppLabel     = "levelrail.volume.app"
	VolumeOwnerServiceLabel = "levelrail.volume.service"
	VolumeOwnerNameLabel    = "levelrail.volume.name"
)

// ErrVolumeOwnerMismatch means a volume of that name belongs to another owner.
var ErrVolumeOwnerMismatch = errors.New("volume belongs to a different owner")

// VolumeOwner identifies who a volume is for. An empty App means unknown.
type VolumeOwner struct {
	App     string
	Service string
	Name    string
}

func (o VolumeOwner) labels() map[string]string {
	if o.App == "" {
		return nil
	}
	out := map[string]string{VolumeOwnerAppLabel: o.App}
	if o.Service != "" {
		out[VolumeOwnerServiceLabel] = o.Service
	}
	if o.Name != "" {
		out[VolumeOwnerNameLabel] = o.Name
	}
	return out
}

// checkVolumeOwner rejects a volume whose owner label is set and differs.
// A volume without the label predates ownership and is accepted as is.
func checkVolumeOwner(name string, existing map[string]string, want VolumeOwner) error {
	if want.App == "" {
		return nil
	}
	have, ok := existing[VolumeOwnerAppLabel]
	if !ok || have == want.App {
		return nil
	}
	return fmt.Errorf("docker: volume %q: %w", name, ErrVolumeOwnerMismatch)
}

// OwnedVolumeEnsurer is implemented by runtimes that record volume owners.
type OwnedVolumeEnsurer interface {
	EnsureOwnedVolume(ctx context.Context, name string, owner VolumeOwner) error
}

// EnsureVolumeFor ensures a volume with owner checks when the runtime
// supports them, and falls back to a plain ensure otherwise.
func EnsureVolumeFor(ctx context.Context, rt interface {
	EnsureVolume(ctx context.Context, name string) error
}, name string, owner VolumeOwner) error {
	if o, ok := rt.(OwnedVolumeEnsurer); ok {
		return o.EnsureOwnedVolume(ctx, name, owner)
	}
	return rt.EnsureVolume(ctx, name)
}

// EnsureOwnedVolume creates the volume stamped with owner, or accepts an
// existing one that has no owner label or the same one.
func (c *Client) EnsureOwnedVolume(ctx context.Context, name string, owner VolumeOwner) error {
	v, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: c.withInstanceLabel(owner.labels()),
	})
	if err != nil {
		return fmt.Errorf("docker: ensure volume %q: %w", name, err)
	}
	return checkVolumeOwner(name, v.Labels, owner)
}
