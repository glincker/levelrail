package compose

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
	"gopkg.in/yaml.v3"
)

// Deploy is the deploy: block; only resources.reservations.devices (GPU
// device reservations) and replicas are read. Every other subkey (mode,
// placement, update_config, rollback_config, restart_policy,
// endpoint_mode, labels, ...) is a Swarm-specific field with no meaning
// outside a Swarm cluster, so UnmarshalYAML records its presence in
// unsupported rather than letting it silently parse and vanish: see
// validateDeploy.
type Deploy struct {
	Resources struct {
		Reservations struct {
			Devices []Device `yaml:"devices"`
		} `yaml:"reservations"`
	} `yaml:"resources"`
	// Replicas maps onto the same store.DesiredService.Replicas /
	// spec.Service.Replicas field a service's own top-level replicas:
	// (app.yaml) already resolves through. Zero means unset.
	Replicas    int `yaml:"replicas"`
	unsupported []string
}

// deployAllowedKeys are the only deploy: subkeys this package reads.
var deployAllowedKeys = map[string]bool{
	"resources": true,
	"replicas":  true,
}

// UnmarshalYAML decodes the known resources:/replicas: shapes normally,
// then walks the raw mapping a second time to record every other
// top-level key present, so validateDeploy can report exactly which
// Swarm-specific fields a service declared instead of a generic "deploy
// not supported".
func (d *Deploy) UnmarshalYAML(node *yaml.Node) error {
	type rawDeploy Deploy
	if err := node.Decode((*rawDeploy)(d)); err != nil {
		return fmt.Errorf("deploy: %w", err)
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if key := node.Content[i].Value; !deployAllowedKeys[key] {
			d.unsupported = append(d.unsupported, key)
		}
	}
	sort.Strings(d.unsupported)
	return nil
}

// replicasFromDeploy validates deploy.replicas and maps it onto
// store.DesiredService.Replicas. Zero (unset) passes through unchanged;
// SaveDesiredService resolves that to store.DefaultReplicas, the same
// place a service's own top-level replicas: already gets resolved.
func replicasFromDeploy(d *Deploy) (int, error) {
	if d == nil || d.Replicas == 0 {
		return 0, nil
	}
	if d.Replicas < 1 {
		return 0, fmt.Errorf("deploy.replicas: want a positive integer, got %d", d.Replicas)
	}
	return d.Replicas, nil
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
