package agent

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestContainerSpecUserWorkingDirRoundTrip(t *testing.T) {
	back := containerSpecFromPB(containerSpecToPB(docker.ContainerSpec{Name: "web", Image: "img:v1", User: "1000:1000", WorkingDir: "/srv"}))
	if back.User != "1000:1000" || back.WorkingDir != "/srv" {
		t.Fatalf("round trip user/working_dir = %q/%q", back.User, back.WorkingDir)
	}
}
