package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestGroupTopology(t *testing.T) {
	apps := func(nodeIDs ...string) []networkTopologyAppResource {
		var out []networkTopologyAppResource
		for i, id := range nodeIDs {
			out = append(out, networkTopologyAppResource{Name: string(rune('a' + i)), NodeID: id})
		}
		return out
	}
	dbs := func(nodeIDs ...string) []networkTopologyDatabaseResource {
		var out []networkTopologyDatabaseResource
		for i, id := range nodeIDs {
			out = append(out, networkTopologyDatabaseResource{Name: string(rune('x' + i)), NodeID: id})
		}
		return out
	}
	remote := store.Node{ID: "node_r", Name: "remote", Status: store.NodeStatusOnline, Schedulable: true}
	local := store.Node{ID: "node_l", Name: "local", Status: store.NodeStatusOnline, Schedulable: true}

	tests := []struct {
		name         string
		in           topologyGroupInput
		wantKinds    []string
		wantLocal    [2]int
		wantUnplaced [2]int
		wantStale    []string
	}{
		{
			name:      "mesh off, empty placement, no node rows",
			in:        topologyGroupInput{apps: apps("", ""), databases: dbs("")},
			wantKinds: []string{"local"}, wantLocal: [2]int{2, 1},
		},
		{
			name:      "mesh on, resources carry the local node id",
			in:        topologyGroupInput{nodes: []store.Node{local}, localNodeID: "node_l", apps: apps("", "node_l"), databases: dbs("node_l")},
			wantKinds: []string{"local"}, wantLocal: [2]int{2, 1},
		},
		{
			name:      "remote node gets its own group",
			in:        topologyGroupInput{nodes: []store.Node{local, remote}, localNodeID: "node_l", apps: apps("", "node_r"), databases: dbs("node_r")},
			wantKinds: []string{"local", "node"}, wantLocal: [2]int{1, 0},
		},
		{
			name:         "stale node id is unplaced, not local",
			in:           topologyGroupInput{nodes: []store.Node{local}, localNodeID: "node_l", apps: apps("node_gone", ""), databases: dbs("node_gone")},
			wantKinds:    []string{"local", "unplaced"},
			wantLocal:    [2]int{1, 0},
			wantUnplaced: [2]int{1, 1},
			wantStale:    []string{"node_gone"},
		},
		{
			name:         "mesh off with a non-empty placement is unplaced",
			in:           topologyGroupInput{apps: apps("node_old")},
			wantKinds:    []string{"local", "unplaced"},
			wantUnplaced: [2]int{1, 0},
			wantStale:    []string{"node_old"},
		},
		{
			name:      "empty remote node still listed",
			in:        topologyGroupInput{nodes: []store.Node{remote}},
			wantKinds: []string{"local", "node"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupTopology(tt.in)
			var kinds []string
			for _, g := range got {
				kinds = append(kinds, g.Kind)
			}
			if len(kinds) != len(tt.wantKinds) {
				t.Fatalf("kinds = %v, want %v", kinds, tt.wantKinds)
			}
			for i := range kinds {
				if kinds[i] != tt.wantKinds[i] {
					t.Fatalf("kinds = %v, want %v", kinds, tt.wantKinds)
				}
			}
			if l := got[0]; len(l.Apps) != tt.wantLocal[0] || len(l.Databases) != tt.wantLocal[1] {
				t.Errorf("local = %d apps %d dbs, want %v", len(l.Apps), len(l.Databases), tt.wantLocal)
			}
			if tt.wantStale != nil {
				u := got[len(got)-1]
				if len(u.Apps) != tt.wantUnplaced[0] || len(u.Databases) != tt.wantUnplaced[1] {
					t.Errorf("unplaced = %d apps %d dbs, want %v", len(u.Apps), len(u.Databases), tt.wantUnplaced)
				}
				if len(u.StaleNodeIDs) != 1 || u.StaleNodeIDs[0] != tt.wantStale[0] {
					t.Errorf("stale = %v, want %v", u.StaleNodeIDs, tt.wantStale)
				}
			}
		})
	}
}
