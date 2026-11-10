package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCloneDesiredService_KeepsUserAndWorkingDir(t *testing.T) {
	src := store.DesiredService{Name: "web", Image: "nginx:1", User: "1000:1000", WorkingDir: "/srv"}
	got := cloneDesiredService(src, "web-staging", nil)
	if got.User != src.User || got.WorkingDir != src.WorkingDir {
		t.Fatalf("clone user/working_dir = %q/%q", got.User, got.WorkingDir)
	}
}

func TestComposeConfigChanged_UserAndWorkingDir(t *testing.T) {
	base := store.DesiredService{Name: "app-web", Image: "nginx:1", User: "node", WorkingDir: "/app"}
	if composeConfigChanged(base, base) {
		t.Fatal("identical services reported as changed")
	}
	user, dir := base, base
	user.User = "1000"
	dir.WorkingDir = "/srv"
	if !composeConfigChanged(base, user) || !composeConfigChanged(base, dir) {
		t.Fatal("a user or working_dir change must count as a config change")
	}
}
