package live

import (
	"context"

	dockerclient "github.com/moby/moby/client"

	"github.com/glincker/levelrail/internal/benchreport"
)

// DockerInfo asks the local daemon what it is; an unreachable daemon yields the zero value.
func DockerInfo(ctx context.Context) benchreport.Docker {
	var d benchreport.Docker
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return d
	}
	defer func() { _ = cli.Close() }()
	if v, err := cli.ServerVersion(ctx); err == nil {
		d.Version = v.Version
	}
	if info, err := cli.Info(ctx); err == nil {
		d.OS = info.OperatingSystem
		d.MemBytes = info.MemTotal
	}
	return d
}
