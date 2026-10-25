package agent

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestContainerSpecDNSRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		dns  []string
	}{
		{name: "nil dns", dns: nil},
		{name: "single address", dns: []string{"172.17.0.1"}},
		{name: "multiple addresses preserve order", dns: []string{"172.17.0.1", "10.0.0.1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := docker.ContainerSpec{Name: "web", Image: "img:v1", DNS: tt.dns}

			pb := containerSpecToPB(original)
			if !reflect.DeepEqual(pb.Dns, tt.dns) {
				t.Errorf("containerSpecToPB().Dns = %+v, want %+v", pb.Dns, tt.dns)
			}

			back := containerSpecFromPB(pb)
			if !reflect.DeepEqual(back.DNS, tt.dns) {
				t.Errorf("containerSpecFromPB(containerSpecToPB(spec)).DNS = %+v, want %+v", back.DNS, tt.dns)
			}
		})
	}
}

func TestContainerSpecCapAddNetworkModeRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		capAdd      []string
		networkMode string
	}{
		{name: "neither set", capAdd: nil, networkMode: ""},
		{name: "net admin only", capAdd: []string{"NET_ADMIN"}, networkMode: ""},
		{name: "network mode only", capAdd: nil, networkMode: "container:abc123"},
		{name: "both set", capAdd: []string{"NET_ADMIN"}, networkMode: "container:abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := docker.ContainerSpec{Name: "web", Image: "img:v1", CapAdd: tt.capAdd, NetworkMode: tt.networkMode}

			pb := containerSpecToPB(original)
			if !reflect.DeepEqual(pb.CapAdd, tt.capAdd) {
				t.Errorf("containerSpecToPB().CapAdd = %+v, want %+v", pb.CapAdd, tt.capAdd)
			}
			if pb.NetworkMode != tt.networkMode {
				t.Errorf("containerSpecToPB().NetworkMode = %q, want %q", pb.NetworkMode, tt.networkMode)
			}

			back := containerSpecFromPB(pb)
			if !reflect.DeepEqual(back.CapAdd, tt.capAdd) {
				t.Errorf("containerSpecFromPB(containerSpecToPB(spec)).CapAdd = %+v, want %+v", back.CapAdd, tt.capAdd)
			}
			if back.NetworkMode != tt.networkMode {
				t.Errorf("containerSpecFromPB(containerSpecToPB(spec)).NetworkMode = %q, want %q", back.NetworkMode, tt.networkMode)
			}
		})
	}
}

func TestResourcesRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		res  *docker.Resources
	}{
		{name: "nil resources", res: nil},
		{name: "memory and cpu only", res: &docker.Resources{MemoryBytes: 512 * 1024 * 1024, NanoCPUs: 500_000_000}},
		{
			name: "swap and cpuset alongside memory and cpu",
			res: &docker.Resources{
				MemoryBytes:     512 * 1024 * 1024,
				NanoCPUs:        500_000_000,
				SwapMemoryBytes: 1024 * 1024 * 1024,
				CPUSetCPUs:      "0-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			back := resourcesFromPB(resourcesToPB(tt.res))
			if !reflect.DeepEqual(back, tt.res) {
				t.Errorf("resourcesFromPB(resourcesToPB(%+v)) = %+v, want %+v", tt.res, back, tt.res)
			}
		})
	}
}

// A nil ContainerSpec can arrive over the wire as an unset oneof, so it must yield the zero value.
func TestContainerSpecFromPB_NilDoesNotPanic(t *testing.T) {
	got := containerSpecFromPB(nil)
	if !reflect.DeepEqual(got, docker.ContainerSpec{}) {
		t.Errorf("containerSpecFromPB(nil) = %+v, want zero value", got)
	}
}

func TestContainerStateLabelsSurviveTheWire(t *testing.T) {
	in := &docker.ContainerState{ID: "c1", Name: "db", Running: true, Labels: map[string]string{"levelrail.tls-cert": "ecdsa"}}
	got := containerStateFromPB(containerStateToPB(in))
	if got.Labels["levelrail.tls-cert"] != "ecdsa" {
		t.Errorf("labels = %v, want them carried so a reconnect adopts the container", got.Labels)
	}
}
