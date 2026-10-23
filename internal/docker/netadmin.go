package docker

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/filters"
	dockernetwork "github.com/docker/docker/api/types/network"
)

const (
	networkOwnerKindSuffix = "network-owner-kind"
	networkOwnerNameSuffix = "network-owner-name"
	appNetworkInfix        = "-app-"
)

// NetworkDetail is one product network with the facts GC and the
// diagnostics API need.
type NetworkDetail struct {
	ID       string
	Name     string
	Labels   map[string]string
	Subnets  []string
	Created  time.Time
	Attached int
}

// PoolUsage is capacity and use of the configured network pool.
type PoolUsage struct {
	Pool     string
	Enabled  bool
	Capacity int
	InUse    int
}

// NetworkAdmin is the optional surface a local Docker client offers on top
// of Runtime; remote node transports do not implement it.
type NetworkAdmin interface {
	DescribeNetworks(ctx context.Context, prefix string) ([]NetworkDetail, error)
	RemoveNetworkIfEmpty(ctx context.Context, name string) (bool, error)
	NetworkPoolUsage(ctx context.Context) (PoolUsage, error)
}

// SiblingLabelKey derives a network label key next to the instance label
// key, so owner labels share the reserved prefix without importing spec.
func SiblingLabelKey(instanceKey, suffix string) string {
	return strings.TrimSuffix(instanceKey, "instance") + suffix
}

// OwnerKindLabelKey and OwnerNameLabelKey are the owner label keys.
func OwnerKindLabelKey(instanceKey string) string {
	return SiblingLabelKey(instanceKey, networkOwnerKindSuffix)
}

// OwnerNameLabelKey is the label key holding the owning resource's name.
func OwnerNameLabelKey(instanceKey string) string {
	return SiblingLabelKey(instanceKey, networkOwnerNameSuffix)
}

func (c *Client) networkLabels(name string) map[string]string {
	labels := c.withInstanceLabel(nil)
	if !c.hasInstanceLabel() {
		return labels
	}
	if i := strings.Index(name, appNetworkInfix); i >= 0 {
		labels[OwnerKindLabelKey(c.instanceLabelKey)] = "app"
		labels[OwnerNameLabelKey(c.instanceLabelKey)] = name[i+len(appNetworkInfix):]
	}
	return labels
}

// WithNetworkPool sets the pool explicitly (tests, embedders); NewClient
// otherwise reads APP_NETWORK_POOL.
func WithNetworkPool(p NetworkPool) ClientOption {
	return func(c *Client) { c.pool = &p }
}

func poolFromEnv(lookup func(string) (string, bool)) *NetworkPool {
	v, _ := lookup(EnvNetworkPool)
	p, ok, err := ParseNetworkPool(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v; using the Docker daemon's default pools\n", err)
		return nil
	}
	if !ok {
		return nil
	}
	return &p
}

// SelectiveNetworkDescriber is DescribeNetworks for a caller that needs the
// attached-container count only for some networks. The list endpoint carries
// everything else, so the rest cost no extra round trip.
type SelectiveNetworkDescriber interface {
	DescribeNetworksSelective(ctx context.Context, prefix string, needAttached func(name string, labels map[string]string) bool) ([]NetworkDetail, error)
}

// DescribeNetworks implements NetworkAdmin. It inspects each match because
// the list endpoint does not reliably report attached containers.
func (c *Client) DescribeNetworks(ctx context.Context, prefix string) ([]NetworkDetail, error) {
	return c.DescribeNetworksSelective(ctx, prefix, func(string, map[string]string) bool { return true })
}

// DescribeNetworksSelective implements SelectiveNetworkDescriber. A network
// needAttached declines is reported with Attached zero, taken from the list.
func (c *Client) DescribeNetworksSelective(ctx context.Context, prefix string, needAttached func(name string, labels map[string]string) bool) ([]NetworkDetail, error) {
	f := filters.NewArgs()
	f.Add("name", prefix)
	list, err := c.cli.NetworkList(ctx, dockernetwork.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list networks with prefix %q: %w", prefix, err)
	}
	out := make([]NetworkDetail, 0, len(list))
	for _, n := range list {
		if !strings.HasPrefix(n.Name, prefix) {
			continue
		}
		insp := n
		if needAttached(n.Name, n.Labels) {
			insp, err = c.cli.NetworkInspect(ctx, n.ID, dockernetwork.InspectOptions{})
			if err != nil {
				if cerrdefs.IsNotFound(err) {
					continue
				}
				return nil, fmt.Errorf("docker: inspect network %q: %w", n.Name, err)
			}
		}
		d := NetworkDetail{ID: insp.ID, Name: insp.Name, Labels: insp.Labels, Created: insp.Created, Attached: len(insp.Containers)}
		for _, s := range networkSubnets(insp) {
			d.Subnets = append(d.Subnets, s.String())
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RemoveNetworkIfEmpty implements NetworkAdmin: it never disconnects.
func (c *Client) RemoveNetworkIfEmpty(ctx context.Context, name string) (bool, error) {
	insp, err := c.cli.NetworkInspect(ctx, name, dockernetwork.InspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("docker: inspect network %q: %w", name, err)
	}
	if len(insp.Containers) > 0 {
		return false, nil
	}
	if err := c.cli.NetworkRemove(ctx, insp.ID); err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("docker: remove network %q: %w", name, err)
	}
	return true, nil
}

// NetworkPoolUsage implements NetworkAdmin.
func (c *Client) NetworkPoolUsage(ctx context.Context) (PoolUsage, error) {
	if c.pool == nil {
		return PoolUsage{}, nil
	}
	nets, err := c.cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return PoolUsage{}, fmt.Errorf("docker: list networks: %w", err)
	}
	return PoolUsage{Pool: c.pool.Base.String(), Enabled: true, Capacity: c.pool.Capacity(), InUse: poolUsage(*c.pool, nets)}, nil
}
