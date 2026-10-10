package docker

import (
	"context"
	"fmt"
)

// ContainerMount is one mount of an inspected container.
type ContainerMount struct {
	Source      string
	Destination string
}

// ContainerLaunch is how a container was started: its full argument list
// (entrypoint plus command), mounts, extra hosts and network mode. Used to
// read a foreign reverse proxy's configuration, never to manage it.
type ContainerLaunch struct {
	Args        []string
	Mounts      []ContainerMount
	ExtraHosts  []string
	NetworkMode string
}

// InspectLaunch returns name's launch configuration.
func (c *Client) InspectLaunch(ctx context.Context, name string) (ContainerLaunch, error) {
	resp, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return ContainerLaunch{}, fmt.Errorf("docker: inspect %q: %w", name, err)
	}
	var out ContainerLaunch
	if resp.Config != nil {
		out.Args = append(out.Args, resp.Config.Entrypoint...)
		out.Args = append(out.Args, resp.Config.Cmd...)
	}
	if resp.ContainerJSONBase != nil {
		if len(out.Args) == 0 {
			out.Args = append(out.Args, resp.Args...)
		}
		if resp.HostConfig != nil {
			out.ExtraHosts = append(out.ExtraHosts, resp.HostConfig.ExtraHosts...)
			out.NetworkMode = string(resp.HostConfig.NetworkMode)
		}
	}
	for _, m := range resp.Mounts {
		out.Mounts = append(out.Mounts, ContainerMount{Source: m.Source, Destination: m.Destination})
	}
	return out, nil
}
