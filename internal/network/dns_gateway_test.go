package network

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func gwTestManager(port int, enabled *bool, bridges []GatewayBridge) *GatewayDNS {
	return NewGatewayDNS(GatewayDNSConfig{
		Enabled:  func() bool { return *enabled },
		Bridges:  func(context.Context) ([]GatewayBridge, error) { return bridges, nil },
		Port:     port,
		Resolver: NewResolver("node"), BindTimeout: 2 * time.Second,
	})
}

func TestGatewayDNSBindsAndClosesWithSetting(t *testing.T) {
	on := true
	gw := netip.MustParseAddr("127.0.0.1")
	g := gwTestManager(0, &on, []GatewayBridge{{Iface: "br-test", Gateway: gw, Subnet: netip.MustParsePrefix("127.0.0.0/8")}})
	defer g.Close()
	g.Reconcile(context.Background())
	if !g.Confirmed(gw) {
		t.Fatalf("listener not confirmed: %+v", g.Report())
	}
	g.Reconcile(context.Background())
	if n := len(g.Report().Listeners); n != 1 {
		t.Fatalf("reconcile not idempotent, %d listeners", n)
	}
	on = false
	g.Reconcile(context.Background())
	if g.Confirmed(gw) {
		t.Fatal("listener still confirmed after disable")
	}
}

func TestGatewayDNSPortTakenReportsCodeAndStaysUnconfirmed(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	port := pc.LocalAddr().(*net.UDPAddr).Port
	on := true
	gw := netip.MustParseAddr("127.0.0.1")
	g := gwTestManager(port, &on, []GatewayBridge{{Iface: "br-test", Gateway: gw}})
	defer g.Close()
	g.Reconcile(context.Background())
	if g.Confirmed(gw) {
		t.Fatal("must not confirm a gateway whose port is taken")
	}
	l := g.Report().Listeners
	if len(l) != 1 || l[0].Code != GatewayReasonInUse {
		t.Fatalf("want %s, got %+v", GatewayReasonInUse, l)
	}
}

func TestDNSServerAccessRefusesOutsiders(t *testing.T) {
	s := NewDNSServer(NewResolver("node"), nil)
	s.SetAccess(func(a netip.Addr) bool { return netip.MustParsePrefix("10.0.0.0/8").Contains(a) })
	if err := s.Start(context.Background(), "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	q := new(dns.Msg)
	q.SetQuestion("web.node.", dns.TypeA)
	r, _, err := new(dns.Client).Exchange(q, s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeRefused {
		t.Fatalf("outsider got rcode %d, want REFUSED", r.Rcode)
	}
	s.SetAccess(func(a netip.Addr) bool { return a.IsLoopback() })
	r, _, err = new(dns.Client).Exchange(q, s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeNameError {
		t.Fatalf("allowed source got rcode %d, want NXDOMAIN", r.Rcode)
	}
}
