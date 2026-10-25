package main

// Port 53 is required: Docker rejects a non-53 nameserver address at container start.

import (
	"context"
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TestContainerDNSAddr_EarlyReturns checks every branch that returns "" without touching the Docker client.
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

// TestContainerDNSAddr_Live_Port53ResolvesGateway checks that a port-53 mesh DNS server yields the bridge gateway IP.
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

func TestMeshHubHostNeedsIP(t *testing.T) {
	for host, want := range map[string]bool{
		"":               false,
		"localhost":      false,
		"10.77.0.10":     false,
		"2001:db8::1":    false,
		"cp.example.com": true,
		"lrqa-cp":        true,
	} {
		if got := meshHubHostNeedsIP(host); got != want {
			t.Errorf("meshHubHostNeedsIP(%q) = %v, want %v", host, got, want)
		}
	}
}
