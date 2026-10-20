package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func seedListNodes(t *testing.T, db *DB, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("node-%04d", i)
		node := testNode(fmt.Sprintf("nd_%04d", i), name)
		node.Address = fmt.Sprintf("10.0.%d.%d:9443", i/256, i%256)
		if err := db.SaveNode(ctx, node); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
}

func TestListNodesFiltered(t *testing.T) {
	db := openTestDB(t)
	seedListNodes(t, db, 600)
	ctx := context.Background()
	tests := []struct {
		name      string
		f         NodeListFilter
		wantTotal int
		wantRows  int
	}{
		{"no filter unpaged", NodeListFilter{}, 600, 600},
		{"no filter paged", NodeListFilter{Limit: 50}, 600, 50},
		{"paged with offset", NodeListFilter{Limit: 10, Offset: 595}, 600, 5},
		{"query by name", NodeListFilter{Query: "node-059"}, 10, 10},
		{"query by address", NodeListFilter{Query: "10.0.2.3:9443"}, 1, 1},
		{"query underscore is literal", NodeListFilter{Query: "node_"}, 0, 0},
		{"query no match", NodeListFilter{Query: "nope"}, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			got, total, err := db.ListNodesFiltered(ctx, tc.f)
			if err != nil {
				t.Fatal(err)
			}
			if total != tc.wantTotal || len(got) != tc.wantRows {
				t.Errorf("total=%d rows=%d, want %d/%d", total, len(got), tc.wantTotal, tc.wantRows)
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Errorf("query took %s, over budget", d)
			}
		})
	}
}

func TestListNodesFiltered_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_b", "zebra")); err != nil {
		t.Fatalf("SaveNode(zebra) error = %v", err)
	}
	if err := db.SaveNode(ctx, testNode("node_a", "alpha")); err != nil {
		t.Fatalf("SaveNode(alpha) error = %v", err)
	}

	got, total, err := db.ListNodesFiltered(ctx, NodeListFilter{})
	if err != nil {
		t.Fatalf("ListNodesFiltered() error = %v", err)
	}
	if total != 2 || len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zebra" {
		t.Fatalf("ListNodesFiltered() = total=%d got=%+v, want 2/[alpha zebra] in that order", total, got)
	}
}

func TestListNodesFiltered_Empty(t *testing.T) {
	db := openTestDB(t)
	got, total, err := db.ListNodesFiltered(context.Background(), NodeListFilter{})
	if err != nil {
		t.Fatalf("ListNodesFiltered() error = %v", err)
	}
	if total != 0 || len(got) != 0 {
		t.Errorf("ListNodesFiltered() = total=%d rows=%d, want 0/0", total, len(got))
	}
}

// ListNodes (the plain, unpaged primitive) must stay byte-for-byte
// unchanged by the new filtered sibling: nothing that already calls it
// should ever observe a behavior difference.
func TestListNodes_UnaffectedByFilteredSibling(t *testing.T) {
	db := openTestDB(t)
	seedListNodes(t, db, 5)
	ctx := context.Background()

	got, err := db.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("ListNodes() = %d nodes, want 5", len(got))
	}
}
