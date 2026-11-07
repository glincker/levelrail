package api

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// hostPatchRunner runs fixed, absolute-path argv on the control plane's host; tests swap in a fake.
type hostPatchRunner interface {
	// Run never signals the child once started: ctx only bounds read-only simulations.
	Run(ctx context.Context, env []string, name string, args ...string) (output []byte, exitCode int, err error)
	FileExists(path string) bool
	// UnderSystemd reports whether installs can run as a transient unit outside the control plane's cgroup.
	UnderSystemd() bool
}

type realHostPatchRunner struct{}

func (realHostPatchRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, int, error) {
	// Installs pass a ctx with no deadline, so a timeout can never kill dpkg mid-transaction.
	cmd := exec.Command(name, args...) //nolint:gosec,noctx // absolute fixed binary, fixed flags plus validated package specs
	if _, ok := ctx.Deadline(); ok {
		cmd = exec.CommandContext(ctx, name, args...) //nolint:gosec // same fixed argv, read-only simulations only
	}
	cmd.Env = append(patchChildEnv(), env...)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, exitErr.ExitCode(), nil
	}
	if err != nil {
		return out, -1, fmt.Errorf("run %s: %w", name, err)
	}
	return out, 0, nil
}

func (realHostPatchRunner) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (r realHostPatchRunner) UnderSystemd() bool {
	return os.Getenv("INVOCATION_ID") != "" && r.FileExists("/run/systemd/system") && r.FileExists(patchBinSystemdRun) && r.FileExists(patchBinSystemctl)
}

const (
	patchManagerAPT     = "apt"
	patchManagerDNF     = "dnf"
	patchManagerYUM     = "yum"
	aptRebootMarkerPath = "/var/run/reboot-required"
	patchBinAptGet      = "/usr/bin/apt-get"
	patchBinDNF         = "/usr/bin/dnf"
	patchBinYUM         = "/usr/bin/yum"
	patchBinSystemdRun  = "/usr/bin/systemd-run"
	patchBinSystemctl   = "/usr/bin/systemctl"
	patchBinJournalctl  = "/usr/bin/journalctl"
	patchBinShutdown    = "/sbin/shutdown"
)

var (
	errNoPatchManager = errors.New("no supported package manager (apt, dnf or yum) on this host")
	// Debian and RPM names and versions; anything else in parsed output is dropped, never passed to argv.
	patchPackageName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._:~-]*$`)
	patchPackageVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._:~-]*$`)
	patchSecretKV       = regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization)\s*[=:]\s*)\S+`)
	patchURLCreds       = regexp.MustCompile(`(://[^/\s:@]+:)[^@\s/]+@`)
	aptInstLine         = regexp.MustCompile(`^Inst (\S+) (?:\[([^\]]+)\] )?\((\S+) (.*?)(?: \[[^\]]*\])?\)`)
	unitNameUnsafe      = regexp.MustCompile(`[^a-z0-9-]`)
)

// patchChildEnv is the whole child environment: the control plane's own env may hold the master key or LD_ vars.
func patchChildEnv() []string {
	return []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive",
		"APT_LISTCHANGES_FRONTEND=none", "NEEDRESTART_MODE=l", "NEEDRESTART_SUSPEND=1"}
}

// detectPatchManager mirrors HostPatchCollector's order so preview and status agree on one manager.
func detectPatchManager(r hostPatchRunner) (string, string, error) {
	for _, m := range [][2]string{{patchManagerAPT, patchBinAptGet}, {patchManagerDNF, patchBinDNF}, {patchManagerYUM, patchBinYUM}} {
		if r.FileExists(m[1]) {
			return m[0], m[1], nil
		}
	}
	return "", "", errNoPatchManager
}

// simulatePatches lists every upgradable package without changing the host.
func simulatePatches(ctx context.Context, r hostPatchRunner, manager, bin string, refresh bool) ([]nodePatchPackage, error) {
	switch manager {
	case patchManagerAPT:
		if refresh {
			if out, code, err := r.Run(ctx, nil, bin, "update", "-q"); err != nil || code != 0 {
				return nil, fmt.Errorf("apt-get update failed (exit %d): %w: %s", code, err, lastPatchLines(string(out), 5))
			}
		}
		out, code, err := r.Run(ctx, nil, bin, "-s", "-q", "upgrade")
		if err != nil || code != 0 {
			return nil, fmt.Errorf("apt-get -s upgrade failed (exit %d): %w: %s", code, err, lastPatchLines(string(out), 5))
		}
		return parseAptSimulation(string(out)), nil
	case patchManagerDNF, patchManagerYUM:
		args := []string{"-q", "check-update"}
		if refresh {
			args = append(args, "--refresh")
		}
		all, err := dnfCheckUpdate(ctx, r, bin, args)
		if err != nil {
			return nil, err
		}
		sec, err := dnfCheckUpdate(ctx, r, bin, append(args, "--security"))
		if err != nil {
			return nil, err
		}
		secure := map[string]bool{}
		for _, p := range sec {
			secure[p.Name] = true
		}
		for i := range all {
			all[i].Security = secure[all[i].Name]
		}
		return all, nil
	}
	return nil, errNoPatchManager
}

// dnfCheckUpdate treats exit 100 as "updates available", the documented dnf and yum signal.
func dnfCheckUpdate(ctx context.Context, r hostPatchRunner, bin string, args []string) ([]nodePatchPackage, error) {
	out, code, err := r.Run(ctx, nil, bin, args...)
	if err != nil || (code != 0 && code != 100) {
		return nil, fmt.Errorf("%s check-update failed (exit %d): %w: %s", bin, code, err, lastPatchLines(string(out), 5))
	}
	return parseCheckUpdate(string(out)), nil
}

// parseAptSimulation reads "Inst name [old] (new origin [arch])" lines of apt-get -s output.
func parseAptSimulation(out string) []nodePatchPackage {
	var pkgs []nodePatchPackage
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		m := aptInstLine.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil || !patchPackageName.MatchString(m[1]) || !patchPackageVersion.MatchString(m[3]) {
			continue
		}
		p := nodePatchPackage{Name: m[1], CurrentVersion: m[2], NewVersion: m[3], Source: m[4]}
		p.Security = strings.Contains(m[4], "-security") || strings.Contains(m[4], "/updates")
		classifyPatchPackage(&p)
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// parseCheckUpdate reads "name.arch version repo" rows and stops at the obsoletes section.
func parseCheckUpdate(out string) []nodePatchPackage {
	var pkgs []nodePatchPackage
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "Obsoleting") || strings.HasPrefix(line, "Security:") {
			break
		}
		f := strings.Fields(line)
		if len(f) != 3 || !strings.Contains(f[0], ".") || !patchPackageName.MatchString(f[0]) || !patchPackageVersion.MatchString(f[1]) {
			continue
		}
		p := nodePatchPackage{Name: f[0], NewVersion: f[1], Source: f[2]}
		classifyPatchPackage(&p)
		pkgs = append(pkgs, p)
	}
	return pkgs
}

var patchArchSuffixes = []string{".x86_64", ".aarch64", ".noarch", ".i686", ".armv7hl", ".ppc64le", ".s390x"}

func splitPatchArch(name string) (base, arch string) {
	for _, a := range patchArchSuffixes {
		if strings.HasSuffix(name, a) {
			return strings.TrimSuffix(name, a), a
		}
	}
	return name, ""
}

func classifyPatchPackage(p *nodePatchPackage) {
	base, _ := splitPatchArch(p.Name)
	for _, k := range []string{"linux-image", "linux-headers", "linux-modules", "linux-generic", "linux-virtual", "linux-firmware"} {
		if strings.HasPrefix(base, k) {
			p.Kernel = true
		}
	}
	if base == "kernel" || strings.HasPrefix(base, "kernel-") {
		p.Kernel = true
	}
	for _, k := range []string{"docker", "containerd", "runc", "moby"} {
		if base == k || strings.HasPrefix(base, k+"-") || strings.HasPrefix(base, k+".") {
			p.ContainerRuntime = true
		}
	}
}

// splitPatchPlan picks what an apply installs; kernel and container runtime packages wait for include_all.
func splitPatchPlan(all []nodePatchPackage, includeAll bool) (apply, held []nodePatchPackage, otherAvailable int) {
	apply, held = []nodePatchPackage{}, []nodePatchPackage{}
	for _, p := range all {
		if !includeAll && !p.Security {
			otherAvailable++
			continue
		}
		if !includeAll && (p.Kernel || p.ContainerRuntime) {
			held = append(held, p)
			continue
		}
		apply = append(apply, p)
	}
	return apply, held, otherAvailable
}

// patchPlanFingerprint binds a preview to its exact package list, so a changed host rejects the apply.
func patchPlanFingerprint(manager string, includeAll bool, apply, held []nodePatchPackage) string {
	lines := []string{manager, "all=" + strconv.FormatBool(includeAll)}
	for _, p := range apply {
		lines = append(lines, "a "+p.Name+"="+p.NewVersion)
	}
	for _, p := range held {
		lines = append(lines, "h "+p.Name+"="+p.NewVersion)
	}
	sort.Strings(lines[2:])
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// patchInstallArgv pins every package to its previewed version, so nothing outside the preview changes.
func patchInstallArgv(manager, bin string, pkgs []nodePatchPackage) []string {
	specs := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if !patchPackageName.MatchString(p.Name) || !patchPackageVersion.MatchString(p.NewVersion) {
			continue
		}
		if manager == patchManagerAPT {
			specs = append(specs, p.Name+"="+p.NewVersion)
			continue
		}
		base, arch := splitPatchArch(p.Name)
		specs = append(specs, base+"-"+p.NewVersion+arch)
	}
	if manager == patchManagerAPT {
		args := []string{bin, "install", "--only-upgrade", "--no-remove", "-y", "-q",
			"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
		return append(args, specs...)
	}
	return append([]string{bin, "-y", "-q", "upgrade"}, specs...)
}

// patchUnitName is the transient systemd unit an install runs in, outside the control plane's cgroup.
func patchUnitName(shortName, runID string) string {
	stem := unitNameUnsafe.ReplaceAllString(strings.ToLower(shortName), "")
	if stem == "" {
		stem = "os"
	}
	return stem + "-patch-" + unitNameUnsafe.ReplaceAllString(strings.ToLower(runID), "")
}

// patchSystemdRunArgv starts the install detached: a docker restart cannot take dpkg down with the control plane.
func patchSystemdRunArgv(unit string, install []string) []string {
	args := []string{"--unit=" + unit, "--description=OS package upgrade", "--quiet", "-p", "RemainAfterExit=yes"}
	for _, kv := range patchChildEnv() {
		args = append(args, "--setenv="+kv)
	}
	return append(append(args, "--"), install...)
}

type patchUnitState struct {
	done     bool
	lost     bool
	exitCode int
}

// parsePatchUnitShow reads "systemctl show" key=value output for the transient unit.
func parsePatchUnitShow(out string) patchUnitState {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			kv[k] = v
		}
	}
	code, _ := strconv.Atoi(kv["ExecMainStatus"])
	switch {
	case kv["ActiveState"] == "failed", kv["ActiveState"] == "active" && kv["SubState"] == "exited":
		// A failed unit or a non-success Result is never a success, whatever ExecMainStatus says.
		if code == 0 && (kv["ActiveState"] == "failed" || kv["Result"] != "success") {
			code = -1
		}
		return patchUnitState{done: true, exitCode: code}
	case kv["ActiveState"] == "inactive" && kv["LoadState"] != "loaded":
		return patchUnitState{done: true, lost: true, exitCode: -1}
	}
	return patchUnitState{}
}

func maskPatchOutput(s string) string {
	s = patchSecretKV.ReplaceAllString(s, "${1}****")
	return patchURLCreds.ReplaceAllString(s, "${1}****@")
}

func lastPatchLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return maskPatchOutput(strings.Join(lines, "\n"))
}
