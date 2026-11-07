package buildkitd

import (
	"context"
	"testing"
)

const mirrorPinned = "docker.io/moby/buildkit:v0.34.1-rootless@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestConfigFromEnvRegistryMirror(t *testing.T) {
	env := map[string]string{
		"APP_BUILDKIT_IMAGE": mirrorPinned, "APP_REGISTRY_MIRROR_PREFIX": "m.daocloud.io",
		"APP_DATA_DIR": t.TempDir(),
	}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] }, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "m.daocloud.io/"+mirrorPinned {
		t.Errorf("image = %q", cfg.Image)
	}
}

func TestSupervisorLiveMirrorPrefix(t *testing.T) {
	for _, tc := range []struct{ name, prefix, want string }{
		{"live prefix", "m.example.com", "m.example.com/" + testImage},
		{"empty keeps seed", "", testImage},
		{"invalid ignored", "https://bad", testImage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &fakeRT{}
			s, _ := newSup(t, rt)
			s.cfg.MirrorPrefix = func(context.Context) string { return tc.prefix }
			if _, err := s.Acquire(context.Background()); err != nil {
				t.Fatal(err)
			}
			if rt.created.Image != tc.want {
				t.Errorf("image = %q, want %q", rt.created.Image, tc.want)
			}
		})
	}
}
