package buildkitd

import (
	"context"
	"io"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
)

type dockerEngine struct{ cli *dockerclient.Client }

// NewDockerEngine adapts an Engine API client to Engine.
func NewDockerEngine(cli *dockerclient.Client) Engine { return dockerEngine{cli: cli} }

func (d dockerEngine) InspectContainer(ctx context.Context, name string) (*Observed, error) {
	r, err := d.cli.ContainerInspect(ctx, name)
	if cerrdefs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o := &Observed{ID: r.ID}
	if r.Config != nil {
		o.Image, o.Labels = r.Config.Image, r.Config.Labels
	}
	if r.State != nil {
		o.Running = r.State.Running
	}
	if r.HostConfig != nil {
		o.Privileged = r.HostConfig.Privileged
	}
	for _, mp := range r.Mounts {
		o.Mounts = append(o.Mounts, Mount{Name: mp.Name, Destination: mp.Destination})
	}
	return o, nil
}

func (d dockerEngine) CreateContainer(ctx context.Context, s CreateSpec) (string, error) {
	cfg := &container.Config{Image: s.Image, Cmd: s.Cmd, Labels: s.Labels}
	hc := &container.HostConfig{
		SecurityOpt: s.SecurityOpt,
		Mounts:      []mount.Mount{{Type: mount.TypeVolume, Source: s.Volume, Target: s.DataPath}},
	}
	if s.PidsLimit > 0 {
		hc.PidsLimit = &s.PidsLimit
	}
	if s.CPUShares > 0 {
		hc.CPUShares = s.CPUShares
	}
	r, err := d.cli.ContainerCreate(ctx, cfg, hc, nil, nil, s.Name)
	if cerrdefs.IsConflict(err) {
		return "", ErrConflict
	}
	return r.ID, err
}

func (d dockerEngine) StartContainer(ctx context.Context, id string) error {
	err := d.cli.ContainerStart(ctx, id, container.StartOptions{})
	if cerrdefs.IsNotModified(err) {
		return nil
	}
	return err
}

func (d dockerEngine) PutFiles(ctx context.Context, id, dir string, body io.Reader) error {
	return d.cli.CopyToContainer(ctx, id, dir, body, container.CopyToContainerOptions{})
}

func (d dockerEngine) HasImage(ctx context.Context, ref string) (bool, error) {
	_, err := d.cli.ImageInspect(ctx, ref)
	if cerrdefs.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (d dockerEngine) PullImage(ctx context.Context, ref string) error {
	rc, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return DecodePullStream(rc)
}

func (d dockerEngine) InspectVolume(ctx context.Context, name string) (map[string]string, bool, error) {
	v, err := d.cli.VolumeInspect(ctx, name)
	if cerrdefs.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v.Labels, true, nil
}

func (d dockerEngine) CreateVolume(ctx context.Context, name string, labels map[string]string) error {
	_, err := d.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name, Labels: labels})
	return err
}
