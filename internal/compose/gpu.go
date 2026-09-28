package compose

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Deploy is the deploy: block; only resources.reservations.devices is read.
type Deploy struct {
	Resources struct {
		Reservations struct {
			Devices []Device `yaml:"devices"`
		} `yaml:"reservations"`
	} `yaml:"resources"`
}

// Device is one deploy.resources.reservations.devices entry.
type Device struct {
	Driver       string           `yaml:"driver"`
	Count        yamlScalarString `yaml:"count"`
	DeviceIDs    []string         `yaml:"device_ids"`
	Capabilities []string         `yaml:"capabilities"`
}

// gpuFromDeploy maps an NVIDIA GPU device reservation onto a ServiceGPU.
// An unset count means every GPU, per the Compose spec.
func gpuFromDeploy(d *Deploy) (*store.ServiceGPU, error) {
	if d == nil {
		return nil, nil
	}
	for _, dev := range d.Resources.Reservations.Devices {
		if !wantsGPU(dev) {
			continue
		}
		if len(dev.DeviceIDs) > 0 {
			return &store.ServiceGPU{DeviceIDs: dev.DeviceIDs}, nil
		}
		count := strings.TrimSpace(string(dev.Count))
		if count == "" || count == "all" {
			return &store.ServiceGPU{Count: -1}, nil
		}
		n, err := strconv.Atoi(count)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("deploy.resources.reservations.devices.count: want a positive integer or all, got %q", count)
		}
		return &store.ServiceGPU{Count: n}, nil
	}
	return nil, nil
}

func wantsGPU(dev Device) bool {
	if dev.Driver != "" && dev.Driver != "nvidia" {
		return false
	}
	for _, c := range dev.Capabilities {
		if c == "gpu" {
			return true
		}
	}
	return false
}
