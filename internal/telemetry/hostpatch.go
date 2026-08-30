package telemetry

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// MetricOSPatchesAvailable and MetricOSSecurityPatchesAvailable are the
// sample metric names HostPatchCollector writes, and the names
// internal/api reads back (node_patch_status.go, node_metrics.go).
const (
	MetricOSPatchesAvailable         = "os_patches_available"
	MetricOSSecurityPatchesAvailable = "os_security_patches_available"
)

// ErrNoSupportedPackageManager means none of apt, dnf, or yum exist on
// this host: a real, permanent condition (e.g. a non-Linux dev machine,
// or a distribution this checker doesn't know), not a transient failure.
var ErrNoSupportedPackageManager = errors.New("telemetry: no supported package manager found")

// PatchCounts is one point-in-time reading of a host's available OS
// package updates.
type PatchCounts struct {
	Manager  string
	Total    int
	Security int
}

// commandRunner abstracts exec.CommandContext so tests can supply fixture
// output instead of shelling out to a real package manager, the
// "fake-exec" seam this package's own doc comment (hostpatch_test.go)
// relies on.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runRealCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // name/args are fixed literals chosen by this file, never caller input
}

// HostPatchChecker looks up how many OS package updates a host has
// available, via whichever supported package manager exists on PATH.
type HostPatchChecker struct {
	lookPath func(string) (string, error)
	run      commandRunner
}

// NewHostPatchChecker builds a HostPatchChecker that shells out to the
// real package manager binaries on PATH.
func NewHostPatchChecker() *HostPatchChecker {
	return &HostPatchChecker{lookPath: exec.LookPath, run: runRealCommand}
}

// Check runs the first supported package manager it finds (apt, then
// dnf, then yum) and returns its upgrade counts, or
// ErrNoSupportedPackageManager if none exist. A found manager whose
// command itself fails returns that error wrapped, never a false zero.
func (c *HostPatchChecker) Check(ctx context.Context) (PatchCounts, error) {
	if _, err := c.lookPath("apt"); err == nil {
		return c.checkAPT(ctx)
	}
	if _, err := c.lookPath("dnf"); err == nil {
		return c.checkDNFFamily(ctx, "dnf")
	}
	if _, err := c.lookPath("yum"); err == nil {
		return c.checkDNFFamily(ctx, "yum")
	}
	return PatchCounts{}, ErrNoSupportedPackageManager
}

func (c *HostPatchChecker) checkAPT(ctx context.Context) (PatchCounts, error) {
	out, err := c.run(ctx, "apt", "list", "--upgradable")
	if err != nil {
		return PatchCounts{}, fmt.Errorf("telemetry: apt list --upgradable: %w", err)
	}
	total, security := parseAptUpgradable(string(out))
	return PatchCounts{Manager: "apt", Total: total, Security: security}, nil
}

// checkDNFFamily handles both dnf and yum: their check-update subcommand
// shares the same exit-code contract (0 = no updates, 100 = updates
// listed on stdout, anything else = a real error) and close enough
// output shape for parseCheckUpdateOutput to cover both. The
// --security-only rerun is best-effort: a manager or repo config that
// doesn't support it degrades to Security: 0 rather than failing the
// whole check, since Total is still a real, useful reading on its own.
func (c *HostPatchChecker) checkDNFFamily(ctx context.Context, manager string) (PatchCounts, error) {
	out, err := runCheckUpdate(ctx, c.run, manager)
	if err != nil {
		return PatchCounts{}, fmt.Errorf("telemetry: %s check-update: %w", manager, err)
	}
	total := parseCheckUpdateOutput(out)

	security := 0
	secOut, err := runCheckUpdate(ctx, c.run, manager, "--security")
	if err != nil {
		slog.Default().Warn("telemetry: security-only patch check failed, reporting security count as 0",
			slog.String("manager", manager), slog.String("error", err.Error()))
	} else {
		security = parseCheckUpdateOutput(secOut)
	}

	return PatchCounts{Manager: manager, Total: total, Security: security}, nil
}

// runCheckUpdate runs "<manager> check-update [extraArgs...]" and treats
// exit code 100 (dnf/yum's documented "updates are available" signal) as
// success, not failure: only exec.CommandContext's *exec.ExitError case is
// inspected for the code, so a context cancellation or a binary that
// vanished mid-call still surfaces as a real error.
func runCheckUpdate(ctx context.Context, run commandRunner, manager string, extraArgs ...string) (string, error) {
	args := append([]string{"check-update"}, extraArgs...)
	out, err := run(ctx, manager, args...)
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 100 {
		return string(out), nil
	}
	return "", err
}

// parseAptUpgradable parses "apt list --upgradable" stdout. Each real
// package line looks like:
//
//	name/suite[,suite...] version arch [upgradable from: oldversion]
//
// A package can list multiple suites when it's available from more than
// one repo at once (a real, observed apt behavior, not a hypothetical);
// it counts as security-relevant if any of them ends in "-security".
// Anything else (the "Listing..." banner, blank lines, a locale-warning
// line with no "/") is silently skipped rather than counted or erroring:
// this must degrade, never crash, on unexpected apt output.
func parseAptUpgradable(output string) (total, security int) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		nameAndSuites := fields[0]
		slashIdx := strings.Index(nameAndSuites, "/")
		if slashIdx < 0 {
			continue
		}
		total++
		if strings.Contains(nameAndSuites[slashIdx+1:], "-security") {
			security++
		}
	}
	return total, security
}

// parseCheckUpdateOutput counts package lines in dnf/yum check-update
// output: "name.arch  version  repo", three or more whitespace-separated
// fields. Header/footer lines ("Last metadata expiration check: ...",
// "Obsoleting Packages", a blank separator line) never have three fields
// of that shape and are skipped, the same "unrecognized line means skip
// it, not count it or fail" defensiveness parseAptUpgradable uses.
func parseCheckUpdateOutput(output string) int {
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if !strings.Contains(fields[0], ".") {
			continue
		}
		count++
	}
	return count
}

// HostPatchCollector polls one host's available OS package updates on an
// interval and writes MetricOSPatchesAvailable/
// MetricOSSecurityPatchesAvailable samples under one resource ID, the
// same shape HostDiskCollector already establishes for host disk space.
type HostPatchCollector struct {
	checker    *HostPatchChecker
	resourceID string
	store      *DB
	interval   time.Duration
	logger     *slog.Logger
}

// NewHostPatchCollector builds a HostPatchCollector. checker defaults to
// NewHostPatchChecker() if nil, logger to slog.Default() if nil.
func NewHostPatchCollector(checker *HostPatchChecker, resourceID string, store *DB, interval time.Duration, logger *slog.Logger) *HostPatchCollector {
	if checker == nil {
		checker = NewHostPatchChecker()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HostPatchCollector{checker: checker, resourceID: resourceID, store: store, interval: interval, logger: logger}
}

// CollectOnce checks this host's available updates once and writes them.
// ErrNoSupportedPackageManager is not an error from this method's own
// perspective: it logs at Debug and returns nil without writing a
// sample, so a host with no supported package manager reports "unknown,
// not checked" (no recent sample) rather than a crashed collector or a
// false zero.
func (c *HostPatchCollector) CollectOnce(ctx context.Context) error {
	counts, err := c.checker.Check(ctx)
	if errors.Is(err, ErrNoSupportedPackageManager) {
		c.logger.Debug("telemetry: no supported package manager found, skipping OS patch check", slog.String("resource_id", c.resourceID))
		return nil
	}
	if err != nil {
		return fmt.Errorf("collect host patch status: %w", err)
	}

	now := time.Now()
	return c.store.WriteSamples(ctx, []Sample{
		{ResourceID: c.resourceID, Metric: MetricOSPatchesAvailable, Timestamp: now, Value: float64(counts.Total)},
		{ResourceID: c.resourceID, Metric: MetricOSSecurityPatchesAvailable, Timestamp: now, Value: float64(counts.Security)},
	})
}

// Run calls CollectOnce every interval until ctx is done, the same "log
// and keep going, one bad tick must not stop the collector" shape
// HostDiskCollector.Run already establishes.
func (c *HostPatchCollector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.CollectOnce(ctx); err != nil {
				c.logger.Warn("telemetry: host patch collection tick failed", slog.String("error", err.Error()))
			}
		}
	}
}
