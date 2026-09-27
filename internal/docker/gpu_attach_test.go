package docker

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
)

func TestCDIDeviceRequest(t *testing.T) {
	all := []string{"nvidia.com/gpu=all", "nvidia.com/gpu=0", "nvidia.com/gpu=1", "nvidia.com/gpu=GPU-abc"}
	tests := []struct {
		name       string
		g          GPURequest
		discovered []string
		wantIDs    []string
		wantOK     bool
	}{
		{"all gpus", GPURequest{Count: -1}, all, []string{"nvidia.com/gpu=all"}, true},
		{"zero count means all", GPURequest{}, all, []string{"nvidia.com/gpu=all"}, true},
		{"count takes the first indexes", GPURequest{Count: 2}, all, []string{"nvidia.com/gpu=0", "nvidia.com/gpu=1"}, true},
		{"device ids by index and uuid", GPURequest{DeviceIDs: []string{"1", "GPU-abc"}}, all, []string{"nvidia.com/gpu=1", "nvidia.com/gpu=GPU-abc"}, true},
		{"device ids win over count", GPURequest{Count: 4, DeviceIDs: []string{"0"}}, all, []string{"nvidia.com/gpu=0"}, true},
		{"more gpus than the spec lists falls back", GPURequest{Count: 3}, all, nil, false},
		{"unknown device falls back", GPURequest{DeviceIDs: []string{"7"}}, all, nil, false},
		{"no cdi devices falls back", GPURequest{Count: -1}, nil, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, ok := cdiDeviceRequest(tc.g, tc.discovered)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if req.Driver != "cdi" || !reflect.DeepEqual(req.DeviceIDs, tc.wantIDs) || req.Count != 0 || len(req.Capabilities) != 0 {
				t.Errorf("request = %+v, want cdi %v", req, tc.wantIDs)
			}
		})
	}
}

func TestAttachGPU(t *testing.T) {
	cached := func(devs ...string) *Client {
		c := &Client{}
		c.cdi.at, c.cdi.devices = time.Now(), devs
		return c
	}
	tests := []struct {
		name       string
		env        string
		client     *Client
		wantDriver string
	}{
		{"cdi devices present uses cdi", "", cached("nvidia.com/gpu=all"), "cdi"},
		{"auto spelled out", "auto", cached("nvidia.com/gpu=all"), "cdi"},
		{"no cdi devices keeps the legacy runtime", "", cached(), "nvidia"},
		{"legacy override ignores cdi", "legacy", cached("nvidia.com/gpu=all"), "nvidia"},
		{"legacy override is case insensitive", " Legacy ", cached("nvidia.com/gpu=all"), "nvidia"},
		{"unknown value means auto", "bogus", cached("nvidia.com/gpu=all"), "cdi"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvGPUAttach, tc.env)
			g := GPURequest{Count: -1}
			hc := buildHostConfig(ContainerSpec{GPU: &g}, nat.PortMap{})
			tc.client.attachGPU(context.Background(), hc, g)
			if len(hc.DeviceRequests) != 1 || hc.DeviceRequests[0].Driver != tc.wantDriver {
				t.Fatalf("device requests = %+v, want driver %s", hc.DeviceRequests, tc.wantDriver)
			}
		})
	}
}

func TestAttachGPUFallsBackWhenSpecIsIncomplete(t *testing.T) {
	c := &Client{}
	c.cdi.at, c.cdi.devices = time.Now(), []string{"nvidia.com/gpu=0"}
	g := GPURequest{Count: 2}
	hc := buildHostConfig(ContainerSpec{GPU: &g}, nat.PortMap{})
	c.attachGPU(context.Background(), hc, g)
	if hc.DeviceRequests[0].Driver != "nvidia" || hc.DeviceRequests[0].Count != 2 {
		t.Fatalf("a CDI spec missing GPU 1 must not be used: %+v", hc.DeviceRequests)
	}
}
