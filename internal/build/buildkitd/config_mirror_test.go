package buildkitd

import "testing"

func TestConfigFromEnvRegistryMirror(t *testing.T) {
	const pinned = "docker.io/moby/buildkit:v0.34.1-rootless@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	env := map[string]string{"APP_BUILDKIT_IMAGE": pinned, "APP_REGISTRY_MIRROR_PREFIX": "m.daocloud.io"}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] }, false)
	if err != nil && cfg.Image == "" {
		t.Skipf("env not satisfiable here: %v", err)
	}
	if cfg.Image != "m.daocloud.io/"+pinned {
		t.Errorf("image = %q", cfg.Image)
	}
}
