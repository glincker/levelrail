package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// LocalNetwork is one network a container is attached to and the address it
// has there.
type LocalNetwork struct {
	Name string
	IP   string
	// Gateway is the host side of the network, reachable from the container.
	Gateway string
}

// LocalContainer is a container on this Docker daemon with its networks and
// ports, used to find a database to migrate from without going through the
// host's public address.
type LocalContainer struct {
	Name     string
	Image    string
	Running  bool
	Networks []LocalNetwork
	// Ports are every port the image exposes, published or not.
	Ports []int
	// Published are the host ports this container publishes.
	Published []int
}

// ListLocalContainers returns every container on the daemon, running or not.
func (c *Client) ListLocalContainers(ctx context.Context) ([]LocalContainer, error) {
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}
	out := make([]LocalContainer, 0, len(summaries))
	for _, s := range summaries {
		lc := LocalContainer{Image: s.Image, Running: s.State == "running"}
		if len(s.Names) > 0 {
			lc.Name = strings.TrimPrefix(s.Names[0], "/")
		}
		if s.NetworkSettings != nil {
			for name, ep := range s.NetworkSettings.Networks {
				if ep != nil && ep.IPAddress != "" {
					lc.Networks = append(lc.Networks, LocalNetwork{Name: name, IP: ep.IPAddress, Gateway: ep.Gateway})
				}
			}
		}
		sort.Slice(lc.Networks, func(i, j int) bool { return lc.Networks[i].Name < lc.Networks[j].Name })
		seen := map[int]bool{}
		for _, p := range s.Ports {
			if !seen[int(p.PrivatePort)] {
				seen[int(p.PrivatePort)] = true
				lc.Ports = append(lc.Ports, int(p.PrivatePort))
			}
			if p.PublicPort != 0 {
				lc.Published = append(lc.Published, int(p.PublicPort))
			}
		}
		sort.Ints(lc.Ports)
		sort.Ints(lc.Published)
		out = append(out, lc)
	}
	return out, nil
}
