package api

import "net/http"

// Routes sit under /api/v1/cloud-accounts/{id}/aws. Reads are view, snapshot create is operate,
// snapshot delete is delete and only removes snapshots this control plane created.
func (rt *Router) registerCloudAWSOpsRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/ebs/volumes", rt.requireCloudAccess(db, "view", acct, rt.handleEBSVolumes))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/ebs/snapshots", rt.requireCloudAccess(db, "view", acct, rt.handleEBSSnapshots))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/ebs/snapshots", rt.requireCloudAccess(db, "operate", acct, rt.handleEBSSnapshotCreate))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/ebs/snapshots/{sid}", rt.requireCloudAccess(db, "delete", acct, rt.handleEBSSnapshotDelete))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cloudwatch/metrics", rt.requireCloudAccess(db, "view", acct, rt.handleCloudWatchMetrics))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cloudwatch/log-groups", rt.requireCloudAccess(db, "view", acct, rt.handleCloudWatchLogGroups))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cloudwatch/log-events", rt.requireCloudAccess(db, "view", acct, rt.handleCloudWatchLogEvents))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/idle", rt.requireCloudAccess(db, "view", acct, rt.handleAWSIdle))
}
