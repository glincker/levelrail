package ingress

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestDialPicksServicePortNotFirstPublished(t *testing.T) {
	state := &docker.ContainerState{Running: true, Ports: []docker.PortBinding{
		{ContainerPort: 22, HostPort: 2222, Protocol: "tcp"},
		{ContainerPort: 3000, HostPort: 32768, Protocol: "tcp"},
	}}
	if got, ok := dialAddr(state, 3000); !ok || got != "127.0.0.1:32768" {
		t.Fatalf("dialAddr = %q %v, want the 3000 binding", got, ok)
	}
	if got, running, _ := upstreamDial(state, 3000, "10.0.0.2", true); !running || got != "10.0.0.2:32768" {
		t.Fatalf("upstreamDial = %q %v, want the 3000 binding", got, running)
	}
	if p, ok := primaryOf(state, 3000); !ok || p.HostPort != 32768 {
		t.Fatalf("primaryOf = %+v %v, want the 3000 binding", p, ok)
	}
}
