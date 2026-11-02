package application

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEphemeralPublishDrifted(t *testing.T) {
	p := store.PublishedPort{ContainerPort: 25565, Protocol: store.PortProtocolTCP, Bind: "private"}
	streams := map[int]bool{25565: true}
	cases := []struct {
		name  string
		ports []docker.PortBinding
		want  bool
	}{
		{"old container holding the stream port", []docker.PortBinding{{ContainerPort: 25565, HostPort: 25565, HostIP: "127.0.0.1"}}, true},
		{"ephemeral binding", []docker.PortBinding{{ContainerPort: 25565, HostPort: 32768, HostIP: "127.0.0.1"}}, false},
		{"no binding of the port", []docker.PortBinding{{ContainerPort: 80, HostPort: 32768}}, true},
	}
	for _, tc := range cases {
		if got := ephemeralPublishDrifted(tc.ports, p, streams); got != tc.want {
			t.Errorf("%s: drifted = %v, want %v", tc.name, got, tc.want)
		}
	}
}
