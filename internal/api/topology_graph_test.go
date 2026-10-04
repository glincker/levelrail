package api

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestBuildTopologyGraph exercises every edge kind against one known
// set of services/databases, asserting the exact nodes and edges that
// should come out, not just counts: this is deterministic, pure logic
// (buildTopologyGraph's own doc comment lists what each edge kind
// traces back to), so it's tested precisely rather than loosely.
func TestBuildTopologyGraph(t *testing.T) {
	healthy := appStatusSummary{Label: "Healthy", Variant: "success"}

	services := []store.DesiredService{
		{
			Name:    "web",
			AppID:   "shop",
			Domains: []string{"shop.example.com"},
			DatabaseEnv: map[string]store.DatabaseEnvRef{
				"DATABASE_URL": {Database: "main", Field: "url"},
			},
			DependsOn: []string{"worker"},
			Volumes:   []store.ServiceVolume{{Name: "app-web-uploads", ContainerPath: "/data"}},
			Egress: &store.ServiceEgressPolicy{
				Mode: store.EgressModeAllowlist,
				Allow: []store.ServiceEgressAllow{
					{Host: "admin.example.com", Port: 443}, // resolves to another app
					{Host: "api.stripe.com", Port: 443},    // external, no matching node
				},
			},
		},
		{
			Name:               "shop-worker",
			AppID:              "shop",
			DatabaseAttachment: &store.DatabaseAttachment{DatabaseName: "main", EnvVar: "DATABASE_URL", Field: "url"},
			Volumes:            []store.ServiceVolume{{Name: "app-web-uploads", ContainerPath: "/data"}},
		},
		{
			Name:    "admin",
			Domains: []string{"admin.example.com"},
		},
		{
			// DependsOn with no AppID and no matching sibling: must not
			// produce a dangling edge.
			Name:      "orphan",
			DependsOn: []string{"nothing-here"},
		},
	}
	databases := []store.DesiredDatabase{{Name: "main", Engine: store.EnginePostgres}}

	appStatus := map[string]appStatusSummary{"web": healthy}
	dbStatus := map[string]appStatusSummary{"main": healthy}

	got := buildTopologyGraph(services, databases, appStatus, dbStatus)

	wantNodes := []topologyNode{
		{ID: "app:admin", Kind: TopologyKindApp, Label: "admin"},
		{ID: "app:orphan", Kind: TopologyKindApp, Label: "orphan"},
		{ID: "app:shop-worker", Kind: TopologyKindApp, Label: "shop-worker"},
		{ID: "app:web", Kind: TopologyKindApp, Label: "web", Status: &healthy},
		{ID: "database:main", Kind: TopologyKindDatabase, Label: "main", Status: &healthy},
		{ID: "volume:app-web-uploads", Kind: TopologyKindVolume, Label: "app-web-uploads"},
	}
	if !reflect.DeepEqual(got.Nodes, wantNodes) {
		t.Fatalf("Nodes = %+v, want %+v", got.Nodes, wantNodes)
	}

	wantEdges := []topologyEdge{
		{From: "app:shop-worker", To: "database:main", Kind: TopologyEdgeDatabaseBinding},
		{From: "app:shop-worker", To: "volume:app-web-uploads", Kind: TopologyEdgeVolume},
		{From: "app:web", To: "app:admin", Kind: TopologyEdgeEgressAllow},
		{From: "app:web", To: "app:shop-worker", Kind: TopologyEdgeDependsOn},
		{From: "app:web", To: "database:main", Kind: TopologyEdgeDatabaseBinding},
		{From: "app:web", To: "volume:app-web-uploads", Kind: TopologyEdgeVolume},
	}
	if !reflect.DeepEqual(got.Edges, wantEdges) {
		t.Fatalf("Edges = %+v, want %+v", got.Edges, wantEdges)
	}
}

// TestBuildTopologyGraph_Empty guards the zero-apps, zero-databases
// case: nodes/edges should be empty slices (via nil, encoded as [] by
// writeJSON), never panic on empty maps.
func TestBuildTopologyGraph_Empty(t *testing.T) {
	got := buildTopologyGraph(nil, nil, nil, nil)
	if len(got.Nodes) != 0 || len(got.Edges) != 0 {
		t.Fatalf("got = %+v, want empty graph", got)
	}
}
