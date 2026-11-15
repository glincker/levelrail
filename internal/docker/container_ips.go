package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// AppContainerIPs maps each running app container's network IPs to its app name.
func (c *Client) AppContainerIPs(ctx context.Context) (map[string]string, error) {
	f := filters.NewArgs()
	f.Add("label", containerAppLabel)
	f.Add("status", "running")
	c.instanceLabelFilter(f)
	list, err := c.cli.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list app containers for ip map: %w", err)
	}
	out := map[string]string{}
	for _, s := range list {
		app := s.Labels[containerAppLabel]
		if app == "" || s.NetworkSettings == nil {
			continue
		}
		for _, n := range s.NetworkSettings.Networks {
			if n != nil && n.IPAddress != "" {
				out[n.IPAddress] = app
			}
		}
	}
	return out, nil
}
