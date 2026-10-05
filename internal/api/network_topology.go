package api

// This file: GET /api/v1/network/topology, a single read-only summary of
// the whole mesh for the network topology UI page (the "VPC console"
// view: one flat mesh as the trust boundary, each node a zone). Every
// fact here already exists somewhere else in the API (GET /apps,
// /databases, /nodes, /loadbalancers); this handler's only job is joining
// them into one response shaped for a graph, not deriving anything new.
// Placement reuses exactly what internal/reconcile/mesh's own Controller
// reads (ListDesiredServices/ListDesiredDatabases' NodeID fields), and DNS
// naming reuses internal/network.Zone/BuildRecords' own "<service>.<zone>"
// convention, so a name shown here matches what a container's own
// connection string actually resolves.

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

// networkTopologyNodeResource is one fleet member.
type networkTopologyNodeResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Region      string `json:"region,omitempty"`
	Status      string `json:"status"`
	Schedulable bool   `json:"schedulable"`
	MeshAddress string `json:"mesh_address,omitempty"`
	IsLocal     bool   `json:"is_local"`
}

// networkTopologyAppResource is one desired service, joined with its
// placement's mesh identity. NodeID is the raw, possibly-empty
// desired_services.node_id (see store.DesiredService.NodeID's own doc
// comment for what empty means); Node is the resolved node ID a UI can
// key a zone lookup on directly, matching BuildRecords' own selfID
// resolution.
type networkTopologyAppResource struct {
	Name        string   `json:"name"`
	NodeID      string   `json:"node_id"`
	Domains     []string `json:"domains,omitempty"`
	DNSName     string   `json:"dns_name,omitempty"`
	MeshAddress string   `json:"mesh_address,omitempty"`
}

// networkTopologyDatabaseResource is one managed database, same shape as
// networkTopologyAppResource.
type networkTopologyDatabaseResource struct {
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	NodeID      string `json:"node_id"`
	DNSName     string `json:"dns_name,omitempty"`
	MeshAddress string `json:"mesh_address,omitempty"`
}

// networkTopologyLoadBalancerResource is one configured balancer. Unlike
// a conventional load balancer fronting many backends, this platform's own
// model (internal/loadbalancer, service_load_balancers) is one balancer
// per service, spreading traffic across that service's own replicas: see
// internal/api/loadbalancer_list.go's summarizeLoadBalancer for the same
// one-row-per-service shape. Service is the app/service it balances.
type networkTopologyLoadBalancerResource struct {
	Service   string `json:"service"`
	Algorithm string `json:"algorithm"`
}

// networkTopologyConnectionResource is one app-to-database link, resolved
// from whichever of DesiredService.DatabaseEnv or .DatabaseAttachment
// declared it (the same two sources internal/reconcile/application's
// resolveDatabaseEnv resolves at container-create time). This is read-only
// here: the connect-an-app-to-a-database feature (API/CLI/per-app UI) owns
// writing these, this handler only surfaces what already exists.
type networkTopologyConnectionResource struct {
	App      string `json:"app"`
	Database string `json:"database"`
	EnvVar   string `json:"env_var,omitempty"`
}

// networkTopologyResponse is GET /api/v1/network/topology's response body.
type networkTopologyResponse struct {
	// Zone is the internal DNS zone every DNSName below is a member of
	// (internal/network.Zone), e.g. "acme.internal".
	Zone          string                                `json:"zone"`
	MeshEnabled   bool                                  `json:"mesh_enabled"`
	Nodes         []networkTopologyNodeResource         `json:"nodes"`
	Apps          []networkTopologyAppResource          `json:"apps"`
	Databases     []networkTopologyDatabaseResource     `json:"databases"`
	LoadBalancers []networkTopologyLoadBalancerResource `json:"load_balancers"`
	Connections   []networkTopologyConnectionResource   `json:"connections"`
}

// handleGetNetworkTopology handles GET /api/v1/network/topology. See this
// file's own header for what it assembles and why nothing here is
// derived beyond simple joins.
func (rt *Router) handleGetNetworkTopology(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.internalError(w, "api: get network topology: list nodes failed", err)
		return
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.internalError(w, "api: get network topology: list services failed", err)
		return
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		rt.internalError(w, "api: get network topology: list databases failed", err)
		return
	}

	canSeeApp, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: get network topology: visibility", err)
		return
	}
	canSeeDB, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: get network topology: visibility", err)
		return
	}

	byID := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	zone := network.Zone(rt.brand.ShortName)

	res := networkTopologyResponse{
		Zone:        zone,
		MeshEnabled: rt.mesh != nil,
		Nodes:       toNetworkTopologyNodes(nodes, rt.localNodeID),
		Apps:        make([]networkTopologyAppResource, 0, len(services)),
		Databases:   make([]networkTopologyDatabaseResource, 0, len(databases)),
		Connections: []networkTopologyConnectionResource{},
	}

	for _, s := range services {
		if !canSeeApp(s.Name) {
			continue
		}
		node := resolvePlacementNode(s.NodeID, rt.localNodeID, byID)
		app := networkTopologyAppResource{
			Name:    s.Name,
			NodeID:  s.NodeID,
			Domains: s.Domains,
			DNSName: dnsNameFor(s.Name, zone),
		}
		if node != nil {
			app.MeshAddress = node.MeshAddress
		}
		res.Apps = append(res.Apps, app)
		res.Connections = append(res.Connections, databaseConnectionsFor(s)...)
	}

	for _, d := range databases {
		if !canSeeDB(d.Name) {
			continue
		}
		node := resolvePlacementNode(d.NodeID, rt.localNodeID, byID)
		db := networkTopologyDatabaseResource{
			Name:    d.Name,
			Engine:  d.Engine,
			NodeID:  d.NodeID,
			DNSName: dnsNameFor(d.Name, zone),
		}
		if node != nil {
			db.MeshAddress = node.MeshAddress
		}
		res.Databases = append(res.Databases, db)
	}

	if rt.lb.store != nil {
		lbRows, err := rt.lb.store.ListLoadBalancerRows(ctx)
		if err != nil {
			rt.internalError(w, "api: get network topology: list load balancers failed", err)
			return
		}
		res.LoadBalancers = toNetworkTopologyLoadBalancers(lbRows, rt.logger)
	}

	sort.Slice(res.Apps, func(i, j int) bool { return res.Apps[i].Name < res.Apps[j].Name })
	sort.Slice(res.Databases, func(i, j int) bool { return res.Databases[i].Name < res.Databases[j].Name })
	sort.Slice(res.Connections, func(i, j int) bool {
		if res.Connections[i].App != res.Connections[j].App {
			return res.Connections[i].App < res.Connections[j].App
		}
		return res.Connections[i].Database < res.Connections[j].Database
	})

	writeJSON(w, http.StatusOK, res)
}

func toNetworkTopologyNodes(nodes []store.Node, localNodeID string) []networkTopologyNodeResource {
	out := make([]networkTopologyNodeResource, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, networkTopologyNodeResource{
			ID:          n.ID,
			Name:        n.Name,
			Region:      n.Region,
			Status:      string(n.Status),
			Schedulable: n.Schedulable,
			MeshAddress: n.MeshAddress,
			IsLocal:     n.ID == localNodeID,
		})
	}
	return out
}

// resolvePlacementNode resolves a placement's raw node ID (which may be
// "", meaning the control plane's own node, per store.DesiredService.NodeID's
// own doc comment) to its store.Node row, the same resolution
// internal/network.BuildRecords does for the same reason. Returns nil when
// the resolved node isn't in the fleet at all, a real if unlikely state
// (a placement pointing at a since-deleted node) that must not panic.
func resolvePlacementNode(nodeID, localNodeID string, byID map[string]store.Node) *store.Node {
	id := nodeID
	if id == "" {
		id = localNodeID
	}
	if n, ok := byID[id]; ok {
		return &n
	}
	return nil
}

// dnsNameFor mirrors internal/network.BuildRecords' own
// "<service>.<zone>" naming exactly, without needing a live resolver: the
// topology view is meant to show what a connection string resolves to
// even when the mesh reconciler hasn't run a pass since this row last
// changed.
func dnsNameFor(service, zone string) string {
	if service == "" || zone == "" {
		return ""
	}
	return service + "." + zone
}

// databaseConnectionsFor collects one connection per database a service
// resolves an env var from, from both declaration sources
// (DesiredService.DatabaseEnv's own doc comment): app.yaml's `from:` env
// vars and the API/UI's single DatabaseAttachment. A service can have both
// at once (an app.yaml-deployed service later attached via the API), so
// this is a union, not an either/or.
func databaseConnectionsFor(s store.DesiredService) []networkTopologyConnectionResource {
	var out []networkTopologyConnectionResource
	for envVar, ref := range s.DatabaseEnv {
		out = append(out, networkTopologyConnectionResource{
			App: s.Name, Database: ref.Database, EnvVar: envVar,
		})
	}
	if s.DatabaseAttachment != nil {
		out = append(out, networkTopologyConnectionResource{
			App: s.Name, Database: s.DatabaseAttachment.DatabaseName, EnvVar: s.DatabaseAttachment.EnvVar,
		})
	}
	return out
}

func toNetworkTopologyLoadBalancers(rows []store.LoadBalancerRow, logger *slog.Logger) []networkTopologyLoadBalancerResource {
	out := make([]networkTopologyLoadBalancerResource, 0, len(rows))
	for _, row := range rows {
		var cfg loadbalancer.Config
		if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
			logger.Warn("api: get network topology: skip undecodable load balancer config",
				slog.String("service", row.Service), slog.String("error", err.Error()))
			continue
		}
		out = append(out, networkTopologyLoadBalancerResource{
			Service: row.Service, Algorithm: cfg.EffectiveAlgorithm(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out
}
