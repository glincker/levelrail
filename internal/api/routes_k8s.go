package api

import "net/http"

const k8sClustersPath = "/api/v1/clusters"

// Every cluster route is root only, like node provisioning, and gated by k8s.clusters_enabled.
func (rt *Router) registerK8sRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/clusters", rt.requireAbility(AbilityRoot, rt.handleListK8sClusters))
	mux.HandleFunc("POST /api/v1/clusters", rt.requireAbility(AbilityRoot, rt.handleConnectK8sCluster))
	mux.HandleFunc("GET /api/v1/clusters/{name}", rt.requireAbility(AbilityRoot, rt.handleGetK8sCluster))
	mux.HandleFunc("DELETE /api/v1/clusters/{name}", rt.requireAbility(AbilityRoot, rt.handleDisconnectK8sCluster))
	mux.HandleFunc("POST /api/v1/clusters/{name}/test", rt.requireAbility(AbilityRoot, rt.handleTestK8sCluster))
	mux.HandleFunc("GET /api/v1/clusters/{name}/nodes", rt.requireAbility(AbilityRoot, rt.handleListK8sNodes))
	mux.HandleFunc("GET /api/v1/clusters/{name}/namespaces", rt.requireAbility(AbilityRoot, rt.handleListK8sNamespaces))
	mux.HandleFunc("GET /api/v1/clusters/{name}/deployments", rt.requireAbility(AbilityRoot, rt.handleListK8sDeployments))
	mux.HandleFunc("POST /api/v1/clusters/{name}/deployments", rt.requireAbility(AbilityRoot, rt.handleDeployK8sApp))
	mux.HandleFunc("GET /api/v1/clusters/{name}/deployments/{app}", rt.requireAbility(AbilityRoot, rt.handleGetK8sDeploymentStatus))
	mux.HandleFunc("GET /api/v1/clusters/{name}/deployments/{app}/logs", rt.requireAbility(AbilityRoot, rt.handleGetK8sDeploymentLogs))
	mux.HandleFunc("DELETE /api/v1/clusters/{name}/deployments/{app}", rt.requireAbility(AbilityRoot, rt.handleRemoveK8sDeployment))
}
