package api

import "net/http"

// Routes sit under /api/v1/cloud-accounts/{id}/aws/elasticache. Reads are view, reboot and snapshot
// create are operate, delete needs the delete ability and a typed confirm.
func (rt *Router) registerCloudElastiCacheRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/elasticache/replication-groups", rt.requireCloudAccess(db, "view", acct, rt.handleElastiCacheGroups))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/elasticache/replication-groups/{rid}", rt.requireCloudAccess(db, "view", acct, rt.handleElastiCacheGroup))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/elasticache/replication-groups/{rid}", rt.requireCloudAccess(db, "delete", acct, rt.handleElastiCacheGroupDelete))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/elasticache/clusters", rt.requireCloudAccess(db, "view", acct, rt.handleElastiCacheClusters))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/elasticache/clusters/{cid}", rt.requireCloudAccess(db, "view", acct, rt.handleElastiCacheCluster))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/elasticache/clusters/{cid}/reboot", rt.requireCloudAccess(db, "operate", acct, rt.handleElastiCacheReboot))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/elasticache/clusters/{cid}", rt.requireCloudAccess(db, "delete", acct, rt.handleElastiCacheClusterDelete))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/elasticache/snapshots", rt.requireCloudAccess(db, "view", acct, rt.handleElastiCacheSnapshots))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/elasticache/snapshots", rt.requireCloudAccess(db, "operate", acct, rt.handleElastiCacheSnapshotCreate))
}
