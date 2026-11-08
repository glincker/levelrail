package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestApplyRuntimeOptions(t *testing.T) {
	hc := &container.HostConfig{CapDrop: []string{"ALL"}, Tmpfs: map[string]string{"/tmp": "x"}}
	applyRuntimeOptions(hc, ContainerSpec{Init: true, Tmpfs: []string{"/run:size=8m", "/tmp:size=4m"}, CapDrop: []string{"NET_RAW", "ALL"},
		Ulimits: []Ulimit{{Name: "nofile", Soft: 1, Hard: 2}}, Sysctls: map[string]string{"net.core.somaxconn": "9"}})
	if hc.Init == nil || !*hc.Init || hc.Tmpfs["/run"] != "size=8m" || hc.Tmpfs["/tmp"] != "size=4m" {
		t.Fatalf("init/tmpfs wrong: %+v", hc)
	}
	if len(hc.CapDrop) != 2 || len(hc.Ulimits) != 1 || hc.Ulimits[0].Hard != 2 || hc.Sysctls["net.core.somaxconn"] != "9" {
		t.Fatalf("caps/ulimits/sysctls wrong: %+v", hc)
	}
	var none container.HostConfig
	applyRuntimeOptions(&none, ContainerSpec{})
	if none.Init != nil || none.Ulimits != nil || none.Tmpfs != nil || none.CapDrop != nil {
		t.Fatal("empty spec must not touch the host config")
	}
	if (ContainerSpec{}).NeedsRuntimeCapability() || !(ContainerSpec{PidsLimit: 5}).NeedsRuntimeCapability() {
		t.Fatal("capability gate wrong")
	}
}
