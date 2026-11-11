package application

import (
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToContainerSpec_AppliesOnlyStoredGrants(t *testing.T) {
	granted, err := toContainerSpec("a", &store.DesiredService{Name: "a", Image: "img:1", GrantedCaps: []string{"CHOWN"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(granted.CapAdd, []string{"CHOWN"}) {
		t.Fatalf("CapAdd = %v, want [CHOWN]", granted.CapAdd)
	}
	plain, err := toContainerSpec("b", &store.DesiredService{Name: "b", Image: "img:1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.CapAdd) != 0 {
		t.Fatalf("CapAdd = %v, want none", plain.CapAdd)
	}
	if desiredRuntimeOptionsHash(&store.DesiredService{GrantedCaps: []string{"CHOWN"}}) == desiredRuntimeOptionsHash(&store.DesiredService{}) {
		t.Fatal("a grant must change the identity hash so the container is recreated")
	}
}
