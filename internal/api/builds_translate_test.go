package api

import (
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestFormatMemoryBytes covers formatMemoryBytes, the reverse of
// internal/deploy's own parseMemoryBytes (Mi/Gi only). ResourceLimitsEditor.tsx's
// own comment (web/src/components/ResourceLimitsEditor.tsx) is why every
// byte count this codebase's UI can actually produce is an exact multiple
// of 1Mi: "a whole-number MiB field for memory chosen specifically to
// keep the byte round-trip lossless."
func TestFormatMemoryBytes(t *testing.T) {
	const mi = 1024 * 1024
	const gi = mi * 1024

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"exact Gi prefers Gi over the equivalent Mi count", 2 * gi, "2Gi"},
		{"one Mi", mi, "1Mi"},
		{"512 Mi, not evenly divisible by Gi", 512 * mi, "512Mi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMemoryBytes(tt.bytes); got != tt.want {
				t.Errorf("formatMemoryBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

// TestSpecServiceFromDesired covers the reconstruction
// specServiceFromDesired does for a manual build request: everything
// store.DesiredService can represent (port, domains, literal env, secret
// env names, resources, health) carried forward into a spec.Service the
// existing internal/deploy.Pipeline already knows how to build from,
// plus the caller-supplied Build block layered on top.
func TestSpecServiceFromDesired(t *testing.T) {
	svc := store.DesiredService{
		Name:      "web",
		Port:      8080,
		Domains:   []string{"a.example.com", "b.example.com"},
		Env:       map[string]string{"FOO": "bar"},
		SecretEnv: []string{"TOKEN"},
		Resources: &store.ServiceResources{MemoryBytes: 256 * 1024 * 1024, NanoCPUs: 250_000_000},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/ready", Interval: 5 * time.Second, Timeout: 2 * time.Second, Failures: 3},
			Liveness:  &store.ServiceProbe{Path: "/live", Interval: 30 * time.Second},
		},
	}
	buildCfg := spec.Build{Type: spec.BuildDockerfile, Path: "./Dockerfile"}

	got := specServiceFromDesired(svc, buildCfg)

	if !reflect.DeepEqual(got.Build, buildCfg) {
		t.Errorf("Build = %+v, want %+v", got.Build, buildCfg)
	}
	if got.Port != 8080 {
		t.Errorf("Port = %d, want 8080", got.Port)
	}
	if len(got.Domains) != 2 || got.Domains[0] != "a.example.com" || got.Domains[1] != "b.example.com" {
		t.Errorf("Domains = %v, want [a.example.com b.example.com]", got.Domains)
	}
	if v := got.Env["FOO"]; v.Value != "bar" || v.Secret {
		t.Errorf("Env[FOO] = %+v, want literal value bar", v)
	}
	if v := got.Env["TOKEN"]; !v.Secret || v.Value != "" || v.Required {
		t.Errorf("Env[TOKEN] = %+v, want a non-required secret marker with no value", v)
	}
	if got.Resources == nil || got.Resources.Memory != "256Mi" || got.Resources.CPU != 0.25 {
		t.Errorf("Resources = %+v, want Memory=256Mi CPU=0.25", got.Resources)
	}
	if got.Health == nil || got.Health.Readiness == nil {
		t.Fatalf("Health.Readiness is nil, want set")
	}
	if got.Health.Readiness.Path != "/ready" || got.Health.Readiness.Interval != "5s" || got.Health.Readiness.Timeout != "2s" || got.Health.Readiness.Failures != 3 {
		t.Errorf("Health.Readiness = %+v, want path=/ready interval=5s timeout=2s failures=3", got.Health.Readiness)
	}
	if got.Health.Liveness == nil || got.Health.Liveness.Path != "/live" || got.Health.Liveness.Interval != "30s" {
		t.Errorf("Health.Liveness = %+v, want path=/live interval=30s", got.Health.Liveness)
	}
}

// TestSpecServiceFromDesired_NoOptionalFields covers the zero-value case:
// an app with no resources/health/env set must not synthesize any of
// them, so a manual build against a bare-minimum app doesn't fail
// internal/deploy's own parseMemoryBytes on an empty string or similar.
func TestSpecServiceFromDesired_NoOptionalFields(t *testing.T) {
	svc := store.DesiredService{Name: "web", Port: 3000}
	got := specServiceFromDesired(svc, spec.Build{Type: spec.BuildDockerfile})

	if got.Env != nil {
		t.Errorf("Env = %v, want nil", got.Env)
	}
	if got.Resources != nil {
		t.Errorf("Resources = %+v, want nil", got.Resources)
	}
	if got.Health != nil {
		t.Errorf("Health = %+v, want nil", got.Health)
	}
}
