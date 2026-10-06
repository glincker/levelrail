package upgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func byCode(checks []Check, code string) Check {
	for _, c := range checks {
		if c.Code == code {
			return c
		}
	}
	return Check{}
}

func baseInputs() Inputs {
	now := time.Now()
	return Inputs{
		Lookup:        env(nil),
		DockerVersion: func(context.Context) (string, error) { return "27.3.1", nil },
		FreeBytes:     func() (int64, error) { return 10 << 30, nil },
		NewestBackup:  func() (time.Time, bool, error) { return now.Add(-time.Hour), true, nil },
		Now:           now,
		AssetNames:    []string{ChecksumsAsset, SignatureAsset, "acme-linux-amd64"},
		ReleaseKnown:  true,
		LookPath:      func(string) (string, error) { return "/usr/bin/cosign", nil },
	}
}

func TestRunHealthy(t *testing.T) {
	checks := Run(context.Background(), baseInputs())
	if Blocked(checks) {
		t.Fatalf("healthy inputs blocked: %+v", checks)
	}
	for _, c := range checks {
		if c.Status != StatusOK {
			t.Errorf("%s = %s (%s), want ok", c.Code, c.Status, c.Message)
		}
	}
}

func TestRunFailures(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badFile, []byte(`{"entries":[{"prefix":"27.3.","reason":"daemon deadlock"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Inputs)
		code   string
		want   string
	}{
		{"known bad docker", func(in *Inputs) { in.Lookup = env(map[string]string{envKnownBadFile: badFile}) }, "docker_engine", StatusFail},
		{"docker unreachable", func(in *Inputs) {
			in.DockerVersion = func(context.Context) (string, error) { return "", errors.New("no socket") }
		}, "docker_engine", StatusFail},
		{"low disk", func(in *Inputs) { in.FreeBytes = func() (int64, error) { return 100 << 20, nil } }, "disk_space", StatusFail},
		{"disk threshold override", func(in *Inputs) {
			in.FreeBytes = func() (int64, error) { return 100 << 20, nil }
			in.Lookup = env(map[string]string{envMinFreeBytes: "1048576"})
		}, "disk_space", StatusOK},
		{"missing signature", func(in *Inputs) { in.AssetNames = []string{ChecksumsAsset} }, "release_signature", StatusFail},
		{"no cosign on host", func(in *Inputs) {
			in.LookPath = func(string) (string, error) { return "", errors.New("not found") }
		}, "release_verifier", StatusWarn},
		{"verifier lookup unavailable", func(in *Inputs) { in.LookPath = nil }, "release_verifier", StatusUnknown},
		{"release unknown", func(in *Inputs) { in.ReleaseKnown = false }, "release_signature", StatusUnknown},
		{"stale backup", func(in *Inputs) {
			in.NewestBackup = func() (time.Time, bool, error) { return in.Now.Add(-72 * time.Hour), true, nil }
		}, "control_plane_backup", StatusWarn},
		{"no backup", func(in *Inputs) { in.NewestBackup = func() (time.Time, bool, error) { return time.Time{}, false, nil } }, "control_plane_backup", StatusWarn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInputs()
			tc.mutate(&in)
			if got := byCode(Run(context.Background(), in), tc.code); got.Status != tc.want {
				t.Errorf("%s = %s (%s), want %s", tc.code, got.Status, got.Message, tc.want)
			}
		})
	}
}

func TestEmbeddedKnownBadParses(t *testing.T) {
	if _, err := LoadKnownBad(env(nil)); err != nil {
		t.Fatalf("embedded list: %v", err)
	}
}
