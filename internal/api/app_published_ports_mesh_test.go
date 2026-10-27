package api

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// The mesh reserves the UDP port its device really bound, not the default.
func TestPublishedPorts_ReservesConfiguredMeshPort(t *testing.T) {
	stubUDPListeners(t)
	rt, _ := newTestRouter(t)
	rt.mesh = &fakeMeshStatus{status: network.Status{ListenPort: 51900}}
	udp := func(port int) []store.PublishedPort {
		return []store.PublishedPort{{ContainerPort: port, HostPort: port, Protocol: spec.PortProtocolUDP}}
	}
	ctx := context.Background()
	if _, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "vpn", PublishedPorts: udp(51900)}); err == nil || !strings.Contains(err.Error(), "mesh WireGuard") {
		t.Fatalf("configured mesh port: error = %v, want reserved", err)
	}
	release, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "vpn", PublishedPorts: udp(network.DefaultListenPort)})
	if err != nil {
		t.Fatalf("default port while the mesh uses another: error = %v", err)
	}
	release()
}
