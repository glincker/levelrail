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

	mux.HandleFunc("GET /api/v1/storage/defaults", rt.requireAbility(AbilityRead, rt.handleListStorageDefaults))
	mux.HandleFunc("PUT /api/v1/storage/defaults/{class}", rt.requireAbility(AbilityWriteSensitive, rt.handleSetStorageDefault))
	mux.HandleFunc("DELETE /api/v1/storage/defaults/{class}", rt.requireAbility(AbilityWriteSensitive, rt.handleClearStorageDefault))

	mux.HandleFunc("GET /api/v1/storage/buckets", rt.requireAbility(AbilityRead, rt.handleListStorageBuckets))
	mux.HandleFunc("POST /api/v1/storage/buckets", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateStorageBucket))
	mux.HandleFunc("GET /api/v1/storage/buckets/{id}", rt.requireAbility(AbilityRead, rt.handleGetStorageBucket))
	mux.HandleFunc("PATCH /api/v1/storage/buckets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateStorageBucket))
	mux.HandleFunc("DELETE /api/v1/storage/buckets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteStorageBucket))
	mux.HandleFunc("GET /api/v1/storage/buckets/{id}/usage", rt.requireAbility(AbilityRead, rt.handleBucketUsage))
	mux.HandleFunc("GET /api/v1/storage/buckets/{id}/objects", rt.requireAbility(AbilityReadSensitive, rt.handleListBucketObjects))
	mux.HandleFunc("PUT /api/v1/storage/buckets/{id}/objects", rt.requireAbility(AbilityWrite, rt.handleUploadBucketObject))
	mux.HandleFunc("DELETE /api/v1/storage/buckets/{id}/objects", rt.requireAbility(AbilityWrite, rt.handleDeleteBucketObject))
	mux.HandleFunc("GET /api/v1/storage/buckets/{id}/objects/download", rt.requireAbility(AbilityReadSensitive, rt.handleDownloadBucketObject))
	mux.HandleFunc("POST /api/v1/storage/buckets/{id}/presign", rt.requireAbility(AbilityReadSensitive, rt.handlePresignBucketObject))
	mux.HandleFunc("POST /api/v1/storage/buckets/{id}/lifecycle/run", rt.requireAbility(AbilityWrite, rt.handleSweepStorageBucket))
	mux.HandleFunc("GET /api/v1/apps/{name}/buckets", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppBuckets))
	mux.HandleFunc("PUT /api/v1/apps/{name}/buckets/{id}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleBindAppBucket))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/buckets/{id}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleUnbindAppBucket))
	// Unauthenticated by design: answers only for buckets whose public flag is on.
	mux.HandleFunc("GET "+publicBucketPathPrefix+"{id}/{key...}", rt.handlePublicBucketObject)

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

func randomLogArchiveID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate %s id: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}
