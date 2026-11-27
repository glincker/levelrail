package compose

import (
	"io/fs"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/registry"
)

// A udp port without a host port is refused at admission, so no registry
// template may declare one.
func TestRegistryUDPPortsHaveHostPorts(t *testing.T) {
	files, err := fs.Glob(registry.Templates, "templates/*/compose.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no registry compose files: %v", err)
	}
	for _, name := range files {
		data, err := fs.ReadFile(registry.Templates, name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(data)
		if err != nil {
			continue
		}
		for key, svc := range f.Services {
			for _, p := range svc.Ports {
				if p.Protocol == spec.PortProtocolUDP && p.HostPort == 0 {
					t.Errorf("%s service %s: udp port %d needs a host port", name, key, p.ContainerPort)
				}
			}
			if _, _, err := splitPorts(svc.Ports, nil); err != nil {
				t.Errorf("%s service %s: %v", name, key, err)
			}
		}
	}
}
