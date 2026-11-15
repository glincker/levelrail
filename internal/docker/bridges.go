package docker

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockernetwork "github.com/docker/docker/api/types/network"
)

// bridgeNameOption is the label Docker uses for a network's host interface name.
const bridgeNameOption = "com.docker.network.bridge.name"

// ManagedBridge is a Docker bridge network Levelrail containers attach to.
type ManagedBridge struct {
	Name    string
	Iface   string
	Gateway netip.Addr
	Subnet  netip.Prefix
}

// ManagedBridges lists the default bridge plus the bridge networks this instance created, IPv4 only.
func (c *Client) ManagedBridges(ctx context.Context) ([]ManagedBridge, error) {
	nets, err := c.cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: list networks for bridges: %w", err)
	}
	var out []ManagedBridge
	for _, n := range nets {
		if n.Driver != "bridge" || !c.bridgeIsManaged(n) {
			continue
		}
		b := ManagedBridge{Name: n.Name, Iface: n.Options[bridgeNameOption]}
		if b.Iface == "" {
			b.Iface = "br-" + shortID(n.ID)
			if n.Name == "bridge" {
				b.Iface = "docker0"
			}
		}
		for _, cfg := range n.IPAM.Config {
			gw, gerr := netip.ParseAddr(cfg.Gateway)
			sub, serr := netip.ParsePrefix(cfg.Subnet)
			if gerr == nil && serr == nil && gw.Is4() {
				b.Gateway, b.Subnet = gw, sub
				break
			}
		}
		if b.Gateway.IsValid() {
			out = append(out, b)
		}
	}
	return out, nil
}

func (c *Client) bridgeIsManaged(n dockernetwork.Summary) bool {
	if n.Name == "bridge" {
		return true
	}
	own := c.networkLabels(n.Name)
	if len(own) == 0 {
		return false
	}
	for k, v := range own {
		if n.Labels[k] != v {
			return false
		}
	}
	return true
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// DNSUsage counts running app containers and how many have a DNS server in addrs.
func (c *Client) DNSUsage(ctx context.Context, addrs map[string]bool) (using, total int, err error) {
	f := filters.NewArgs()
	f.Add("label", containerAppLabel)
	f.Add("status", "running")
	c.instanceLabelFilter(f)
	list, err := c.cli.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		return 0, 0, fmt.Errorf("docker: list app containers for dns usage: %w", err)
	}
	for _, s := range list {
		info, ierr := c.cli.ContainerInspect(ctx, s.ID)
		if ierr != nil || info.HostConfig == nil {
			continue
		}
		total++
		for _, d := range info.HostConfig.DNS {
			if addrs[d] {
				using++
				break
			}
		}
	}
	return using, total, nil
}
