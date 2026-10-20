package docker

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// LocalNetwork is one network a container is attached to and the address it
// has there.
type LocalNetwork struct {
	Name string
	IP   string
	// Gateway is the host side of the network, reachable from the container.
	Gateway string
}

// ContainerMount is one mount: host Source at container Destination.
type ContainerMount struct {
	Source      string
	Destination string
}

// PortMapping is one published port: container Private on host Public.
type PortMapping struct {
	Private int
	Public  int
}

// LocalContainer is a container on this Docker daemon with its networks and
// ports, used to find a database to migrate from or a reverse proxy holding
// the host's web ports. Args and ExtraHosts are only filled by
// InspectLocalContainer.
type LocalContainer struct {
	Name     string
	Image    string
	Running  bool
	Networks []LocalNetwork
	// Ports are every port the image exposes, published or not.
	Ports []int
	// Published are the host ports this container publishes.
	Published   []int
	Mappings    []PortMapping
	Mounts      []ContainerMount
	NetworkMode string
	Args        []string
	ExtraHosts  []string
}

// ListLocalContainers returns every container on the daemon, running or not.
func (c *Client) ListLocalContainers(ctx context.Context) ([]LocalContainer, error) {
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}
	out := make([]LocalContainer, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, localFromSummary(s))
	}
	return out, nil
}

func localFromSummary(s container.Summary) LocalContainer {
	lc := LocalContainer{Image: s.Image, Running: s.State == "running", NetworkMode: s.HostConfig.NetworkMode}
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
	for _, m := range s.Mounts {
		lc.Mounts = append(lc.Mounts, ContainerMount{Source: m.Source, Destination: m.Destination})
	}
	var mappings []PortMapping
	for _, p := range s.Ports {
		lc.Ports = append(lc.Ports, int(p.PrivatePort))
		if p.PublicPort != 0 {
			mappings = append(mappings, PortMapping{Private: int(p.PrivatePort), Public: int(p.PublicPort)})
		}
	}
	lc.setMappings(mappings)
	lc.normalise()
	return lc
}

// InspectLocalContainer returns name with its full launch configuration:
// entrypoint plus command, mounts, extra hosts and published ports. Used to
// read a foreign reverse proxy's configuration, never to manage it.
func (c *Client) InspectLocalContainer(ctx context.Context, name string) (LocalContainer, error) {
	resp, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return LocalContainer{}, fmt.Errorf("docker: inspect %q: %w", name, err)
	}
	return localFromInspect(resp), nil
}

func localFromInspect(resp container.InspectResponse) LocalContainer {
	var lc LocalContainer
	if resp.Config != nil {
		lc.Image = resp.Config.Image
		lc.Args = append(lc.Args, resp.Config.Entrypoint...)
		lc.Args = append(lc.Args, resp.Config.Cmd...)
	}
	if resp.ContainerJSONBase != nil {
		lc.Name = strings.TrimPrefix(resp.Name, "/")
		lc.Running = resp.State != nil && resp.State.Running
		if len(lc.Args) == 0 {
			lc.Args = append([]string{resp.Path}, resp.Args...)
		}
		if resp.HostConfig != nil {
			lc.ExtraHosts = append(lc.ExtraHosts, resp.HostConfig.ExtraHosts...)
			lc.NetworkMode = string(resp.HostConfig.NetworkMode)
		}
	}
	for _, m := range resp.Mounts {
		lc.Mounts = append(lc.Mounts, ContainerMount{Source: m.Source, Destination: m.Destination})
	}
	if resp.NetworkSettings != nil {
		for name, ep := range resp.NetworkSettings.Networks {
			if ep != nil && ep.IPAddress != "" {
				lc.Networks = append(lc.Networks, LocalNetwork{Name: name, IP: ep.IPAddress, Gateway: ep.Gateway})
			}
		}
		lc.setMappings(mappingsFromPortMap(resp.NetworkSettings.Ports))
	}
	lc.normalise()
	return lc
}

func mappingsFromPortMap(pm nat.PortMap) []PortMapping {
	var out []PortMapping
	for port, bindings := range pm {
		for _, b := range bindings {
			public, err := strconv.Atoi(b.HostPort)
			if err != nil || public == 0 {
				continue
			}
			out = append(out, PortMapping{Private: port.Int(), Public: public})
		}
	}
	return out
}

func (lc *LocalContainer) setMappings(m []PortMapping) {
	seen := map[PortMapping]bool{}
	for _, p := range m {
		if seen[p] {
			continue
		}
		seen[p] = true
		lc.Mappings = append(lc.Mappings, p)
		lc.Published = append(lc.Published, p.Public)
		lc.Ports = append(lc.Ports, p.Private)
	}
	sort.Slice(lc.Mappings, func(i, j int) bool {
		if lc.Mappings[i].Public != lc.Mappings[j].Public {
			return lc.Mappings[i].Public < lc.Mappings[j].Public
		}
		return lc.Mappings[i].Private < lc.Mappings[j].Private
	})
}

func (lc *LocalContainer) normalise() {
	sort.Slice(lc.Networks, func(i, j int) bool { return lc.Networks[i].Name < lc.Networks[j].Name })
	lc.Ports = uniqueSorted(lc.Ports)
	lc.Published = uniqueSorted(lc.Published)
}

func uniqueSorted(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	sort.Ints(in)
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
