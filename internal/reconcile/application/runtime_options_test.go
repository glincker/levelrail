package application

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToContainerSpec_RuntimeOptions(t *testing.T) {
	on, to := true, 15
	d := &store.DesiredService{Name: "web", Image: "nginx:1", Runtime: &spec.RuntimeOptions{Init: &on, StopTimeout: &to, PidsLimit: 100,
		DNS: []string{"1.1.1.1"}, Ulimits: map[string]spec.Ulimit{"nofile": {Soft: 1, Hard: 2}}}}
	s, err := toContainerSpec("web-1", d)
	if err != nil || !s.Init || *s.StopTimeoutSeconds != 15 || s.PidsLimit != 100 || s.DNS[0] != "1.1.1.1" || s.Ulimits[0].Name != "nofile" {
		t.Fatalf("mapping wrong: %+v %v", s, err)
	}
	s, _ = toContainerSpec("web-1", &store.DesiredService{Name: "web", Image: "nginx:1"})
	if s.NeedsRuntimeCapability() || s.DNS != nil {
		t.Fatal("no runtime block must leave the spec untouched")
	}
}
