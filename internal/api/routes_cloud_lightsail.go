package api

import "net/http"

// Routes sit under /api/v1/cloud-accounts/{id}/aws/lightsail. Reads are view, power changes and
// snapshot create are operate, create is build, deletes need the delete ability and a typed confirm.
func (rt *Router) registerCloudLightsailRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/instances", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailInstances))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/instances/{name}", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailInstance))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/lightsail/instances", rt.requireCloudAccess(db, "build", acct, rt.handleLightsailCreate))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/lightsail/instances/{name}/start", rt.requireCloudAccess(db, "operate", acct, rt.handleLightsailStart))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/lightsail/instances/{name}/stop", rt.requireCloudAccess(db, "operate", acct, rt.handleLightsailStop))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/lightsail/instances/{name}/reboot", rt.requireCloudAccess(db, "operate", acct, rt.handleLightsailReboot))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/lightsail/instances/{name}", rt.requireCloudAccess(db, "delete", acct, rt.handleLightsailDelete))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/bundles", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailBundles))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/blueprints", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailBlueprints))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/regions", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailRegions))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/static-ips", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailStaticIPs))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/lightsail/snapshots", rt.requireCloudAccess(db, "view", acct, rt.handleLightsailSnapshots))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/lightsail/snapshots", rt.requireCloudAccess(db, "operate", acct, rt.handleLightsailSnapshotCreate))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/aws/lightsail/snapshots/{name}", rt.requireCloudAccess(db, "delete", acct, rt.handleLightsailSnapshotDelete))
}
