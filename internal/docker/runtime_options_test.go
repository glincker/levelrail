package docker

import (
	"testing"
	"time"

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

func TestPickStopTimeout(t *testing.T) {
	n := 45
	for _, tc := range []struct {
		grace time.Duration
		cfg   *int
		want  time.Duration
	}{
		{0, &n, 0}, {-time.Second, &n, -time.Second}, {10 * time.Second, nil, 10 * time.Second}, {10 * time.Second, &n, 45 * time.Second}, {60 * time.Second, &n, 60 * time.Second},
	} {
		if got := pickStopTimeout(tc.grace, tc.cfg); got != tc.want {
			t.Errorf("pickStopTimeout(%v) = %v, want %v", tc.grace, got, tc.want)
		}
	}
}
