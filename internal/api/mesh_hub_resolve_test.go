package api

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	meshnet "github.com/GLINCKER/levelrail/internal/network"
)

type fakeHubLookup struct {
	addrs []netip.Addr
	err   error
}

func (f *fakeHubLookup) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return f.addrs, f.err
}

func hubTestResolver(l *fakeHubLookup, now *time.Time) *meshnet.HubHostResolver {
	return meshnet.NewHubHostResolver(meshnet.HubHostConfig{
		Lookup: l,
		TTL:    func() time.Duration { return 5 * time.Minute },
		Now:    func() time.Time { return *now },
	})
}

func TestDoctorMeshHubHostnameResolves(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHubLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "server.example.com"
	rt.mesh = &fakeMeshStatus{}
	rt.meshHubResolver = hubTestResolver(l, &now)

	c := rt.doctorCheckMeshHubEndpoint(context.Background())
	if c.Status != doctorStatusOK || !strings.Contains(c.Message, "resolved server.example.com to 203.0.113.7") || !strings.Contains(c.Message, "5m0s") {
		t.Fatalf("status = %q, message = %q", c.Status, c.Message)
	}
	if rt.meshHubHostProblem(context.Background(), "server.example.com") != "" {
		t.Error("a resolvable hostname must be usable for enrolment")
	}
}

func TestDoctorMeshHubHostnamePrivateExplained(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHubLookup{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.5")}}
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "server.example.com"
	rt.mesh = &fakeMeshStatus{}
	rt.meshHubResolver = hubTestResolver(l, &now)

	c := rt.doctorCheckMeshHubEndpoint(context.Background())
	if c.Status != doctorStatusWarn || !strings.Contains(c.Message, "resolved to 10.0.0.5") || c.Fix == "" {
		t.Fatalf("status = %q, message = %q, fix = %q", c.Status, c.Message, c.Fix)
	}
}

func TestDoctorMeshHubHostnameFailureKeepsLastGoodAndGivesFix(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHubLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "server.example.com"
	rt.mesh = &fakeMeshStatus{}
	rt.meshHubResolver = hubTestResolver(l, &now)
	_ = rt.doctorCheckMeshHubEndpoint(context.Background())

	now = now.Add(time.Hour)
	l.addrs, l.err = nil, &net.DNSError{Err: "no such host", IsNotFound: true}
	c := rt.doctorCheckMeshHubEndpoint(context.Background())
	if c.Status != doctorStatusWarn || !strings.Contains(c.Message, "NXDOMAIN") || !strings.Contains(c.Message, "last good address 203.0.113.7") {
		t.Fatalf("status = %q, message = %q", c.Status, c.Message)
	}
	if !strings.Contains(c.Fix, "DNS record") || !strings.Contains(c.Fix, "Save the address as a setting") {
		t.Errorf("fix = %q, want the DNS and setting CTA", c.Fix)
	}
}

func TestMeshHubHostUsableIPLiteralNeedsNoResolver(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	if rt.meshHubHostProblem(context.Background(), "203.0.113.7") != "" {
		t.Error("an IP literal is always usable")
	}
	if rt.meshHubHostProblem(context.Background(), "server.example.com") == "" {
		t.Error("without a resolver a hostname keeps the old IP-only rule")
	}
}
