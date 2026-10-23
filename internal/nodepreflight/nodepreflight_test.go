package nodepreflight

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func healthy() Facts {
	return Facts{
		OS: "linux", Arch: "amd64", DockerOK: true, DockerVersion: "27.1",
		WireGuardKernel: true, MeshPort: 51820, MeshPortBindable: true,
		DiskTotalBytes: 100 << 30, DiskFreeBytes: 50 << 30, AgentTime: now,
	}
}

func cfg() Config { return ConfigFromEnv(func(string) (string, bool) { return "", false }) }

func byID(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", id, checks)
	return Check{}
}

func TestHealthyNodeIsReadyWithOnlyFirewallUnknown(t *testing.T) {
	checks := Evaluate(healthy(), now, cfg())
	s := Summarize(checks)
	if !s.Ready || s.Fail != 0 || s.Warn != 0 || s.Unknown != 1 {
		t.Fatalf("summary = %+v, checks = %+v", s, checks)
	}
	if fw := byID(t, checks, CheckFirewall); fw.Status != StatusUnknown || fw.Fix == "" {
		t.Errorf("firewall must be unverified with a fix, got %+v", fw)
	}
}

func TestEveryFailureCarriesAConcreteFix(t *testing.T) {
	f := Facts{OS: "darwin", Arch: "386", DockerOK: false, DockerError: "permission denied",
		DiskTotalBytes: 10 << 30, DiskFreeBytes: 100 << 20, AgentTime: now.Add(-10 * time.Minute)}
	checks := Evaluate(f, now, cfg())
	for _, id := range []string{CheckDocker, CheckArchitecture, CheckDisk, CheckClock, CheckWireGuard} {
		c := byID(t, checks, id)
		if c.Status != StatusFail || c.Fix == "" || c.Detail == "" {
			t.Errorf("%s = %+v, want a failure with a fix", id, c)
		}
	}
	if d := byID(t, checks, CheckDocker); !strings.Contains(d.Detail, "permission denied") || d.FixCommand == "" {
		t.Errorf("docker = %+v", d)
	}
	if Summarize(checks).Ready {
		t.Error("a failing fleet must not be ready")
	}
}

func TestClockSkewBothDirectionsAndThresholds(t *testing.T) {
	tests := []struct {
		name string
		skew time.Duration
		want string
	}{
		{"in sync", 200 * time.Millisecond, StatusPass},
		{"node ahead warn", -20 * time.Second, StatusWarn},
		{"node behind warn", 20 * time.Second, StatusWarn},
		{"node ahead fail", -2 * time.Minute, StatusFail},
		{"node behind fail", 2 * time.Minute, StatusFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := healthy()
			f.AgentTime = now.Add(-tc.skew)
			if got := byID(t, Evaluate(f, now, cfg()), CheckClock).Status; got != tc.want {
				t.Errorf("status = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestMissingClockIsUnknownNotPass(t *testing.T) {
	f := healthy()
	f.AgentTime = time.Time{}
	if got := byID(t, Evaluate(f, now, cfg()), CheckClock).Status; got != StatusUnknown {
		t.Errorf("status = %s", got)
	}
}

func TestDiskThresholdsComeFromConfig(t *testing.T) {
	f := healthy()
	f.DiskFreeBytes = 8 << 30
	env := map[string]string{EnvMinDiskFree: "10240"}
	c := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if got := byID(t, Evaluate(f, now, c), CheckDisk).Status; got != StatusFail {
		t.Errorf("8 GiB free with a 10 GiB minimum = %s", got)
	}
	f.DiskFreeBytes = 15 << 30
	if got := byID(t, Evaluate(f, now, c), CheckDisk).Status; got != StatusWarn {
		t.Errorf("15 GiB free with a 10 GiB minimum = %s", got)
	}
	f.DiskFreeBytes = 30 << 30
	if got := byID(t, Evaluate(f, now, c), CheckDisk).Status; got != StatusPass {
		t.Errorf("30 GiB free with a 10 GiB minimum = %s", got)
	}
}

func TestUnreadableDiskIsUnknown(t *testing.T) {
	f := healthy()
	f.DiskTotalBytes, f.DiskFreeBytes = 0, 0
	if got := byID(t, Evaluate(f, now, cfg()), CheckDisk).Status; got != StatusUnknown {
		t.Errorf("status = %s", got)
	}
}

func TestWireGuardMissingOnLinuxOnlyWarns(t *testing.T) {
	f := healthy()
	f.WireGuardKernel = false
	c := byID(t, Evaluate(f, now, cfg()), CheckWireGuard)
	if c.Status != StatusWarn || c.FixCommand == "" {
		t.Errorf("wireguard = %+v", c)
	}
	if !Summarize(Evaluate(f, now, cfg())).Ready {
		t.Error("a missing kernel module must not block joining")
	}
}

func TestMeshPortStates(t *testing.T) {
	f := healthy()
	f.MeshPortBindable = false
	if c := byID(t, Evaluate(f, now, cfg()), CheckMeshPort); c.Status != StatusWarn || !strings.Contains(c.Detail, "51820") {
		t.Errorf("busy port = %+v", c)
	}
	f.MeshEnabled = true
	if c := byID(t, Evaluate(f, now, cfg()), CheckMeshPort); c.Status != StatusPass {
		t.Errorf("a running mesh holds the port itself, got %+v", c)
	}
}

func TestFirewallFixNamesTheProvider(t *testing.T) {
	f := healthy()
	f.Provider = "hetzner"
	c := byID(t, Evaluate(f, now, cfg()), CheckFirewall)
	if !strings.Contains(c.Fix, "Hetzner Cloud") || !strings.Contains(c.Fix, "UDP 51820") {
		t.Errorf("fix = %q", c.Fix)
	}
	f.Provider = "somewhere-unknown"
	if c := byID(t, Evaluate(f, now, cfg()), CheckFirewall); !strings.Contains(c.Fix, "security group") {
		t.Errorf("generic fix = %q", c.Fix)
	}
}

func TestArchitecturesFromEnv(t *testing.T) {
	f := healthy()
	f.Arch = "riscv64"
	if got := byID(t, Evaluate(f, now, cfg()), CheckArchitecture).Status; got != StatusFail {
		t.Fatalf("default config must refuse riscv64, got %s", got)
	}
	c := ConfigFromEnv(func(k string) (string, bool) {
		if k == EnvArchitectures {
			return "amd64, riscv64", true
		}
		return "", false
	})
	if got := byID(t, Evaluate(f, now, c), CheckArchitecture).Status; got != StatusPass {
		t.Errorf("status = %s", got)
	}
}

func TestConfigFromEnvIgnoresGarbage(t *testing.T) {
	env := map[string]string{EnvMinDiskFree: "lots", EnvMaxClockSkew: "-5s"}
	c := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if c.MinDiskFreeBytes != defaultMinDiskFreeMB*mib || c.MaxClockSkew != defaultMaxClockSkew {
		t.Errorf("cfg = %+v", c)
	}
}

func TestEvaluateIsDeterministic(t *testing.T) {
	a := Evaluate(healthy(), now, cfg())
	for range 10 {
		b := Evaluate(healthy(), now, cfg())
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("check %d changed between runs: %+v vs %+v", i, a[i], b[i])
			}
		}
	}
}
