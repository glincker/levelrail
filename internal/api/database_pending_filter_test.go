package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDropPendingDatabases(t *testing.T) {
	all := []store.DesiredDatabase{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	tests := []struct {
		name    string
		pending map[string]bool
		want    []string
	}{
		{"nil pending keeps all", nil, []string{"a", "b", "c"}},
		{"empty pending keeps all", map[string]bool{}, []string{"a", "b", "c"}},
		{"drops one", map[string]bool{"b": true}, []string{"a", "c"}},
		{"drops all", map[string]bool{"a": true, "b": true, "c": true}, nil},
		{"unknown name ignored", map[string]bool{"z": true}, []string{"a", "b", "c"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dropPendingDatabases(all, tc.pending)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d databases, want %d", len(got), len(tc.want))
			}
			for i, d := range got {
				if d.Name != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, d.Name, tc.want[i])
				}
			}
		})
	}
}
