package api

import "net/http"

const k8sClustersPath = "/api/v1/clusters"

// Every cluster route is root only, like node provisioning, and gated by k8s.clusters_enabled.
func (rt *Router) registerK8sRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+k8sClustersPath, rt.requireAbility(AbilityRoot, rt.handleListK8sClusters))
	mux.HandleFunc("POST "+k8sClustersPath, rt.requireAbility(AbilityRoot, rt.handleConnectK8sCluster))
	mux.HandleFunc("GET "+k8sClustersPath+"/{name}", rt.requireAbility(AbilityRoot, rt.handleGetK8sCluster))
	mux.HandleFunc("DELETE "+k8sClustersPath+"/{name}", rt.requireAbility(AbilityRoot, rt.handleDisconnectK8sCluster))
	mux.HandleFunc("POST "+k8sClustersPath+"/{name}/test", rt.requireAbility(AbilityRoot, rt.handleTestK8sCluster))
}
