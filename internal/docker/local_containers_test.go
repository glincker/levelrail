package docker

import (
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

func TestLocalFromInspect(t *testing.T) {
	tests := []struct {
		name string
		resp container.InspectResponse
		want LocalContainer
	}{
		{
			name: "coolify proxy",
			resp: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					Name:  "/coolify-proxy",
					State: &container.State{Running: true},
					HostConfig: &container.HostConfig{
						ExtraHosts:  []string{"host.docker.internal:host-gateway"},
						NetworkMode: "coolify",
					},
				},
				Config:          &container.Config{Image: "traefik:v3.6", Cmd: []string{"--entrypoints.http.address=:80"}},
				Mounts:          []container.MountPoint{{Source: "/data/coolify/proxy", Destination: "/traefik"}},
				NetworkSettings: coolifyNetworkSettings(),
			},
			want: LocalContainer{
				Name: "coolify-proxy", Image: "traefik:v3.6", Running: true,
				Networks:  []LocalNetwork{{Name: "coolify", IP: "10.0.1.5", Gateway: "10.0.1.1"}},
				Ports:     []int{80, 443},
				Published: []int{80, 443},
				Mappings:  []PortMapping{{Private: 80, Public: 80}, {Private: 443, Public: 443}},
				Mounts:    []ContainerMount{{Source: "/data/coolify/proxy", Destination: "/traefik"}},
				Args:      []string{"--entrypoints.http.address=:80"}, ExtraHosts: []string{"host.docker.internal:host-gateway"},
				NetworkMode: "coolify",
			},
		},
		{
			name: "no config falls back to path and args",
			resp: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{Name: "/p", Path: "traefik", Args: []string{"--x"}},
			},
			want: LocalContainer{Name: "p", Args: []string{"traefik", "--x"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localFromInspect(tt.resp); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestLocalFromSummaryMappings(t *testing.T) {
	s := container.Summary{
		Names: []string{"/nginx"}, Image: "nginx", State: "running",
		Ports:  []container.Port{{PrivatePort: 80, PublicPort: 8081}, {PrivatePort: 80, PublicPort: 8081}, {PrivatePort: 9000}},
		Mounts: []container.MountPoint{{Source: "/etc/nginx", Destination: "/etc/nginx"}},
	}
	s.HostConfig.NetworkMode = "host"
	got := localFromSummary(s)
	want := LocalContainer{
		Name: "nginx", Image: "nginx", Running: true, Ports: []int{80, 9000}, Published: []int{8081},
		Mappings: []PortMapping{{Private: 80, Public: 8081}}, Mounts: []ContainerMount{{Source: "/etc/nginx", Destination: "/etc/nginx"}},
		NetworkMode: "host",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func coolifyNetworkSettings() *container.NetworkSettings {
	ns := &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"coolify": {IPAddress: "10.0.1.5", Gateway: "10.0.1.1"}}}
	ns.Ports = nat.PortMap{
		"80/tcp":   {{HostIP: "0.0.0.0", HostPort: "80"}, {HostIP: "::", HostPort: "80"}},
		"443/tcp":  {{HostIP: "0.0.0.0", HostPort: "443"}},
		"8080/tcp": nil,
	}
	return ns
}
