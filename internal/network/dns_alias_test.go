package network

import (
	"net/netip"
	"testing"
)

func TestBuildRecordsAliasesResolveLikeService(t *testing.T) {
	nodes := []NodeInfo{{ID: "n1", Name: "one", Address: netip.MustParseAddr("10.9.0.2")}}
	set, err := BuildRecords("acme.internal", "n1", []Placement{
		{Service: "web", NodeID: "n1", Aliases: []string{"shop", "www.shop.acme.internal"}},
		{Service: "api", NodeID: "n1", Aliases: []string{"shop"}},
	}, nodes)
	if err != nil {
		t.Fatal(err)
	}
	r := NewResolver("acme.internal")
	r.SetRecords(set)
	for _, name := range []string{"web.acme.internal", "shop.acme.internal", "www.shop.acme.internal"} {
		if got, ok := r.Lookup(name); !ok || got != netip.MustParseAddr("10.9.0.2") {
			t.Fatalf("%s: %v %v", name, got, ok)
		}
	}
	if len(set.Unresolved) != 1 {
		t.Fatalf("the clashing alias should be reported once: %+v", set.Unresolved)
	}
}
