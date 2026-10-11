package catalog

import (
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// StackService is one service of a manifest template as the catalog
// browser and docs page describe it.
type StackService struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Ports     []int    `json:"ports,omitempty"`
	Volumes   []string `json:"volumes,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// Stack lists the manifest's services with their image, container ports,
// named volumes and start-order dependencies, sorted by service name.
func (m Manifest) Stack() ([]StackService, error) {
	f, err := compose.Parse([]byte(m.Compose))
	if err != nil {
		return nil, fmt.Errorf("template %s: parse compose: %w", m.ID, err)
	}
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]StackService, 0, len(names))
	for _, n := range names {
		svc := f.Services[n]
		s := StackService{Name: n, Image: svc.Image, DependsOn: append([]string(nil), svc.DependsOn...)}
		for _, p := range svc.Ports {
			s.Ports = append(s.Ports, p.ContainerPort)
		}
		for _, v := range svc.Volumes {
			if v.Name != "" {
				s.Volumes = append(s.Volumes, v.Name+":"+v.ContainerPath)
			}
		}
		sort.Strings(s.DependsOn)
		out = append(out, s)
	}
	return out, nil
}
