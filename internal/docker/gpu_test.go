package docker

import (
	"reflect"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestBuildHostConfig_ShmSize(t *testing.T) {
	if got := buildHostConfig(ContainerSpec{}, nat.PortMap{}).ShmSize; got != 0 {
		t.Errorf("default ShmSize = %d, want 0", got)
	}
	if got := buildHostConfig(ContainerSpec{ShmSizeBytes: 1 << 30}, nat.PortMap{}).ShmSize; got != 1<<30 {
		t.Errorf("ShmSize = %d, want %d", got, 1<<30)
	}
}

func TestBuildHostConfig_GPU(t *testing.T) {
	tests := []struct {
		name    string
		gpu     *GPURequest
		wantNil bool
		count   int
		ids     []string
	}{
		{name: "no gpu", gpu: nil, wantNil: true},
		{name: "all", gpu: &GPURequest{Count: -1}, count: -1},
		{name: "zero count defaults to all", gpu: &GPURequest{}, count: -1},
		{name: "count", gpu: &GPURequest{Count: 2}, count: 2},
		{name: "device ids win over count", gpu: &GPURequest{Count: 4, DeviceIDs: []string{"0", "1"}}, ids: []string{"0", "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := buildHostConfig(ContainerSpec{GPU: tt.gpu}, nat.PortMap{})
			if tt.wantNil {
				if len(hc.DeviceRequests) != 0 {
					t.Fatalf("DeviceRequests = %v, want none", hc.DeviceRequests)
				}
				return
			}
			if len(hc.DeviceRequests) != 1 {
				t.Fatalf("DeviceRequests len = %d, want 1", len(hc.DeviceRequests))
			}
			got := hc.DeviceRequests[0]
			if got.Driver != "nvidia" || !reflect.DeepEqual(got.Capabilities, [][]string{{"gpu"}}) {
				t.Errorf("driver/capabilities = %q/%v", got.Driver, got.Capabilities)
			}
			if got.Count != tt.count {
				t.Errorf("Count = %d, want %d", got.Count, tt.count)
			}
			if !reflect.DeepEqual(got.DeviceIDs, tt.ids) {
				t.Errorf("DeviceIDs = %v, want %v", got.DeviceIDs, tt.ids)
			}
		})
	}
}
