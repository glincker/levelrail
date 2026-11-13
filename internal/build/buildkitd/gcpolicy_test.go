package buildkitd

import (
	"strings"
	"testing"
	"time"
)

const gib = int64(1024) * mib

func testGCConfig() Config {
	return Config{Snapshotter: "auto", GCKeepFor: 168 * time.Hour, GCSourceKeep: 48 * time.Hour, GCSourceMaxMB: 512}
}

func TestSizeGCFromDisk(t *testing.T) {
	tests := []struct {
		name                  string
		total                 int64
		mut                   func(*Config)
		reserved, maxUsed, mf int64
	}{
		{"unknown disk", 0, nil, 2 * gib, 5 * gib, 15},
		{"10 GB host clamps reserved", 10 * gib, nil, gib, 2 * gib, 15},
		{"25 GB host", 25 * gib, nil, 2 * gib, 5 * gib, 15},
		{"80 GB host capped", 80 * gib, nil, 2 * gib, 10 * gib, 15},
		{"500 GB host capped", 500 * gib, nil, 2 * gib, 10 * gib, 15},
		{"alias keep mb", 500 * gib, func(c *Config) { c.GCKeepMB = 512 }, 256 * mib, 512 * mib, 15},
		{"explicit overrides", 500 * gib, func(c *Config) { c.GCKeepMB = 4096; c.GCReservedMB = 1024; c.GCMinFreePct = 30 }, gib, 4 * gib, 30},
		{"reserved above max clamps", 500 * gib, func(c *Config) { c.GCKeepMB = 1024; c.GCReservedMB = 4096 }, gib, gib, 15},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := testGCConfig()
			if tc.mut != nil {
				tc.mut(&c)
			}
			got := c.sizeGC(tc.total)
			if got.ReservedBytes != tc.reserved || got.MaxUsedBytes != tc.maxUsed || got.MinFreePct != tc.mf {
				t.Fatalf("got %+v want reserved %d max %d minfree %d", got, tc.reserved, tc.maxUsed, tc.mf)
			}
			if got.ReservedBytes > got.MaxUsedBytes {
				t.Fatalf("reserved above max used: %+v", got)
			}
		})
	}
}

func TestGCPolicyTOMLUsesCurrentKeys(t *testing.T) {
	body := testGCConfig().tomlBody(25 * gib)
	for _, want := range []string{
		"all = true", `reservedSpace = "2048MB"`, `maxUsedSpace = "5120MB"`, `minFreeSpace = "15%"`,
		`filters = ["type==source.local", "type==source.git.checkout"]`, "keepDuration = 172800", "keepDuration = 604800",
		`maxUsedSpace = "512MB"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "keepBytes") || strings.Count(body, "[[worker.oci.gcpolicy]]") != 3 {
		t.Fatalf("unexpected policy:\n%s", body)
	}
}

func TestGCConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"APP_DATA_DIR": "/data", "APP_BUILDKIT_GC_RESERVED_MB": "1024", "APP_BUILDKIT_GC_MIN_FREE_PERCENT": "20",
		"APP_BUILDKIT_GC_SOURCE_KEEP": "24h", "APP_BUILDKIT_GC_SOURCE_MAX_MB": "256", "APP_BUILDKIT_GC_DISK_PATH": "/var/lib/docker",
	}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] }, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GCReservedMB != 1024 || cfg.GCMinFreePct != 20 || cfg.GCSourceKeep != 24*time.Hour || cfg.GCSourceMaxMB != 256 || cfg.DiskPath != "/var/lib/docker" || cfg.GCKeepMB != 0 {
		t.Fatalf("%+v", cfg)
	}
	for _, bad := range []string{"APP_BUILDKIT_GC_MIN_FREE_PERCENT=100", "APP_BUILDKIT_GC_RESERVED_MB=-1", "APP_BUILDKIT_GC_SOURCE_MAX_MB=x"} {
		k, v, _ := strings.Cut(bad, "=")
		if _, err := ConfigFromEnv(func(g string) string {
			if g == k {
				return v
			}
			return map[string]string{"APP_DATA_DIR": "/data"}[g]
		}, false); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
}
