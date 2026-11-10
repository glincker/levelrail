package database

import (
	"context"
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// defaultBridgeNetwork is Docker's shared network every container without
// an explicit network joins, reachable from any unrelated container.
const defaultBridgeNetwork = "bridge"

// PrivateNetworkName is the per-database network a managed database is
// created on, so it never sits on the default bridge. Apps reach it on
// their own network via NetworkConnect, by container name.
func PrivateNetworkName(dbName string) string {
	return containerName(dbName) + "-net"
}

// attachPrivateNetwork sets both NetworkMode and the endpoint: with
// NetworkMode left empty Docker would also join the default bridge.
func attachPrivateNetwork(spec *docker.ContainerSpec, dbName string) {
	netName := PrivateNetworkName(dbName)
	spec.NetworkMode = netName
	spec.Network = &docker.NetworkAttachment{Name: netName, Alias: containerName(dbName)}
}

func onDefaultBridge(state *docker.ContainerState) bool {
	return state != nil && slices.ContainsFunc(state.Networks, func(n docker.ContainerNetwork) bool {
		return n.Name == defaultBridgeNetwork
	})
}

// leaveDefaultBridge moves a running container created before the private
// network existed: join the private network first so the host-side probe
// and every app attachment keep a route, then drop the default bridge.
func (c *Controller) leaveDefaultBridge(ctx context.Context, state *docker.ContainerState) error {
	if !onDefaultBridge(state) {
		return nil
	}
	netName := PrivateNetworkName(c.dbName)
	if _, err := c.runtime.EnsureNetwork(ctx, netName); err != nil {
		return fmt.Errorf("ensure private network %q: %w", netName, err)
	}
	if err := c.runtime.NetworkConnect(ctx, netName, state.ID); err != nil {
		return fmt.Errorf("connect to private network %q: %w", netName, err)
	}
	if err := c.runtime.NetworkDisconnect(ctx, defaultBridgeNetwork, state.ID, false); err != nil {
		return fmt.Errorf("leave the default bridge network: %w", err)
	}
	return nil
}
