package application

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToContainerSpec_UserAndWorkingDir(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "nginx:1", User: "1000:1000", WorkingDir: "/srv"}
	spec, err := toContainerSpec("web-1", desired)
	if err != nil {
		t.Fatal(err)
	}
	if spec.User != "1000:1000" || spec.WorkingDir != "/srv" {
		t.Fatalf("container spec user/working_dir = %q/%q", spec.User, spec.WorkingDir)
	}
	spec, err = toContainerSpec("web-1", &store.DesiredService{Name: "web", Image: "nginx:1"})
	if err != nil || spec.User != "" || spec.WorkingDir != "" {
		t.Fatalf("unset fields must keep the image defaults: %v %q/%q", err, spec.User, spec.WorkingDir)
	}
}
