package application

import "testing"

func TestMeshDNSForPrefersFuncAndKeepsStaticFallback(t *testing.T) {
	c := &Controller{meshDNSAddr: "172.17.0.1"}
	if got := c.meshDNSFor("n1"); got != "172.17.0.1" {
		t.Fatalf("static fallback lost: %q", got)
	}
	WithMeshDNSFunc(func(id string) string {
		if id == "remote" {
			return ""
		}
		return "172.18.0.1"
	})(c)
	if got := c.meshDNSFor("remote"); got != "" {
		t.Fatalf("remote node must get no DNS, got %q", got)
	}
	if got := c.meshDNSFor("local"); got != "172.18.0.1" {
		t.Fatalf("local node got %q", got)
	}
}
