package api

import (
	"context"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"strings"
	"testing"
)

func TestBuildJoinCommandsRegistryMirror(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, wantImage string
	}{
		{"mirror set", "m.example.com", " m.example.com/ghcr.io/glincker/"},
		{"no mirror", "", " ghcr.io/glincker/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_REGISTRY_MIRROR_PREFIX", tc.prefix)
			rt, _ := newTestRouter(t)
			cmds := rt.buildJoinCommands(context.Background(), "cp:9443", "tok", "", "n1", false, joinDomain{})
			for _, c := range []string{cmds.Docker, cmds.Podman} {
				if !strings.Contains(c, tc.wantImage) {
					t.Errorf("command %q lacks %q", c, tc.wantImage)
				}
			}
			if tc.prefix == "" && strings.Contains(cmds.Docker, "m.example.com") {
				t.Error("unexpected mirror in command")
			}
		})
	}
}

func TestBuildJoinCommandsMacOSFlag(t *testing.T) {
	t.Cleanup(func() { experimental.Set() })
	rt, _ := newTestRouter(t)

	experimental.Set()
	if got := rt.buildJoinCommands(context.Background(), "cp:9443", "tok", "fp", "n1", false, joinDomain{}).MacOS; got != "" {
		t.Errorf("flag off: macos command %q, want empty", got)
	}

	experimental.Set(experimental.MacOSAgent)
	got := rt.buildJoinCommands(context.Background(), "cp:9443", "tok", "fp", "n1", false, joinDomain{}).MacOS
	if !strings.HasPrefix(got, "brew install glincker/tap/") {
		t.Errorf("macos command %q lacks brew line", got)
	}
	for _, want := range []string{"APP_CONTROL_PLANE_ADDR=cp:9443", "APP_JOIN_TOKEN=tok", "APP_CA_FINGERPRINT=fp"} {
		if !strings.Contains(got, want) {
			t.Errorf("macos command %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "sudo") {
		t.Error("macos command must not use sudo")
	}
}
