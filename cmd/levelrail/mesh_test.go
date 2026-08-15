package main

// Tests for containerDNSAddr (mesh.go): the function that decides
// whether a real, Docker-and-resolv.conf-usable nameserver address can
// be computed for the mesh DNS server right now. See
// dockerNameserverPort's own doc comment in mesh.go for the live-verified
// reason a non-53 bound port must never produce a usable address: Docker
// rejects "ip:port" DNS entries at container start, and no container
// resolver supports a non-standard nameserver port at all.

import (
	"context"
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TestContainerDNSAddr_EarlyReturns covers every branch that must return
// "" without ever touching client: mesh disabled, mesh enabled but the
// DNS server hasn't bound an address yet, and (the case this task's own
// live testing surfaced) a bound address on any port other than 53. A
// nil *docker.Client would panic if any of these branches reached
// BridgeGatewayIP, so passing nil doubles as a regression check that
// they don't.
func TestContainerDNSAddr_EarlyReturns(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	tests := []struct {
		name    string
		meshCfg *meshSetup
	}{
		{name: "mesh disabled: nil meshSetup", meshCfg: nil},
		{name: "mesh enabled but dns server never bound an address", meshCfg: &meshSetup{}},
		{
			name: "dns server bound, but not on port 53",
			meshCfg: &meshSetup{
				dnsAddr: netip.MustParseAddrPort("0.0.0.0:5390"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containerDNSAddr(context.Background(), nil, tt.meshCfg, logger)
			if got != "" {
				t.Errorf("containerDNSAddr() = %q, want \"\"", got)
			}
		})
	}
}

// TestContainerDNSAddr_Live_Port53ResolvesGateway proves the one branch
// TestContainerDNSAddr_EarlyReturns can't (it needs a real client): a
// mesh DNS server correctly bound to port 53 makes containerDNSAddr
// return the real bridge gateway IP, the same address BridgeGatewayIP
// itself returns directly, verified independently.
func TestContainerDNSAddr_Live_Port53ResolvesGateway(t *testing.T) {
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() {
		if cerr := client.Close(); cerr != nil {
			t.Errorf("closing client: %v", cerr)
		}
	})

	wantGateway, err := client.BridgeGatewayIP(context.Background())
	if err != nil {
		t.Fatalf("BridgeGatewayIP() error = %v", err)
	}

	meshCfg := &meshSetup{dnsAddr: netip.MustParseAddrPort("0.0.0.0:53")}
	got := containerDNSAddr(context.Background(), client, meshCfg, slog.New(slog.DiscardHandler))
	if got != wantGateway {
		t.Errorf("containerDNSAddr() = %q, want %q (BridgeGatewayIP's own answer)", got, wantGateway)
	}
}
