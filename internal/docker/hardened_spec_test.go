package docker

import (
	"reflect"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestHardenedSpecIgnoresGlobalMode(t *testing.T) {
	for _, mode := range []HardeningMode{HardeningOff, HardeningWarn, HardeningEnforce} {
		spec := ContainerSpec{Name: "p", Hardened: true, CapAdd: []string{"NET_ADMIN"}}
		hc := buildHostConfig(spec, nat.PortMap{})
		HardeningConfig{Mode: mode, ExtraCaps: []string{"CHOWN"}}.apply(hc, spec)
		if !hc.ReadonlyRootfs || !reflect.DeepEqual([]string(hc.CapDrop), []string{"ALL"}) || len(hc.CapAdd) != 0 {
			t.Errorf("mode %s: rootfs=%v drop=%v add=%v, want read-only, drop ALL, nothing added", mode, hc.ReadonlyRootfs, hc.CapDrop, hc.CapAdd)
		}
		if !reflect.DeepEqual(hc.SecurityOpt, []string{"no-new-privileges"}) || hc.PidsLimit == nil || hc.Tmpfs["/tmp"] == "" {
			t.Errorf("mode %s: secopt=%v pids=%v tmpfs=%v", mode, hc.SecurityOpt, hc.PidsLimit, hc.Tmpfs)
		}
	}
}
