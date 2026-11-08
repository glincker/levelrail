package docker

import (
	"context"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
	dockernetwork "github.com/docker/docker/api/types/network"
)

const iccOption = "com.docker.network.bridge.enable_icc"

// IsolatedNetworkOptions are the isolation settings for a plugin network.
type IsolatedNetworkOptions struct {
	// Internal removes the gateway, so members cannot reach anything outside the network.
	Internal bool
	// DisableICC blocks container to container traffic on the bridge.
	DisableICC bool
	// Recreate replaces an existing network whose settings differ. Docker cannot
	// change these in place, so attached containers are disconnected first.
	Recreate bool
}

func (o IsolatedNetworkOptions) matches(in dockernetwork.Inspect) bool {
	iccOff := in.Options[iccOption] == "false"
	return in.Internal == o.Internal && iccOff == o.DisableICC
}

// EnsureIsolatedNetwork creates name with the given isolation. It reports
// whether it created a network, so a caller can restart members that were
// disconnected by a replacement. An existing network that differs is kept
// unless opts.Recreate is set, so a default install never loses connectivity.
func (c *Client) EnsureIsolatedNetwork(ctx context.Context, name string, opts IsolatedNetworkOptions) (string, bool, error) {
	inspect, err := c.cli.NetworkInspect(ctx, name, dockernetwork.InspectOptions{})
	switch {
	case err == nil:
		if opts.matches(inspect) || !opts.Recreate {
			return inspect.ID, false, nil
		}
		if err := c.removeNetwork(ctx, name, false); err != nil {
			return "", false, err
		}
	case !cerrdefs.IsNotFound(err):
		return "", false, fmt.Errorf("docker: inspect network %q: %w", name, err)
	}

	create := dockernetwork.CreateOptions{Driver: "bridge", Labels: c.networkLabels(name), Internal: opts.Internal}
	if opts.DisableICC {
		create.Options = map[string]string{iccOption: "false"}
	}
	resp, err := c.cli.NetworkCreate(ctx, name, create)
	if err != nil {
		return "", false, fmt.Errorf("docker: create network %q: %w", name, err)
	}
	return resp.ID, true, nil
}
