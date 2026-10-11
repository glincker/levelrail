package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPlacementResolverResolve(t *testing.T) {
	nodes := []store.Node{
		{ID: "node_l", Name: "edge-1"},
		{ID: "node_r", Name: "worker-1"},
	}
	tests := []struct {
		name      string
		localID   string
		nodeID    string
		wantName  string
		wantLocal bool
	}{
		{"empty id, mesh off, uses hostname", "", "", "host-a", true},
		{"empty id, mesh on, uses local node row", "node_l", "", "edge-1", true},
		{"local node id", "node_l", "node_l", "edge-1", true},
		{"remote node", "node_l", "node_r", "worker-1", false},
		{"unknown remote node has no name", "node_l", "node_gone", "", false},
		{"mesh off, non-empty id is not local", "", "node_old", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newPlacementResolver(nodes, tc.localID, "host-a")
			name, local := r.resolve(tc.nodeID)
			if name != tc.wantName || local != tc.wantLocal {
				t.Fatalf("resolve(%q) = (%q, %v), want (%q, %v)", tc.nodeID, name, local, tc.wantName, tc.wantLocal)
			}
		})
	}
}

func TestPlacementResolverStamps(t *testing.T) {
	r := newPlacementResolver(nil, "", "host-a")
	app := appResource{}
	r.stampApp(&app)
	if app.NodeName != "host-a" || !app.NodeIsLocal {
		t.Fatalf("app = %+v", app)
	}
	db := databaseResource{NodeID: "gone"}
	r.stampDatabase(&db)
	if db.NodeName != "" || db.NodeIsLocal {
		t.Fatalf("db = %+v", db)
	}
}
