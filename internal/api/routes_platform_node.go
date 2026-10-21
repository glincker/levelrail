package api

import "net/http"

// registerNodeRoutes registers the nodes, mesh, provisioning and node providers routes.
func (rt *Router) registerNodeRoutes(mux *http.ServeMux) {
	// Nodes: fleet-level infrastructure, not scoped to any
	// one app, so every route here requires AbilityRoot specifically
	// rather than AbilityRead/AbilityWrite: minting a join token or
	// removing a node is a materially more sensitive operation than
	// editing one app's config, the same reasoning secrets.go's
	// AbilityWriteSensitive already applies one level down at the
	// per-app layer.
	mux.HandleFunc("GET /api/v1/nodes", rt.requireAbility(AbilityRoot, rt.handleListNodes))
	mux.HandleFunc("GET /api/v1/nodes/{id}", rt.requireAbility(AbilityRoot, rt.handleGetNode))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteNode))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/workloads", rt.requireAbility(AbilityRoot, rt.handleSetNodeWorkloads))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/region", rt.requireAbility(AbilityRoot, rt.handleSetNodeRegion))
	mux.HandleFunc("POST /api/v1/nodes/join-tokens", rt.requireAbility(AbilityRoot, rt.handleCreateNodeJoinToken))
	mux.HandleFunc("GET /api/v1/nodes/join-tokens", rt.requireAbility(AbilityRoot, rt.handleListPendingNodeJoinTokens))
	mux.HandleFunc("DELETE /api/v1/nodes/join-tokens", rt.requireAbility(AbilityRoot, rt.handleRevokeNodeJoinTokens))
	mux.HandleFunc("DELETE /api/v1/nodes/join-tokens/{id}", rt.requireAbility(AbilityRoot, rt.handleRevokeNodeJoinToken))
	// Health, cordon, drain, same AbilityRoot boundary as
	// every other node route above.
	mux.HandleFunc("GET /api/v1/nodes/{id}/health", rt.requireAbility(AbilityRoot, rt.handleGetNodeHealth))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cordon", rt.requireAbility(AbilityRoot, rt.handleCordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/uncordon", rt.requireAbility(AbilityRoot, rt.handleUncordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/drain", rt.requireAbility(AbilityRoot, rt.handleDrainNode))
	// Mesh status and key rotation, same AbilityRoot boundary: WireGuard
	// peer/handshake data and a node's own key material are fleet
	// infrastructure, not app-scoped, matching every other node route
	// above.
	mux.HandleFunc("GET /api/v1/mesh", rt.requireAbility(AbilityRoot, rt.handleGetMeshStatus))
	mux.HandleFunc("POST /api/v1/nodes/{id}/mesh/rotate-key", rt.requireAbility(AbilityRoot, rt.handleRotateNodeMeshKey))
	mux.HandleFunc("POST /api/v1/nodes/{id}/mesh/rejoin", rt.requireAbility(AbilityRoot, rt.handleRejoinNodeMesh))
	// Network topology: a read-only, whole-mesh summary (nodes, apps,
	// databases, load balancers, app-to-database connections). Unlike
	// every other node/mesh route above, AbilityRead rather than
	// AbilityRoot: it exposes no key material, no join tokens, and no
	// mutation, just the same placement/DNS-name facts already visible
	// piecemeal across GET /apps, /databases and /nodes.
	mux.HandleFunc("GET /api/v1/network/topology", rt.requireAbility(AbilityRead, rt.handleGetNetworkTopology))
	// Proxy/traffic: per-domain reachability (CrossNodeIngress, see
	// doctor_cross_node_ingress.go) plus TLS status, joined for the
	// Traffic/Proxy dashboard page. Same AbilityRead boundary as
	// topology above, same reasoning: read-only, no key material.
	mux.HandleFunc("GET /api/v1/network/proxy", rt.requireAbility(AbilityRead, rt.handleGetNetworkProxy))
	// Provider connections: seeing status and catalogs is a plain read
	// (no credential value is ever returned); connecting, testing and
	// disconnecting is write:sensitive so a team can be granted exactly
	// that. Creating VMs and join tokens stays AbilityRoot below.
	mux.HandleFunc("GET /api/v1/node-providers", rt.requireAbility(AbilityRead, rt.handleListNodeProviders))
	mux.HandleFunc("POST /api/v1/node-providers", rt.requireAbility(AbilityWriteSensitive, rt.handleSetNodeProviderCredential))
	mux.HandleFunc("POST /api/v1/node-providers/{provider}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestNodeProvider))
	mux.HandleFunc("DELETE /api/v1/node-providers/{provider}", rt.requireAbility(AbilityWriteSensitive, rt.handleDisconnectNodeProvider))
	mux.HandleFunc("GET /api/v1/node-providers/{provider}/regions", rt.requireAbility(AbilityRead, rt.handleListNodeProviderRegions))
	mux.HandleFunc("GET /api/v1/node-providers/{provider}/sizes", rt.requireAbility(AbilityRead, rt.handleListNodeProviderSizes))
	mux.HandleFunc("POST /api/v1/nodes/provision", rt.requireAbility(AbilityRoot, rt.handleCreateNodeProvision))
	// A top-level sibling of /api/v1/nodes/{id}/..., not nested under it:
	// "/api/v1/nodes/provisions/{id}" and "/api/v1/nodes/{id}/health"
	// would be two equally-specific wildcard patterns net/http's own
	// ServeMux refuses to register together.
	mux.HandleFunc("GET /api/v1/node-provisions", rt.requireAbility(AbilityRoot, rt.handleListNodeProvisions))
	mux.HandleFunc("GET /api/v1/node-provisions/{id}", rt.requireAbility(AbilityRoot, rt.handleGetNodeProvision))
	// SSH node provisioning: AbilityRoot like every node route (an SSH credential is as sensitive as a join token).
	// The show route stays a top-level sibling: nesting {id} under /nodes/ would conflict with /nodes/{id}/health.
	mux.HandleFunc("POST /api/v1/nodes/ssh-provision", rt.requireAbility(AbilityRoot, rt.handleCreateSSHNodeProvision))
	mux.HandleFunc("POST /api/v1/nodes/ssh-provision/preflight", rt.requireAbility(AbilityRoot, rt.handleSSHProvisionPreflight))
	mux.HandleFunc("GET /api/v1/ssh-node-provisions", rt.requireAbility(AbilityRoot, rt.handleListSSHNodeProvisions))
	mux.HandleFunc("GET /api/v1/ssh-node-provisions/{id}", rt.requireAbility(AbilityRoot, rt.handleGetSSHNodeProvision))
	// Node-level metrics (sum of per-container samples for everything
	// placed on this node, see handleQueryNodeMetrics's own doc comment
	// for exactly what that does and doesn't mean): same AbilityRoot
	// boundary and nil-telemetry 501 shape as every route above, and the
	// same query-param contract as GET /apps/{name}/metrics above it.
	mux.HandleFunc("GET /api/v1/nodes/{id}/metrics", rt.requireAbility(AbilityRoot, rt.handleQueryNodeMetrics))
	// Fleet-wide latest CPU/memory/disk snapshot plus rollup
	// (node_resource_usage.go), the node-scoped counterpart to
	// GET /apps/resource-usage above: registered as a literal path
	// segment under /nodes/, which Go's net/http mux matches ahead of
	// the /nodes/{id} wildcard above regardless of registration order,
	// the same precedent apps/resource-usage already relies on.
	mux.HandleFunc("GET /api/v1/nodes/resource-usage", rt.requireAbility(AbilityRoot, rt.handleFleetResourceUsage))
	// Latest OS-patch reading (internal/telemetry/hostpatch.go's
	// HostPatchCollector), a single current fact rather than a time
	// series, same AbilityRoot boundary as every other node route.
	mux.HandleFunc("GET /api/v1/nodes/{id}/patch-status", rt.requireAbility(AbilityRoot, rt.handleGetNodePatchStatus))
	mux.HandleFunc("GET /api/v1/nodes/{id}/events", rt.requireAbility(AbilityRoot, rt.handleListNodeEvents))
	// Rough disk/memory exhaustion projection (node_capacity_forecast.go),
	// a read-and-suggest layer over the same host samples
	// patch-status/metrics above read, same AbilityRoot boundary and
	// nil-telemetry 501 shape as every other node route.
	mux.HandleFunc("GET /api/v1/nodes/{id}/capacity-forecast", rt.requireAbility(AbilityRoot, rt.handleNodeCapacityForecast))
	mux.HandleFunc("GET /api/v1/nodes/{id}/capacity", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleGetNodeCapacity))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/guardrails", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handlePutNodeGuardrails))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cleanup", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleNodeCleanup))
	mux.HandleFunc("GET /api/v1/nodes/{id}/cleanup-runs", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleListNodeCleanupRuns))
	// Agent certificate lifecycle (node_cert.go, ADR 021).
	mux.HandleFunc("POST /api/v1/nodes/{id}/reenroll-token", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleCreateNodeReenrollToken))
	mux.HandleFunc("POST /api/v1/nodes/{id}/revoke-cert", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleRevokeNodeCert))
	// Trust panel (node_trust.go): what was verified and when.
	mux.HandleFunc("GET /api/v1/nodes/{id}/security", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleGetNodeTrust))
	mux.HandleFunc("GET /api/v1/nodes/{id}/security/events", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleListNodeSecurityEvents))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cert/renew", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleRenewNodeCert))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cert/drill", rt.requireAbilityForResource(AbilityRoot, nodeResourceFromPath, rt.handleNodeCertDrill))
	rt.registerMultiNodeRoutes(mux)
}
