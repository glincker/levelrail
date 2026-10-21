package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// registerMultiNodeRoutes wires node moves, drains, labels, selectors and
// mesh pair diagnostics.
func (rt *Router) registerMultiNodeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/apps/{name}/move", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleStartMove(store.NodeMoveKindApp)))
	mux.HandleFunc("POST /api/v1/databases/{name}/move", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleStartMove(store.NodeMoveKindDatabase)))
	mux.HandleFunc("GET /api/v1/apps/{name}/placement", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPlacement(store.NodeMoveKindApp)))
	mux.HandleFunc("GET /api/v1/apps/{name}/placement/decision", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPlacementDecision(store.NodeMoveKindApp)))
	mux.HandleFunc("GET /api/v1/databases/{name}/placement/decision", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetPlacementDecision(store.NodeMoveKindDatabase)))
	mux.HandleFunc("GET /api/v1/nodes/allocations", rt.requireAbility(AbilityRoot, rt.handleNodeAllocations))
	mux.HandleFunc("GET /api/v1/nodes/zones", rt.requireAbility(AbilityRoot, rt.handleListZones))
	mux.HandleFunc("GET /api/v1/nodes/placement-simulation", rt.requireAbility(AbilityRoot, rt.handlePlacementSimulation))
	mux.HandleFunc("GET /api/v1/nodes/failure-simulation", rt.requireAbility(AbilityRoot, rt.handleFailureSimulation))
	mux.HandleFunc("PUT /api/v1/apps/{name}/placement", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetPlacement(store.NodeMoveKindApp)))
	mux.HandleFunc("GET /api/v1/databases/{name}/placement", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetPlacement(store.NodeMoveKindDatabase)))
	mux.HandleFunc("PUT /api/v1/databases/{name}/placement", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleSetPlacement(store.NodeMoveKindDatabase)))
	mux.HandleFunc("GET /api/v1/node-moves", rt.requireAbility(AbilityRoot, rt.handleListNodeMoves))
	mux.HandleFunc("GET /api/v1/node-moves/{id}", rt.requireAbility(AbilityRoot, rt.handleGetNodeMove))
	mux.HandleFunc("GET /api/v1/node-drains/{id}", rt.requireAbility(AbilityRoot, rt.handleGetNodeDrain))
	mux.HandleFunc("GET /api/v1/nodes/{id}/drains", rt.requireAbility(AbilityRoot, rt.handleListNodeDrains))
	mux.HandleFunc("GET /api/v1/nodes/{id}/labels", rt.requireAbility(AbilityRoot, rt.handleGetNodeLabels))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/labels", rt.requireAbility(AbilityRoot, rt.handleSetNodeLabels))
	mux.HandleFunc("GET /api/v1/nodes/{id}/failure", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleGetNodeFailure))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/failure-policy", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleSetNodeFailurePolicy))
	mux.HandleFunc("POST /api/v1/nodes/{id}/failure/lost", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleMarkNodeLost))
	mux.HandleFunc("POST /api/v1/nodes/{id}/failure/resolve", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleResolveNodeFailure))
	mux.HandleFunc("GET /api/v1/mesh/connectivity", rt.requireAbility(AbilityRoot, rt.handleMeshConnectivity))
}
