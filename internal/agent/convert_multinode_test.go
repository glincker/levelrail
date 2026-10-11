package agent

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestContainerSpecMultiNodeFieldsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   docker.ContainerSpec
	}{
		{name: "empty stays empty", in: docker.ContainerSpec{Name: "c"}},
		{name: "every multi-node field", in: docker.ContainerSpec{
			Name: "web-abc", ExtraHosts: []string{"main.lr:10.181.0.2"}, Labels: map[string]string{"k": "v"},
			Network: &docker.NetworkAttachment{Name: "lr-app-1", Alias: "web"}, Entrypoint: []string{"/bin/sh"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containerSpecFromPB(containerSpecToPB(tt.in))
			if !reflect.DeepEqual(got.ExtraHosts, tt.in.ExtraHosts) || !reflect.DeepEqual(got.Network, tt.in.Network) ||
				!reflect.DeepEqual(got.Entrypoint, tt.in.Entrypoint) || len(got.Labels) != len(tt.in.Labels) {
				t.Fatalf("round trip = %+v, want %+v", got, tt.in)
			}
		})
	}
}
