package agent

// This file: focused tests for convert.go's pure Go<->proto mapping,
// covering the field this task added (docker.ContainerSpec.DNS <->
// agentpb.ContainerSpec.Dns) round-trip, table-driven per this
// codebase's own testing convention.

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

// TestContainerSpecFromPB_NilDoesNotPanic matches this file's every
// other from-PB conversion's own nil-safety contract (see
// containerStateFromPB, resourcesFromPB): a nil *agentpb.ContainerSpec
// must produce the zero value, not panic, since a caller can legitimately
// receive one over the wire (an unset oneof field, for instance).
func TestContainerSpecFromPB_NilDoesNotPanic(t *testing.T) {
	got := containerSpecFromPB(nil)
	if !reflect.DeepEqual(got, docker.ContainerSpec{}) {
		t.Errorf("containerSpecFromPB(nil) = %+v, want zero value", got)
	}
}
