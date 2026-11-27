package ingress

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
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

func TestHostNetworkDialNeedsSettingAndHostMode(t *testing.T) {
	svc := store.DesiredService{Port: 8080, GrantedCaps: []string{dockerguard.HostNetworkGrantCap}}
	host := &docker.ContainerState{Running: true, Networks: []docker.ContainerNetwork{{Name: "host"}}}
	bridge := &docker.ContainerState{Running: true, Networks: []docker.ContainerNetwork{{Name: "bridge"}}}
	cases := []struct {
		name  string
		svc   store.DesiredService
		state *docker.ContainerState
		on    bool
		want  bool
	}{
		{"granted, on, host mode", svc, host, true, true},
		{"setting off", svc, host, false, false},
		{"bridge container", svc, bridge, true, false},
		{"no networks reported", svc, &docker.ContainerState{Running: true}, true, false},
		{"no grant", store.DesiredService{Port: 8080}, host, true, false},
	}
	for _, c := range cases {
		if _, ok := hostNetworkDial(c.svc, c.state, c.on); ok != c.want {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.want)
		}
	}
}
