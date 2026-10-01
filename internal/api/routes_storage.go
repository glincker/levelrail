package api

import (
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/idgen"
)

// registerStorageRoutes wires storage destinations and log archive. Reads
// are AbilityRead like the log query routes; anything that touches
// credentials is AbilityWriteSensitive.
func (rt *Router) registerStorageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/storage/providers", rt.requireAbility(AbilityRead, rt.handleListStorageProviders))
	mux.HandleFunc("GET /api/v1/storage/destinations", rt.requireAbility(AbilityRead, rt.handleListStorageDestinations))
	mux.HandleFunc("POST /api/v1/storage/destinations", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateStorageDestination))
	mux.HandleFunc("GET /api/v1/storage/destinations/{id}", rt.requireAbility(AbilityRead, rt.handleGetStorageDestination))
	mux.HandleFunc("PUT /api/v1/storage/destinations/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateStorageDestination))
	mux.HandleFunc("DELETE /api/v1/storage/destinations/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteStorageDestination))
	mux.HandleFunc("POST /api/v1/storage/destinations/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestStorageDestination))

	mux.HandleFunc("GET /api/v1/build-cache", rt.requireAbility(AbilityRead, rt.handleListBuildCache))
	mux.HandleFunc("PUT /api/v1/build-cache", rt.requireAbility(AbilityWrite, rt.handleSetBuildCache))
	mux.HandleFunc("DELETE /api/v1/build-cache", rt.requireAbility(AbilityWrite, rt.handleDeleteBuildCache))
	mux.HandleFunc("GET /api/v1/build-cache/stats", rt.requireAbility(AbilityRead, rt.handleBuildCacheStats))
	mux.HandleFunc("POST /api/v1/build-cache/clear", rt.requireAbility(AbilityWrite, rt.handleClearBuildCache))

	mux.HandleFunc("GET /api/v1/log-archive/policies", rt.requireAbility(AbilityRead, rt.handleListLogArchivePolicies))
	mux.HandleFunc("PUT /api/v1/log-archive/policy", rt.requireAbility(AbilityWrite, rt.handleSetLogArchivePolicy))
	mux.HandleFunc("DELETE /api/v1/log-archive/policy", rt.requireAbility(AbilityWrite, rt.handleDeleteLogArchivePolicy))
	mux.HandleFunc("POST /api/v1/log-archive/dump", rt.requireAbility(AbilityWrite, rt.handleLogArchiveDump))
	mux.HandleFunc("GET /api/v1/log-archive/runs", rt.requireAbility(AbilityRead, rt.handleListLogArchiveRuns))
	mux.HandleFunc("GET /api/v1/log-archive/objects", rt.requireAbility(AbilityRead, rt.handleListLogArchiveObjects))
	mux.HandleFunc("GET /api/v1/log-archive/objects/download", rt.requireAbility(AbilityRead, rt.handleDownloadLogArchiveObject))
}

// randomLogArchiveID mints a random ID with the given prefix via idgen.
func randomLogArchiveID(prefix string) (string, error) {
	id, err := idgen.New(prefix)
	if err != nil {
		return "", fmt.Errorf("api: generate %s id: %w", prefix, err)
	}
	return id, nil
}
