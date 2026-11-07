package api

import (
	"context"
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
