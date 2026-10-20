package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DatabaseMeshPorts allocates a database's stable mesh port.
type DatabaseMeshPorts interface {
	EnsureDatabaseMeshPort(ctx context.Context, name string, minPort, maxPort int) (int, error)
}

// MeshDatabaseConfig wires cross-node database connections over the mesh.
type MeshDatabaseConfig struct {
	Endpoints   meshpath.EndpointResolver
	Ports       DatabaseMeshPorts
	Zone        string
	LocalNodeID string
	MinPort     int
	MaxPort     int
}

// WithMeshDatabases lets an app reach a managed database on another node
// at <db>.<zone>:<mesh port>, the name pinned in /etc/hosts to the
// database node's mesh address, so it resolves with no DNS server.
func WithMeshDatabases(cfg MeshDatabaseConfig) Option {
	return func(c *Controller) { c.meshDB = &cfg }
}

// SameNode reports whether two node IDs name the same node, treating ""
// and the control plane's own node ID as one.
func SameNode(a, b, localNodeID string) bool {
	if a == localNodeID {
		a = ""
	}
	if b == localNodeID {
		b = ""
	}
	return a == b
}

type dbEndpoint struct {
	host string
	port int
	ip   string
}

// crossNodeDatabase returns the mesh endpoint for db when it runs on a
// different node than appNodeID, or ok=false when they share a node.
func (c *Controller) crossNodeDatabase(ctx context.Context, appNodeID string, db *store.DesiredDatabase) (dbEndpoint, bool, error) {
	local := ""
	if c.meshDB != nil {
		local = c.meshDB.LocalNodeID
	}
	if SameNode(appNodeID, db.NodeID, local) {
		return dbEndpoint{}, false, nil
	}
	if c.meshDB == nil || c.meshDB.Zone == "" {
		return dbEndpoint{}, false, fmt.Errorf("database %q runs on another node than this app: cross-node database connections need mesh networking (APP_MESH_ENABLED=1), or move the app or the database so they share a node", db.Name)
	}
	port, err := c.meshDB.Ports.EnsureDatabaseMeshPort(ctx, db.Name, c.meshDB.MinPort, c.meshDB.MaxPort)
	if err != nil {
		return dbEndpoint{}, false, fmt.Errorf("database %q: allocate mesh port: %w", db.Name, err)
	}
	path, err := c.meshDB.Endpoints.Endpoint(ctx, db.NodeID)
	if err != nil {
		return dbEndpoint{}, false, fmt.Errorf("database %q: mesh endpoint: %w", db.Name, err)
	}
	if !path.Usable {
		return dbEndpoint{}, false, fmt.Errorf("database %q is on another node with no usable mesh path yet: %s", db.Name, path.Reason)
	}
	return dbEndpoint{host: DatabaseHost(db.Name, c.meshDB.Zone), port: port, ip: path.Address}, true, nil
}

// crossNodeDatabaseName reports a managed database on another node than
// appNodeID; it is reached over the mesh, not a shared Docker network.
func (c *Controller) crossNodeDatabaseName(ctx context.Context, appNodeID, dbName string) bool {
	if c.meshDB == nil || c.databases == nil {
		return false
	}
	db, err := c.databases.GetDesiredDatabase(ctx, dbName)
	if err != nil {
		return false
	}
	return !SameNode(appNodeID, db.NodeID, c.meshDB.LocalNodeID)
}

// databaseExtraHosts pins every cross-node database name desired uses to
// its node's mesh address.
func (c *Controller) databaseExtraHosts(ctx context.Context, desired *store.DesiredService) ([]string, error) {
	if c.meshDB == nil || c.databases == nil {
		return nil, nil
	}
	names := map[string]bool{}
	for _, ref := range desired.DatabaseEnv {
		names[ref.Database] = true
	}
	if att := desired.DatabaseAttachment; att != nil {
		names[att.DatabaseName] = true
	}
	seen := map[string]bool{}
	var out []string
	for name := range names {
		if ext, err := c.lookupExternal(ctx, name); err != nil || ext != nil {
			continue
		}
		db, err := c.databases.GetDesiredDatabase(ctx, name)
		if err != nil {
			continue
		}
		ep, ok, err := c.crossNodeDatabase(ctx, desired.NodeID, db)
		if err != nil {
			return nil, err
		}
		if ok {
			// No attachment happens cross-node, so the network scope is enforced here instead.
			allowed, why, err := c.databaseScopeAllows(ctx, desired, name)
			if err != nil {
				return nil, err
			}
			if !allowed {
				return nil, fmt.Errorf("database %q is outside this app's network scope: %s", name, why)
			}
		}
		if ok && !seen[ep.host] {
			seen[ep.host] = true
			out = append(out, ep.host+":"+ep.ip)
		}
	}
	sort.Strings(out)
	return out, nil
}
