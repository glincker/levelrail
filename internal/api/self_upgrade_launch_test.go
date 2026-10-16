package api

import (
	"slices"
	"testing"
)

func TestSystemdRunArgsCarryWorkingDirAndEnv(t *testing.T) {
	env := map[string]string{"APP_DATA_DIR": "/data", "APP_MASTER_KEY": "secret", "APP_HTTP_ADDR": "10.0.0.1:1"}
	got := systemdRunArgs("/usr/local/bin/x", "unit-1", "/data", func(k string) string { return env[k] }, []string{"--to", "v1.0.0"})
	for _, want := range []string{"--working-directory=/data", "--setenv=APP_DATA_DIR=/data", "--setenv=APP_HTTP_ADDR=10.0.0.1:1"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, a := range got {
		if a == "--setenv=APP_MASTER_KEY=secret" {
			t.Fatal("secret leaked into argv")
		}
	}
	if tail := got[len(got)-3:]; !slices.Equal(tail, []string{"self-upgrade", "--to", "v1.0.0"}) {
		t.Fatalf("tail = %v", tail)
	}
}
