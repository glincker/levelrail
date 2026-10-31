package docker

import (
	"context"
	"errors"
	"fmt"

	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
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
// A volume without the label is accepted here; EnsureOwnedVolume then checks
// which containers use it.
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

// checkVolumeUsers refuses an unlabelled volume when a container of another
// app mounts it, so a legacy volume in use cannot be adopted.
func checkVolumeUsers(name string, want VolumeOwner, users []map[string]string) error {
	for _, labels := range users {
		if app := labels[appspec.AppLabelKey]; app != "" && app != want.App {
			return fmt.Errorf("docker: volume %q is in use by another app: %w", name, ErrVolumeOwnerMismatch)
		}
	}
	return nil
}

// volumeUserLabels returns the labels of every container, running or
// stopped, that mounts the named volume.
func (c *Client) volumeUserLabels(ctx context.Context, name string) ([]map[string]string, error) {
	f := filters.NewArgs()
	f.Add("volume", name)
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers using volume %q: %w", name, err)
	}
	out := make([]map[string]string, 0, len(list))
	for _, ct := range list {
		out = append(out, ct.Labels)
	}
	return out, nil
}

// EnsureOwnedVolume creates the volume stamped with owner, or accepts an
// existing one with the same owner label, or with none when no other app's
// container uses it.
func (c *Client) EnsureOwnedVolume(ctx context.Context, name string, owner VolumeOwner) error {
	v, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: c.withInstanceLabel(owner.labels()),
	})
	if err != nil {
		return fmt.Errorf("docker: ensure volume %q: %w", name, err)
	}
	if err := checkVolumeOwner(name, v.Labels, owner); err != nil {
		return err
	}
	if _, labelled := v.Labels[VolumeOwnerAppLabel]; labelled || owner.App == "" {
		return nil
	}
	users, err := c.volumeUserLabels(ctx, name)
	if err != nil {
		return err
	}
	return checkVolumeUsers(name, owner, users)
}
