package compose

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToDesiredServicesGPU(t *testing.T) {
	const head = "services:\n  m:\n    image: x:1\n    deploy:\n      resources:\n        reservations:\n          devices:\n"
	tests := []struct {
		name    string
		devices string
		want    *store.ServiceGPU
		wantErr bool
	}{
		{"no deploy block", "", nil, false},
		{"unset count means all", "            - driver: nvidia\n              capabilities: [gpu]\n", &store.ServiceGPU{Count: -1}, false},
		{"count all", "            - driver: nvidia\n              count: all\n              capabilities: [gpu]\n", &store.ServiceGPU{Count: -1}, false},
		{"count 2", "            - driver: nvidia\n              count: 2\n              capabilities: [gpu]\n", &store.ServiceGPU{Count: 2}, false},
		{"device ids win over count", "            - driver: nvidia\n              count: 1\n              device_ids: ['0', '1']\n              capabilities: [gpu]\n", &store.ServiceGPU{DeviceIDs: []string{"0", "1"}}, false},
		{"non-nvidia driver ignored", "            - driver: amd\n              capabilities: [gpu]\n", nil, false},
		{"no gpu capability ignored", "            - driver: nvidia\n              capabilities: [utility]\n", nil, false},
		{"bad count", "            - driver: nvidia\n              count: many\n              capabilities: [gpu]\n", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "services:\n  m:\n    image: x:1\n"
			if tt.devices != "" {
				src = head + tt.devices
			}
			f, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, _, err := ToDesiredServices("app", f)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			var gpu *store.ServiceGPU
			if got[0].Resources != nil {
				gpu = got[0].Resources.GPU
			}
			if !reflect.DeepEqual(gpu, tt.want) {
				t.Fatalf("gpu = %+v, want %+v", gpu, tt.want)
			}
		})
	}
}
