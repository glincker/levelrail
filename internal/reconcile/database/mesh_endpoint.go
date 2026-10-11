package database

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MeshPortLookup reads a database's allocated mesh port (0 when none).
type MeshPortLookup interface {
	GetDatabaseMeshPort(ctx context.Context, name string) (int, error)
}

// meshEndpointResolver is meshpath.EndpointResolver, kept local so the mesh
// dependency stays confined to this file.
type meshEndpointResolver = meshpath.EndpointResolver

// WithMeshEndpoint publishes a database that has a mesh port on its node's
// mesh address, so apps on other nodes can reach it. Only databases with a
// cross-node consumer ever get a port, and the bind is never a wildcard.
func WithMeshEndpoint(r meshpath.EndpointResolver, ports MeshPortLookup) Option {
	return func(c *Controller) {
		c.meshEndpoints = r
		c.meshPorts = ports
	}
}

// DataVolumeName is the Docker volume holding a database's data.
func DataVolumeName(dbName string) string { return dataVolumeName(dbName) }

// WALArchiveVolumeName is the Docker volume holding a database's PITR WAL archive.
func WALArchiveVolumeName(dbName string) string { return walArchiveVolumeName(dbName) }

// meshBinding returns the mesh port binding desired should carry, if any.
// When the mesh path is down it keeps an existing binding as observed, so
// a mesh flap never restarts the database.
func (c *Controller) meshBinding(ctx context.Context, desired *store.DesiredDatabase, state *docker.ContainerState, containerPort int) (docker.PortBinding, bool) {
	if c.meshEndpoints == nil || c.meshPorts == nil {
		return docker.PortBinding{}, false
	}
	port, err := c.meshPorts.GetDatabaseMeshPort(ctx, c.dbName)
	if err != nil || port == 0 {
		return docker.PortBinding{}, false
	}
	path, err := c.meshEndpoints.Endpoint(ctx, desired.NodeID)
	if err == nil && path.Usable && meshpath.ValidateBindIP(path.Address) == nil {
		return docker.PortBinding{ContainerPort: containerPort, HostPort: port, HostIP: path.Address}, true
	}
	if state != nil {
		for _, p := range state.Ports {
			if p.HostPort == port && p.ContainerPort == containerPort && meshpath.ValidateBindIP(p.HostIP) == nil {
				return docker.PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, HostIP: p.HostIP, Protocol: p.Protocol}, true
			}
		}
	}
	return docker.PortBinding{}, false
}
