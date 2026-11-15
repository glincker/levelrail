package application

import (
	"reflect"
	"testing"
)

func TestMeshDNSForPrefersFuncAndKeepsStaticFallback(t *testing.T) {
	c := &Controller{meshDNSAddr: "172.17.0.1"}
	if got := c.meshDNSFor("n1"); !reflect.DeepEqual(got, []string{"172.17.0.1"}) {
		t.Fatalf("static fallback lost: %v", got)
	}
	WithMeshDNSFunc(func(id string) []string {
		if id == "remote" {
			return nil
		}
		return []string{"172.18.0.1", "10.0.0.2"}
	})(c)
	if got := c.meshDNSFor("remote"); len(got) != 0 {
		t.Fatalf("remote node must get no DNS, got %v", got)
	}
	got := prependMeshDNS(c.meshDNSFor("local"), []string{"10.0.0.2", "9.9.9.9"})
	if want := []string{"172.18.0.1", "10.0.0.2", "9.9.9.9"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
