package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
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

	mux.HandleFunc("GET /api/v1/log-archive/policies", rt.requireAbility(AbilityRead, rt.handleListLogArchivePolicies))
	mux.HandleFunc("PUT /api/v1/log-archive/policy", rt.requireAbility(AbilityWrite, rt.handleSetLogArchivePolicy))
	mux.HandleFunc("DELETE /api/v1/log-archive/policy", rt.requireAbility(AbilityWrite, rt.handleDeleteLogArchivePolicy))
	mux.HandleFunc("POST /api/v1/log-archive/dump", rt.requireAbility(AbilityWrite, rt.handleLogArchiveDump))
	mux.HandleFunc("GET /api/v1/log-archive/runs", rt.requireAbility(AbilityRead, rt.handleListLogArchiveRuns))
	mux.HandleFunc("GET /api/v1/log-archive/objects", rt.requireAbility(AbilityRead, rt.handleListLogArchiveObjects))
	mux.HandleFunc("GET /api/v1/log-archive/objects/download", rt.requireAbility(AbilityRead, rt.handleDownloadLogArchiveObject))
}

func randomLogArchiveID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate %s id: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}
