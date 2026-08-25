package telemetry

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestParseAptUpgradable(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantTotal    int
		wantSecurity int
	}{
		{name: "no output", output: "", wantTotal: 0, wantSecurity: 0},
		{name: "banner only", output: "Listing...\n", wantTotal: 0, wantSecurity: 0},
		{
			name: "one non-security package",
			output: "Listing...\n" +
				"curl/jammy-updates 7.81.0-1ubuntu1.15 amd64 [upgradable from: 7.81.0-1ubuntu1.14]\n",
			wantTotal:    1,
			wantSecurity: 0,
		},
		{
			name: "one security package",
			output: "Listing...\n" +
				"openssl/jammy-security 3.0.2-0ubuntu1.15 amd64 [upgradable from: 3.0.2-0ubuntu1.14]\n",
			wantTotal:    1,
			wantSecurity: 1,
		},
		{
			name: "package in multiple suites, one security",
			output: "Listing...\n" +
				"libc6/jammy-updates,jammy-security 2.35-0ubuntu3.7 amd64 [upgradable from: 2.35-0ubuntu3.6]\n",
			wantTotal:    1,
			wantSecurity: 1,
		},
		{
			name: "mixed packages",
			output: "Listing...\n" +
				"curl/jammy-updates 7.81.0-1ubuntu1.15 amd64 [upgradable from: 7.81.0-1ubuntu1.14]\n" +
				"openssl/jammy-security 3.0.2-0ubuntu1.15 amd64 [upgradable from: 3.0.2-0ubuntu1.14]\n" +
				"vim/jammy-updates 2:8.2.3995-1ubuntu2.15 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.14]\n",
			wantTotal:    3,
			wantSecurity: 1,
		},
		{
			name:         "blank lines and stray warnings are ignored",
			output:       "\n\nWARNING: apt does not have a stable CLI interface\n\n",
			wantTotal:    0,
			wantSecurity: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTotal, gotSecurity := parseAptUpgradable(tt.output)
			if gotTotal != tt.wantTotal {
				t.Errorf("total = %d, want %d", gotTotal, tt.wantTotal)
			}
			if gotSecurity != tt.wantSecurity {
				t.Errorf("security = %d, want %d", gotSecurity, tt.wantSecurity)
			}
		})
	}
}

func TestParseCheckUpdateOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{name: "no output", output: "", want: 0},
		{
			name:   "header only, no packages",
			output: "Last metadata expiration check: 0:12:34 ago on Mon 01 Jan 2026.\n",
			want:   0,
		},
		{
			name: "two packages",
			output: "Last metadata expiration check: 0:12:34 ago on Mon 01 Jan 2026.\n" +
				"NetworkManager.x86_64          1:1.42.2-1.fc38          updates\n" +
				"kernel.x86_64                  6.5.6-200.fc38           updates\n",
			want: 2,
		},
		{
			name:   "single field line is not a package",
			output: "Obsoleting Packages\n",
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCheckUpdateOutput(tt.output); got != tt.want {
				t.Errorf("parseCheckUpdateOutput() = %d, want %d", got, tt.want)
			}
		})
	}
}

// fakeExitError builds an *exec.ExitError carrying code without actually
// running a process, using a real failing command ("false") so the
// underlying os.ProcessState is genuine rather than a hand-built zero
// value exec.ExitError's own fields would reject.
func fakeExitError(t *testing.T, code int) error {
	t.Helper()
	shArgs := []string{"-c", "exit " + itoa(code)}
	cmd := exec.Command("sh", shArgs...) //nolint:gosec // fixed test-only args, not user input
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected sh -c 'exit %d' to fail", code)
	}
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestHostPatchChecker_Check_PrefersAPT(t *testing.T) {
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil // every manager "exists"
		},
		run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name != "apt" {
				t.Fatalf("expected apt to be tried first, got %q", name)
			}
			return []byte("curl/jammy-security 7.81.0-1ubuntu1.15 amd64 [upgradable from: 7.81.0-1ubuntu1.14]\n"), nil
		},
	}

	got, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	want := PatchCounts{Manager: "apt", Total: 1, Security: 1}
	if got != want {
		t.Errorf("Check() = %+v, want %+v", got, want)
	}
}

func TestHostPatchChecker_Check_FallsBackToDNF(t *testing.T) {
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "dnf" {
				return "/usr/bin/dnf", nil
			}
			return "", errors.New("not found")
		},
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "dnf" {
				t.Fatalf("expected dnf, got %q", name)
			}
			isSecurity := len(args) > 1 && args[1] == "--security"
			if isSecurity {
				return []byte("kernel.x86_64 6.5.6-200.fc38 updates\n"), nil
			}
			return []byte("kernel.x86_64 6.5.6-200.fc38 updates\nbash.x86_64 5.2.15-1.fc38 updates\n"), nil
		},
	}

	got, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	want := PatchCounts{Manager: "dnf", Total: 2, Security: 1}
	if got != want {
		t.Errorf("Check() = %+v, want %+v", got, want)
	}
}

func TestHostPatchChecker_Check_DNFNoUpdatesExitCode0(t *testing.T) {
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "dnf" {
				return "/usr/bin/dnf", nil
			}
			return "", errors.New("not found")
		},
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(""), nil // exit 0: no updates available
		},
	}

	got, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	want := PatchCounts{Manager: "dnf", Total: 0, Security: 0}
	if got != want {
		t.Errorf("Check() = %+v, want %+v", got, want)
	}
}

func TestHostPatchChecker_Check_DNFExitCode100MeansUpdatesAvailable(t *testing.T) {
	exit100 := fakeExitError(t, 100)
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "dnf" {
				return "/usr/bin/dnf", nil
			}
			return "", errors.New("not found")
		},
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			isSecurity := len(args) > 1 && args[1] == "--security"
			if isSecurity {
				return nil, exit100
			}
			return []byte("kernel.x86_64 6.5.6-200.fc38 updates\n"), exit100
		},
	}

	got, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if got.Total != 1 {
		t.Errorf("Total = %d, want 1", got.Total)
	}
}

func TestHostPatchChecker_Check_DNFRealErrorPropagates(t *testing.T) {
	exit1 := fakeExitError(t, 1)
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "dnf" {
				return "/usr/bin/dnf", nil
			}
			return "", errors.New("not found")
		},
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exit1
		},
	}

	if _, err := c.Check(context.Background()); err == nil {
		t.Error("Check() error = nil, want an error for a real dnf failure")
	}
}

func TestHostPatchChecker_Check_SecurityFailureDegradesToZero(t *testing.T) {
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "dnf" {
				return "/usr/bin/dnf", nil
			}
			return "", errors.New("not found")
		},
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			isSecurity := len(args) > 1 && args[1] == "--security"
			if isSecurity {
				return nil, errors.New("--security not supported by this repo config")
			}
			return []byte("kernel.x86_64 6.5.6-200.fc38 updates\n"), nil
		},
	}

	got, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	want := PatchCounts{Manager: "dnf", Total: 1, Security: 0}
	if got != want {
		t.Errorf("Check() = %+v, want %+v", got, want)
	}
}

func TestHostPatchChecker_Check_NoSupportedManager(t *testing.T) {
	c := &HostPatchChecker{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("run should not be called when no package manager was found")
			return nil, nil
		},
	}

	if _, err := c.Check(context.Background()); !errors.Is(err, ErrNoSupportedPackageManager) {
		t.Errorf("Check() error = %v, want ErrNoSupportedPackageManager", err)
	}
}

func TestHostPatchChecker_Check_APTCommandFailurePropagates(t *testing.T) {
	c := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "apt" {
				return "/usr/bin/apt", nil
			}
			return "", errors.New("not found")
		},
		run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("apt: permission denied")
		},
	}

	if _, err := c.Check(context.Background()); err == nil {
		t.Error("Check() error = nil, want an error when apt itself fails")
	}
}

func TestHostPatchCollector_CollectOnce_WritesBothMetrics(t *testing.T) {
	db := newTestDB(t)
	checker := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "apt" {
				return "/usr/bin/apt", nil
			}
			return "", errors.New("not found")
		},
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(
				"curl/jammy-updates 7.81.0-1ubuntu1.15 amd64 [upgradable from: 7.81.0-1ubuntu1.14]\n" +
					"openssl/jammy-security 3.0.2-0ubuntu1.15 amd64 [upgradable from: 3.0.2-0ubuntu1.14]\n",
			), nil
		},
	}
	c := NewHostPatchCollector(checker, "node:local", db, time.Second, nil)

	if err := c.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}

	from, to := time.Now().Add(-time.Minute), time.Now().Add(time.Minute)
	total, err := db.Query(context.Background(), "node:local", MetricOSPatchesAvailable, from, to)
	if err != nil {
		t.Fatalf("Query(%s) error = %v", MetricOSPatchesAvailable, err)
	}
	if len(total) != 1 || total[0].Value != 2 {
		t.Errorf("%s = %+v, want one sample with value 2", MetricOSPatchesAvailable, total)
	}

	security, err := db.Query(context.Background(), "node:local", MetricOSSecurityPatchesAvailable, from, to)
	if err != nil {
		t.Fatalf("Query(%s) error = %v", MetricOSSecurityPatchesAvailable, err)
	}
	if len(security) != 1 || security[0].Value != 1 {
		t.Errorf("%s = %+v, want one sample with value 1", MetricOSSecurityPatchesAvailable, security)
	}
}

func TestHostPatchCollector_CollectOnce_NoPackageManager_WritesNothingNoError(t *testing.T) {
	db := newTestDB(t)
	checker := &HostPatchChecker{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("run should not be called when no package manager was found")
			return nil, nil
		},
	}
	c := NewHostPatchCollector(checker, "node:local", db, time.Second, nil)

	if err := c.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v, want nil (degrade to unknown, not an error)", err)
	}

	from, to := time.Now().Add(-time.Minute), time.Now().Add(time.Minute)
	samples, err := db.Query(context.Background(), "node:local", MetricOSPatchesAvailable, from, to)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("samples = %+v, want none written when no package manager exists", samples)
	}
}

func TestHostPatchCollector_CollectOnce_CommandFailure_ReturnsError(t *testing.T) {
	db := newTestDB(t)
	checker := &HostPatchChecker{
		lookPath: func(name string) (string, error) {
			if name == "apt" {
				return "/usr/bin/apt", nil
			}
			return "", errors.New("not found")
		},
		run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("apt: permission denied")
		},
	}
	c := NewHostPatchCollector(checker, "node:local", db, time.Second, nil)

	if err := c.CollectOnce(context.Background()); err == nil {
		t.Error("CollectOnce() error = nil, want an error when the command itself fails")
	}
}

func TestNewHostPatchCollector_DefaultsCheckerAndLogger(t *testing.T) {
	db := newTestDB(t)
	c := NewHostPatchCollector(nil, "node:local", db, time.Second, nil)
	if c.checker == nil {
		t.Error("checker = nil, want a default HostPatchChecker")
	}
	if c.logger == nil {
		t.Error("logger = nil, want slog.Default()")
	}
}
