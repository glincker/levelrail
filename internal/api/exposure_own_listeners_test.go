package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/network"
)

func TestOwnListenersReadMeshConfig(t *testing.T) {
	rt := &Router{}
	if got := rt.ownListeners(context.Background()); got != nil {
		t.Fatalf("mesh off must declare nothing: %v", got)
	}
	t.Setenv(network.EnvDNSAddr, ":5999")
	rt.mesh = &fakeMeshStatus{status: network.Status{ListenPort: 51900}}
	got := rt.ownListeners(context.Background())
	want := []exposure.OwnListener{{Port: 51900, Protocol: "udp"}, {Port: 5999}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
}
