package application

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/store"
)

func TestProbeTargetExecFallbackPort(t *testing.T) {
	exec := store.ServiceProbe{Exec: []string{"true"}}
	tests := []struct {
		name  string
		ports []docker.PortBinding
		port  int
		node  string
		want  string
	}{
		{name: "declared port", ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}, port: 80, want: "127.0.0.1:8080"},
		{name: "port 0 uses first published tcp port", ports: []docker.PortBinding{{ContainerPort: 53, HostPort: 5353, Protocol: "udp"}, {ContainerPort: 3000, HostPort: 3001, Protocol: "tcp"}}, want: "127.0.0.1:3001"},
		{name: "port 0 and nothing published has no fallback", want: ""},
		{name: "remote node has no fallback", ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}, port: 80, node: "n1", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{}
			got, err := c.probeTarget(context.Background(), &docker.ContainerState{ID: "c1", Ports: tt.ports}, exec, tt.port, tt.node)
			if err != nil {
				t.Fatal(err)
			}
			if got.FallbackAddr != tt.want {
				t.Errorf("FallbackAddr = %q, want %q", got.FallbackAddr, tt.want)
			}
		})
	}
}
