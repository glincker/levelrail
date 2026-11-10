package database

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func renderParams(t *testing.T, desired *store.DesiredDatabase, ceilingMB int) docker.ContainerSpec {
	t.Helper()
	c := New(desired.Name, nil, nil, WithParamsMaxMemory(func() int { return ceilingMB }))
	spec := docker.ContainerSpec{Labels: map[string]string{}}
	c.engineParamsSpec(desired)(&spec)
	return spec
}

// TestEngineParamsSpec_ClampsToAdminCeiling proves a database with no memory
// limit renders a memory parameter over the live ceiling at the ceiling, so
// the cap still applies instead of the engine's uncapped default.
func TestEngineParamsSpec_ClampsToAdminCeiling(t *testing.T) {
	tests := []struct {
		name, engine, params string
		ceilingMB            int
		want                 string
	}{
		{"redis maxmemory", store.EngineRedis, `{"maxmemory":"8gb"}`, 1024, "--maxmemory 1gb"},
		{"redis maxmemory under", store.EngineRedis, `{"maxmemory":"512mb"}`, 1024, "--maxmemory 512mb"},
		{"redis maxmemory sub-unit", store.EngineRedis, `{"maxmemory":"8gb"}`, 512, "--maxmemory 536870912"},
		{"mongo cache", store.EngineMongoDB, `{"wiredTigerCacheSizeGB":"8"}`, 2048, "--wiredTigerCacheSizeGB 2"},
		{"postgres buffers", store.EnginePostgres, `{"shared_buffers":"2GB","work_mem":"8MB"}`, 1024, "-c shared_buffers=1GB -c work_mem=8MB"},
		{"no ceiling", store.EngineRedis, `{"maxmemory":"8gb"}`, 0, "--maxmemory 8gb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &store.DesiredDatabase{Name: "db", Engine: tt.engine, EngineParams: store.EngineParamSet(tt.params)}
			got := strings.Join(renderParams(t, desired, tt.ceilingMB).Command, " ")
			if !strings.HasSuffix(got, tt.want) {
				t.Errorf("command = %q, want it to end with %q", got, tt.want)
			}
		})
	}
}

// TestEngineParamsSpec_LabelIgnoresCeiling proves changing only the live
// ceiling leaves the fingerprint label alone, so it never replaces a container.
func TestEngineParamsSpec_LabelIgnoresCeiling(t *testing.T) {
	desired := &store.DesiredDatabase{Name: "db", Engine: store.EngineRedis, EngineParams: store.EngineParamSet(`{"maxmemory":"8gb"}`)}
	low := renderParams(t, desired, 1024).Labels[engineParamsLabelKey]
	high := renderParams(t, desired, 16384).Labels[engineParamsLabelKey]
	if low == "" || low != high {
		t.Errorf("label = %q at 1024 MB and %q at 16384 MB, want the same non-empty label", low, high)
	}
}
