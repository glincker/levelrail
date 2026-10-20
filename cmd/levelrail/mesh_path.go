package main

import (
	"os"

	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newMeshPathResolver reports the WireGuard path to each node for the app
// and ingress controllers and the API. A nil meshCfg yields a resolver that
// says mesh networking is off.
func newMeshPathResolver(meshCfg *meshSetup, db *store.DB) *meshpath.Tracker {
	if meshCfg == nil {
		return meshpath.NewTracker("", nil, db)
	}
	return meshpath.NewTracker(meshCfg.localNodeID, meshCfg.device, db)
}

// meshDatabaseConfig wires cross-node database connections for app controllers.
func meshDatabaseConfig(deps dynamicSourceDeps) application.MeshDatabaseConfig {
	lo, hi := meshpath.DatabasePortRange(os.LookupEnv)
	return application.MeshDatabaseConfig{
		Endpoints:   newMeshPathResolver(deps.meshCfg, deps.db),
		Ports:       deps.db,
		Zone:        deps.meshCfg.resolver.Zone(),
		LocalNodeID: deps.meshCfg.localNodeID,
		MinPort:     lo,
		MaxPort:     hi,
	}
}
