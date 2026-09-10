package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSelectLeastLoadedNode(t *testing.T) {
	tests := []struct {
		name       string
		candidates []nodePlacementLoad
		want       string
	}{
		{
			name:       "no candidates: local",
			candidates: nil,
			want:       "",
		},
		{
			name: "one candidate: selected regardless of load",
			candidates: []nodePlacementLoad{
				{NodeID: "node_a", Count: 5},
			},
			want: "node_a",
		},
		{
			name: "distinct loads: fewest wins",
			candidates: []nodePlacementLoad{
				{NodeID: "node_a", Count: 3},
				{NodeID: "node_b", Count: 1},
				{NodeID: "node_c", Count: 2},
			},
			want: "node_b",
		},
		{
			name: "tied load: lexicographically smallest id wins",
			candidates: []nodePlacementLoad{
				{NodeID: "node_z", Count: 0},
				{NodeID: "node_a", Count: 0},
				{NodeID: "node_m", Count: 0},
			},
			want: "node_a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectLeastLoadedNode(tt.candidates); got != tt.want {
				t.Errorf("selectLeastLoadedNode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWithAutoPlacement proves the option sets Router.autoPlacementEnabled,
// the same shape TestWithSessionTTL (auth_test.go) already establishes
// for its own option.
func TestWithAutoPlacement(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithAutoPlacement(false))

	if rt.autoPlacementEnabled {
		t.Error("autoPlacementEnabled = true, want false after WithAutoPlacement(false)")
	}
}

// seedOnlineNode is seedNode (nodes_test.go) plus status/schedulable
// control, since autoPlaceNode's own eligibility rule (schedulable and
// online) needs both dimensions exercised, not just presence in the
// nodes table.
func seedOnlineNode(t *testing.T, db *store.DB, id, name string, schedulable bool) {
	t.Helper()
	now := time.Now()
	if err := db.SaveNode(context.Background(), store.Node{
		ID: id, Name: name, Address: "10.0.0.1:9443", Status: store.NodeStatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed node %q: %v", name, err)
	}
	if !schedulable {
		if err := db.SetNodeSchedulable(context.Background(), id, false); err != nil {
			t.Fatalf("cordon node %q: %v", name, err)
		}
	}
}

func TestRouter_AutoPlaceNode(t *testing.T) {
	ctx := context.Background()

	t.Run("no nodes registered: local", func(t *testing.T) {
		rt, _ := newTestRouter(t)
		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "" {
			t.Errorf("autoPlaceNode() = %q, want local (\"\")", got)
		}
	})

	t.Run("disabled: local even with other nodes registered", func(t *testing.T) {
		rt, db := newTestRouter(t)
		rt.autoPlacementEnabled = false
		seedOnlineNode(t, db, "node_a", "alpha", true)
		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "" {
			t.Errorf("autoPlaceNode() = %q, want local (\"\")", got)
		}
	})

	t.Run("one cordoned node, none eligible: local", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedOnlineNode(t, db, "node_a", "alpha", false)
		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "" {
			t.Errorf("autoPlaceNode() = %q, want local (\"\")", got)
		}
	})

	t.Run("pending (not yet online) node excluded: local", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedNode(t, db, "node_a", "alpha") // seedNode defaults to NodeStatusPending
		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "" {
			t.Errorf("autoPlaceNode() = %q, want local (\"\")", got)
		}
	})

	t.Run("one eligible node, one cordoned: the eligible one wins", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedOnlineNode(t, db, "node_a", "alpha", true)
		seedOnlineNode(t, db, "node_b", "bravo", false)
		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "node_a" {
			t.Errorf("autoPlaceNode() = %q, want %q", got, "node_a")
		}
	})

	t.Run("picks the node with fewer placed resources", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedOnlineNode(t, db, "node_a", "alpha", true)
		seedOnlineNode(t, db, "node_b", "bravo", true)

		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "svc1", Image: "img:1", Port: 80}); err != nil {
			t.Fatalf("SaveDesiredService: %v", err)
		}
		if err := db.UpdateServiceNode(ctx, "svc1", "node_a"); err != nil {
			t.Fatalf("UpdateServiceNode: %v", err)
		}

		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "node_b" {
			t.Errorf("autoPlaceNode() = %q, want %q (fewer resources placed)", got, "node_b")
		}
	})

	t.Run("apps and databases both count toward load", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedOnlineNode(t, db, "node_a", "alpha", true)
		seedOnlineNode(t, db, "node_b", "bravo", true)

		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "db1", Engine: "postgres", Version: "16"}); err != nil {
			t.Fatalf("SaveDesiredDatabase: %v", err)
		}
		if err := db.UpdateDatabaseNode(ctx, "db1", "node_b"); err != nil {
			t.Fatalf("UpdateDatabaseNode: %v", err)
		}

		got, err := rt.autoPlaceNode(ctx)
		if err != nil {
			t.Fatalf("autoPlaceNode() error = %v", err)
		}
		if got != "node_a" {
			t.Errorf("autoPlaceNode() = %q, want %q (node_b already has a database)", got, "node_a")
		}
	})
}

func TestNodeIDKeyPresent(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "key omitted entirely", body: `{"name":"web"}`, want: false},
		{name: "key present, empty string", body: `{"name":"web","node_id":""}`, want: true},
		{name: "key present, real value", body: `{"name":"web","node_id":"node_a"}`, want: true},
		{name: "key present, explicit null (treated as explicit local)", body: `{"name":"web","node_id":null}`, want: true},
		{name: "malformed json", body: `{not json`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeIDKeyPresent([]byte(tt.body)); got != tt.want {
				t.Errorf("nodeIDKeyPresent(%s) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}
