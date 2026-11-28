package api

import (
	"reflect"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestPublishedPortNotes(t *testing.T) {
	svc := func(bind string, ports ...store.PublishedPort) store.DesiredService {
		return store.DesiredService{BindAddress: bind, PublishedPorts: ports}
	}
	tests := []struct {
		name string
		in   []store.DesiredService
		want []string
	}{
		{"none", nil, []string{}},
		{"loopback hidden", []store.DesiredService{svc("127.0.0.1", store.PublishedPort{HostPort: 25565, ContainerPort: 25565})}, []string{}},
		{"per port private", []store.DesiredService{svc("0.0.0.0", store.PublishedPort{HostPort: 53, ContainerPort: 53, Protocol: "udp", Bind: "private"})}, []string{}},
		{"public udp and tcp sorted", []store.DesiredService{svc("0.0.0.0",
			store.PublishedPort{HostPort: 51820, ContainerPort: 51820, Protocol: "udp"},
			store.PublishedPort{HostPort: 25565, ContainerPort: 25565})}, []string{"25565/tcp", "51820/udp"}},
		{"per port public overrides", []store.DesiredService{svc("127.0.0.1", store.PublishedPort{HostPort: 7777, ContainerPort: 7777, Protocol: "udp", Bind: "public"})}, []string{"7777/udp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicHostPorts(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			if (firewallNotice(tt.in) != nil) != (len(tt.want) > 0) {
				t.Fatalf("notice presence mismatch for %v", tt.want)
			}
		})
	}
}

func TestWithoutShadowedPublishedPorts(t *testing.T) {
	in := []store.PublishedPort{
		{HostPort: 8080, ContainerPort: 80},
		{HostPort: 8080, ContainerPort: 80, Protocol: "udp"},
		{HostPort: 9000, ContainerPort: 9000},
	}
	got := withoutShadowedPublishedPorts(in, 8080)
	if len(got) != 2 || got[0].Protocol != "udp" || got[1].HostPort != 9000 {
		t.Fatalf("unexpected %v", got)
	}
}
