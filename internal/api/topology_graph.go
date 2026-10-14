package api

import (
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Topology node/edge kinds: the vocabulary GET /api/v1/projects/{id}/topology
// (topology.go) reports, and web/src/routes/projects/$id/topology.tsx
// renders. An "app" node is one store.DesiredService, the same unit
// GET /api/v1/apps already calls an app (appListResource); multi-service
// apps (store.App) show up as one node per member service, not one
// collapsed node, since depends_on and database bindings are declared
// per service, not per App row.
const (
	TopologyKindApp      = "app"
	TopologyKindDatabase = "database"
	TopologyKindVolume   = "volume"
	TopologyKindDomain   = "domain"
	TopologyKindLB       = "load_balancer"
	TopologyKindNode     = "node"

	TopologyEdgeDatabaseBinding = "database_binding"
	TopologyEdgeDependsOn       = "depends_on"
	TopologyEdgeVolume          = "volume_attachment"
	TopologyEdgeEgressAllow     = "egress_allow"
	TopologyEdgeRouting         = "routing"
	TopologyEdgeLoadBalances    = "load_balances"
	TopologyEdgePlacedOn        = "placed_on"
)

// topologyNode is one box in the diagram. Status is nil for a volume
// node: only app/database nodes have a reconciler condition to
// summarize.
type topologyNode struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	Label  string            `json:"label"`
	Status *appStatusSummary `json:"status,omitempty"`
}

// topologyEdge is one real, derived relationship between two
// topologyNode.ID values. Never fabricated: each Kind traces back to a
// specific store field, documented on buildTopologyGraph.
type topologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type topologyGraph struct {
	Nodes []topologyNode `json:"nodes"`
	Edges []topologyEdge `json:"edges"`
}

func appNodeID(serviceName string) string   { return "app:" + serviceName }
func dbNodeID(dbName string) string         { return "database:" + dbName }
func volumeNodeID(volumeName string) string { return "volume:" + volumeName }
func domainNodeID(host string) string       { return "domain:" + host }

// buildTopologyGraph derives a project's topology from its own
// project-scoped desired state: every edge traces to one real stored
// field (DatabaseEnv/DatabaseAttachment, DependsOn, Volumes,
// Egress.Allow matched against another app's Domains), never invented.
// See docs/service-topology-graph.md for what each edge kind means.
// appStatus/dbStatus key by service/database name, already summarized.
func buildTopologyGraph(services []store.DesiredService, databases []store.DesiredDatabase, appStatus, dbStatus map[string]appStatusSummary) topologyGraph {
	g := topologyGraph{}

	serviceByName := make(map[string]store.DesiredService, len(services))
	domainOwner := make(map[string]string, len(services))
	for _, s := range services {
		serviceByName[s.Name] = s
		for _, d := range s.Domains {
			domainOwner[d] = s.Name
		}
	}

	for _, s := range services {
		node := topologyNode{ID: appNodeID(s.Name), Kind: TopologyKindApp, Label: s.Name}
		if st, ok := appStatus[s.Name]; ok {
			node.Status = &st
		}
		g.Nodes = append(g.Nodes, node)
	}
	for _, d := range databases {
		node := topologyNode{ID: dbNodeID(d.Name), Kind: TopologyKindDatabase, Label: d.Name}
		if st, ok := dbStatus[d.Name]; ok {
			node.Status = &st
		}
		g.Nodes = append(g.Nodes, node)
	}

	dbExists := make(map[string]bool, len(databases))
	for _, d := range databases {
		dbExists[d.Name] = true
	}

	volumeSeen := make(map[string]bool)
	domainSeen := make(map[string]bool)
	for _, s := range services {
		from := appNodeID(s.Name)

		for _, host := range s.Domains {
			if !domainSeen[host] {
				domainSeen[host] = true
				g.Nodes = append(g.Nodes, topologyNode{ID: domainNodeID(host), Kind: TopologyKindDomain, Label: host})
			}
			g.Edges = append(g.Edges, topologyEdge{From: domainNodeID(host), To: from, Kind: TopologyEdgeRouting})
		}

		for _, ref := range s.DatabaseEnv {
			if dbExists[ref.Database] {
				g.Edges = append(g.Edges, topologyEdge{From: from, To: dbNodeID(ref.Database), Kind: TopologyEdgeDatabaseBinding})
			}
		}
		if s.DatabaseAttachment != nil && dbExists[s.DatabaseAttachment.DatabaseName] {
			g.Edges = append(g.Edges, topologyEdge{From: from, To: dbNodeID(s.DatabaseAttachment.DatabaseName), Kind: TopologyEdgeDatabaseBinding})
		}

		for _, dep := range s.DependsOn {
			target := dep
			if s.AppID != "" {
				target = s.AppID + "-" + dep
			}
			if _, ok := serviceByName[target]; ok {
				g.Edges = append(g.Edges, topologyEdge{From: from, To: appNodeID(target), Kind: TopologyEdgeDependsOn})
			}
		}

		for _, v := range s.Volumes {
			if !volumeSeen[v.Name] {
				volumeSeen[v.Name] = true
				g.Nodes = append(g.Nodes, topologyNode{ID: volumeNodeID(v.Name), Kind: TopologyKindVolume, Label: v.Name})
			}
			g.Edges = append(g.Edges, topologyEdge{From: from, To: volumeNodeID(v.Name), Kind: TopologyEdgeVolume})
		}

		if s.Egress != nil {
			for _, allow := range s.Egress.Allow {
				owner, ok := domainOwner[allow.Host]
				if !ok || owner == s.Name {
					continue
				}
				g.Edges = append(g.Edges, topologyEdge{From: from, To: appNodeID(owner), Kind: TopologyEdgeEgressAllow})
			}
		}
	}

	sortTopologyGraph(&g)
	return g
}

func lbNodeID(service string) string  { return "load_balancer:" + service }
func nodeNodeID(nodeID string) string { return "node:" + nodeID }

// addInfraNodes adds a load balancer node per service that has one and a
// node per fleet node a service or database is placed on. An empty
// NodeID means the control plane's own node, like resolvePlacementNode.
func addInfraNodes(g *topologyGraph, services []store.DesiredService, databases []store.DesiredDatabase, lbServices map[string]bool, nodes map[string]store.Node, localNodeID string) {
	placed := make(map[string]bool)
	place := func(from, rawNodeID string) {
		id := rawNodeID
		if id == "" {
			id = localNodeID
		}
		n, ok := nodes[id]
		if !ok {
			return
		}
		if !placed[id] {
			placed[id] = true
			label := n.Name
			if label == "" {
				label = id
			}
			g.Nodes = append(g.Nodes, topologyNode{ID: nodeNodeID(id), Kind: TopologyKindNode, Label: label})
		}
		g.Edges = append(g.Edges, topologyEdge{From: from, To: nodeNodeID(id), Kind: TopologyEdgePlacedOn})
	}
	for _, s := range services {
		if lbServices[s.Name] {
			g.Nodes = append(g.Nodes, topologyNode{ID: lbNodeID(s.Name), Kind: TopologyKindLB, Label: s.Name})
			g.Edges = append(g.Edges, topologyEdge{From: lbNodeID(s.Name), To: appNodeID(s.Name), Kind: TopologyEdgeLoadBalances})
		}
		place(appNodeID(s.Name), s.NodeID)
	}
	for _, d := range databases {
		place(dbNodeID(d.Name), d.NodeID)
	}
	sortTopologyGraph(g)
}

// sortTopologyGraph makes output order deterministic regardless of
// map/slice iteration order upstream, so the same desired state always
// serializes identically: a precondition for both a stable unit test
// and a stable diagram layout.
func sortTopologyGraph(g *topologyGraph) {
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Kind < b.Kind
	})
}
