package api

import "net/http"

// Routes sit under /api/v1/cloud-accounts/{id}/aws/rds. Reads are view, snapshot/stop/start/adopt are operate,
// create, snapshot delete and the database handoff are root only.
func (rt *Router) registerCloudRDSRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/rds/instances", rt.requireCloudAccess(db, "view", acct, rt.handleRDSList))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/rds/clusters", rt.requireCloudAccess(db, "view", acct, rt.handleRDSClusters))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/rds/estimate", rt.requireCloudAccess(db, "view", acct, rt.handleRDSEstimate))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/rds/snapshots", rt.requireCloudAccess(db, "view", acct, rt.handleRDSSnapshots))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/rds/snapshots/{sid}", rt.requireCloudAccess(db, "delete", acct, rt.handleRDSSnapshotDelete))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/rds/clusters/{cid}/actions", rt.requireCloudAccess(db, "operate", acct, rt.handleRDSClusterAction))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/rds/cluster-snapshots", rt.requireCloudAccess(db, "view", acct, rt.handleRDSClusterSnapshots))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/rds/cluster-snapshots/{sid}", rt.requireCloudAccess(db, "delete", acct, rt.handleRDSClusterSnapshotDelete))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/rds/instances/{rid}", rt.requireCloudAccess(db, "view", acct, rt.handleRDSGet))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/rds/instances/{rid}/actions", rt.requireCloudAccess(db, "operate", acct, rt.handleRDSAction))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/rds/instances/{rid}/connect-as-database", rt.requireCloudAccess(db, "build", acct, rt.handleRDSConnectAsDatabase))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/rds/instances", rt.requireCloudAccess(db, "build", acct, rt.handleRDSCreate))
}
