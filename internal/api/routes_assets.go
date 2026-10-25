package api

import (
	"net/http"
)

// registerAssetRoutes wires /api/v1/assets (ADR 044). Metadata reads are AbilityRead, listing and presigned GET are
// AbilityReadSensitive, uploads and deletes AbilityWrite, and anything that connects, exposes or probes a bucket
// is AbilityWriteSensitive. Bucket routes honour IAM on asset_bucket:<id> and its project.
func (rt *Router) registerAssetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/assets/buckets", rt.requireAbility(AbilityRead, rt.handleListAssetBuckets))
	mux.HandleFunc("POST /api/v1/assets/buckets", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateAssetBucket))
	mux.HandleFunc("GET /api/v1/assets/buckets/{id}", rt.requireAssetBucket(AbilityRead, rt.handleGetAssetBucket))
	mux.HandleFunc("PATCH /api/v1/assets/buckets/{id}", rt.requireAssetBucket(AbilityWriteSensitive, rt.handleUpdateAssetBucket))
	mux.HandleFunc("DELETE /api/v1/assets/buckets/{id}", rt.requireAssetBucket(AbilityWriteSensitive, rt.handleDeleteAssetBucket))
	mux.HandleFunc("POST /api/v1/assets/buckets/{id}/probe", rt.requireAssetBucket(AbilityWriteSensitive, rt.handleProbeAssetBucket))
	mux.HandleFunc("GET /api/v1/assets/buckets/{id}/usage", rt.requireAssetBucket(AbilityRead, rt.handleAssetUsage))
	mux.HandleFunc("GET /api/v1/assets/buckets/{id}/assets", rt.requireAssetBucket(AbilityReadSensitive, rt.handleListAssets))
	mux.HandleFunc("GET /api/v1/assets/buckets/{id}/urls", rt.requireAssetBucket(AbilityReadSensitive, rt.handleAssetURLs))
	mux.HandleFunc("POST /api/v1/assets/buckets/{id}/uploads", rt.requireAssetBucket(AbilityWrite, rt.handleRequestAssetUpload))
	mux.HandleFunc("POST /api/v1/assets/buckets/{id}/uploads/confirm", rt.requireAssetBucket(AbilityWrite, rt.handleConfirmAssetUpload))
	mux.HandleFunc("PUT /api/v1/assets/buckets/{id}/objects", rt.requireAssetBucket(AbilityWrite, rt.handleUploadAsset))
	mux.HandleFunc("DELETE /api/v1/assets/buckets/{id}/objects", rt.requireAssetBucket(AbilityWrite, rt.handleDeleteAsset))
}
