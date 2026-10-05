package main

import (
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newMeshPathResolver reports the WireGuard path to each node for the app
// and ingress controllers and the API. A nil meshCfg yields a resolver that
// says mesh networking is off.
func newMeshPathResolver(meshCfg *meshSetup, db *store.DB) meshpath.Resolver {
	if meshCfg == nil {
		return meshpath.NewTracker("", nil, db)
	}
	return meshpath.NewTracker(meshCfg.localNodeID, meshCfg.device, db)
}
