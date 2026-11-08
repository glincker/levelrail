package ingress

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAttachDomainLimits(t *testing.T) {
	routes := []ingress.ProxyRoute{
		{Hosts: []string{"a.test", "b.test"}, BackendDial: "x:1", MaxBodyBytes: 7 << 20},
		{Hosts: []string{"c.test"}, BackendDial: "y:1"},
	}
	rows := []store.DomainLimits{
		{Domain: "a.test", MaxBodyBytes: 2 << 20, ReadTimeoutSeconds: 30},
		{Domain: "c.test", WriteTimeoutSeconds: 45},
	}
	got := attachDomainLimits(routes, rows)
	if len(got) != 3 {
		t.Fatalf("routes = %d, want 3 (a split out, b kept, c)", len(got))
	}
	byHost := map[string]ingress.ProxyRoute{}
	for _, r := range got {
		byHost[r.Hosts[0]] = r
	}
	if a := byHost["a.test"]; a.MaxBodyBytes != 2<<20 || a.ReadTimeout != 30*time.Second || a.WriteTimeout != 0 {
		t.Errorf("a.test = %+v", a)
	}
	if b := byHost["b.test"]; b.MaxBodyBytes != 7<<20 || b.ReadTimeout != 0 {
		t.Errorf("b.test must keep the app cap and no timeouts: %+v", b)
	}
	if c := byHost["c.test"]; c.MaxBodyBytes != 0 || c.WriteTimeout != 45*time.Second {
		t.Errorf("c.test = %+v", c)
	}
}
