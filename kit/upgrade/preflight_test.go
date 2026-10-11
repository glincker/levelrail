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
		}, "control_plane_backup", StatusFail},
		{"no backup", func(in *Inputs) { in.NewestBackup = func() (time.Time, bool, error) { return time.Time{}, false, nil } }, "control_plane_backup", StatusFail},
		{"backup age threshold override", func(in *Inputs) {
			in.NewestBackup = func() (time.Time, bool, error) { return in.Now.Add(-72 * time.Hour), true, nil }
			in.Lookup = env(map[string]string{envMaxBackupAge: "100h"})
		}, "control_plane_backup", StatusOK},
		{"release still publishing", func(in *Inputs) {
			in.AssetNames = []string{ChecksumsAsset}
			in.ReleasePublishedAt = in.Now.Add(-4 * time.Minute)
		}, "release_signature", StatusPending},
		{"old release without signature", func(in *Inputs) {
			in.AssetNames = []string{ChecksumsAsset}
			in.ReleasePublishedAt = in.Now.Add(-5 * time.Hour)
		}, "release_signature", StatusFail},
		{"settle window override", func(in *Inputs) {
			in.AssetNames = nil
			in.ReleasePublishedAt = in.Now.Add(-3 * time.Hour)
			in.Lookup = env(map[string]string{envReleaseSettle: "6h"})
		}, "release_signature", StatusPending},
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

func TestCheckFixActions(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*Inputs)
		code       string
		wantKind   string
		wantFix    string
		wantRetry  bool
		wantBlocks bool
	}{
		{"stale backup offers back up now", func(in *Inputs) {
			in.NewestBackup = func() (time.Time, bool, error) { return in.Now.Add(-40 * time.Hour), true, nil }
		}, "control_plane_backup", KindBackupStale, FixBackupNow, false, true},
		{"missing backup offers back up now", func(in *Inputs) {
			in.NewestBackup = func() (time.Time, bool, error) { return time.Time{}, false, nil }
		}, "control_plane_backup", KindBackupMissing, FixBackupNow, false, true},
		{"half published release is retryable", func(in *Inputs) {
			in.AssetNames = []string{ChecksumsAsset}
			in.ReleasePublishedAt = in.Now.Add(-time.Minute)
		}, "release_signature", KindReleaseIncomplete, FixRecheck, true, true},
		{"old unsigned release is a hard block", func(in *Inputs) {
			in.AssetNames = nil
			in.ReleasePublishedAt = in.Now.Add(-48 * time.Hour)
		}, "release_signature", KindReleaseUnsigned, FixRecheck, false, true},
		{"low disk can be re-checked", func(in *Inputs) {
			in.FreeBytes = func() (int64, error) { return 1 << 20, nil }
		}, "disk_space", "", FixRecheck, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInputs()
			tc.mutate(&in)
			checks := Run(context.Background(), in)
			got := byCode(checks, tc.code)
			if got.Kind != tc.wantKind || got.Retryable != tc.wantRetry {
				t.Errorf("kind=%q retryable=%v, want %q %v (%+v)", got.Kind, got.Retryable, tc.wantKind, tc.wantRetry, got)
			}
			if got.Fix == nil || got.Fix.Action != tc.wantFix {
				t.Errorf("fix = %+v, want action %s", got.Fix, tc.wantFix)
			}
			if got.Hint == "" || got.Protects == "" {
				t.Errorf("a failing check needs a hint and a protects line: %+v", got)
			}
			if Blocked(checks) != tc.wantBlocks {
				t.Errorf("blocked = %v, want %v", Blocked(checks), tc.wantBlocks)
			}
		})
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{20 * time.Second, "under a minute"},
		{45 * time.Minute, "45m"},
		{11*time.Hour + 18*time.Minute, "11h 18m"},
		{26 * time.Hour, "1d 2h"},
		{48 * time.Hour, "2d"},
		{-time.Hour, "under a minute"},
	}
	for _, tc := range tests {
		if got := HumanDuration(tc.d); got != tc.want {
			t.Errorf("HumanDuration(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
