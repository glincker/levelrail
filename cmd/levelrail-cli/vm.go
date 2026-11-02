package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/localmode"
	"github.com/GLINCKER/levelrail/internal/vmruntime"
	"github.com/GLINCKER/levelrail/internal/vmruntime/guest"
	"github.com/GLINCKER/levelrail/internal/vmruntime/lima"
)

const (
	vmInstance     = "levelrail"
	vmGuestBin     = "/usr/local/bin/levelrail"
	vmGuestUnit    = "levelrail-controlplane.service"
	vmGuestToken   = "/etc/levelrail/desktop-token" //nolint:gosec // a file path, not a credential
	vmTokenFile    = "desktop-token"
	vmPollInterval = 2 * time.Second
	vmIngressFrom  = 8880
	vmPortProbes   = 50
	vmTokenBytes   = 32
	vmExitFail     = 1
)

var errVMUnsupported = errors.New("the managed VM is not supported on this platform yet (macOS only for now; use `levelrail dev` instead)")

// vmRunner is the only door to limactl, so tests never boot a VM.
type vmRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
	Interactive(ctx context.Context, args ...string) error
}

// vmEnv is everything a vm command reads from the host.
type vmEnv struct {
	runner       vmRunner
	settingsPath string
	dataDir      string
	homeDir      string
	hostMemMiB   int
	hostCPUs     int
	arch         string
	portFree     localmode.PortFree
	probe        func(ctx context.Context, port int) error
	sleep        func(time.Duration)
}

func (e *vmEnv) instanceDir() string { return filepath.Join(e.dataDir, "lima", vmInstance) }
func (e *vmEnv) tokenPath() string   { return filepath.Join(e.dataDir, vmTokenFile) }

const vmUsage = `Usage:
  %[1]s vm init [--memory MiB] [--cpus N] [--disk GiB] [--dashboard-port P] [--ingress-port P] [--control-plane-bin PATH] [--no-start]
  %[1]s vm start [--control-plane-bin PATH]
  %[1]s vm stop
  %[1]s vm status [--json]
  %[1]s vm shell   (a guest shell; the VM user has passwordless sudo, the VM is the boundary)
  %[1]s vm set <memory|cpus|disk|dashboard-port|ingress-port> <value>

Runs Levelrail in a private Linux VM with no host folders mounted (macOS, experimental).
Enable with APP_VM_EXPERIMENTAL=on. Settings live in a host file; env only seeds it at init.

Flags:
  --memory MiB             VM memory (env APP_VM_MEMORY_MIB, default 3072)
  --cpus N                 VM CPUs (env APP_VM_CPUS, default 2)
  --disk GiB               VM disk (env APP_VM_DISK_GIB, default 30)
  --dashboard-port P       host loopback dashboard port (env APP_VM_DASHBOARD_PORT, default: first free)
  --ingress-port P         host loopback app ingress port (env APP_VM_INGRESS_PORT, default: first free)
  --control-plane-bin PATH linux control plane binary to copy into the guest
  --no-start               init: create the VM but do not boot it
  --json                   status: print JSON

Limits: APP_VM_MEMORY_MAX_PERCENT (50), APP_VM_MEMORY_MIN_MIB (2048), APP_VM_BOOT_TIMEOUT (180s).
`

func runVM(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprintf(stdout, vmUsage, prog)
		return exitOK
	}
	// Parse flags first so a bad flag reports itself even while the gate is closed.
	if _, _, err := parseVMFlags(prog, args[0], args[1:], stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if !vmExperimental(lookupEnv) {
		_, _ = fmt.Fprintf(stderr, "%s vm: experimental. Enable it with APP_VM_EXPERIMENTAL=on (for example `APP_VM_EXPERIMENTAL=on %s vm init`).\n", prog, prog)
		return exitUsage
	}
	env, err := newVMEnv(lookupEnv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s vm: %v\n", prog, err)
		return vmExitFail
	}
	return vmDispatch(context.Background(), env, prog, args, stdout, stderr, lookupEnv)
}

func vmExperimental(env vmruntime.Lookup) bool {
	v, _ := env(vmruntime.EnvExperimental)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

type vmFlags struct {
	memory, cpus, disk, dashPort, ingPort int
	bin                                   string
	noStart, jsonOut                      bool
}

func parseVMFlags(prog, sub string, args []string, stderr io.Writer) (vmFlags, []string, error) {
	var f vmFlags
	fs := flag.NewFlagSet(prog+" vm "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	switch sub {
	case "init":
		fs.IntVar(&f.memory, "memory", 0, "VM memory in MiB (env APP_VM_MEMORY_MIB, default 3072)")
		fs.IntVar(&f.cpus, "cpus", 0, "VM CPUs (env APP_VM_CPUS, default 2)")
		fs.IntVar(&f.disk, "disk", 0, "VM disk in GiB (env APP_VM_DISK_GIB, default 30)")
		fs.IntVar(&f.dashPort, "dashboard-port", 0, "host loopback dashboard port (env APP_VM_DASHBOARD_PORT)")
		fs.IntVar(&f.ingPort, "ingress-port", 0, "host loopback app ingress port (env APP_VM_INGRESS_PORT)")
		fs.BoolVar(&f.noStart, "no-start", false, "create the VM but do not boot it")
		fs.StringVar(&f.bin, "control-plane-bin", "", "linux control plane binary to copy into the guest")
	case "start":
		fs.StringVar(&f.bin, "control-plane-bin", "", "linux control plane binary to copy into the guest")
	case "status":
		fs.BoolVar(&f.jsonOut, "json", false, "print the result as JSON")
	}
	if err := fs.Parse(args); err != nil {
		return f, nil, err
	}
	return f, fs.Args(), nil
}

func vmDispatch(ctx context.Context, env *vmEnv, prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub := args[0]
	switch sub {
	case "init", "start", "stop", "status", "shell", "set":
	default:
		_, _ = fmt.Fprintf(stderr, "%s vm: unknown command %q (init, start, stop, status, shell, set)\n", prog, sub)
		return exitUsage
	}
	f, rest, err := parseVMFlags(prog, sub, args[1:], stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if sub != "set" && len(rest) > 0 {
		_, _ = fmt.Fprintf(stderr, "%s vm %s: unexpected argument %q\n", prog, sub, rest[0])
		return exitUsage
	}
	var code int
	switch sub {
	case "init":
		err = env.init(ctx, f, stdout, lookupEnv)
	case "start":
		err = env.start(ctx, f.bin, stdout, lookupEnv)
	case "stop":
		err = env.stop(ctx, stdout)
	case "status":
		err = env.status(ctx, f.jsonOut, stdout, lookupEnv)
	case "shell":
		err = env.shell(ctx)
	case "set":
		if len(rest) != 2 {
			_, _ = fmt.Fprintf(stderr, "%s vm set: want <key> <value>\n", prog)
			return exitUsage
		}
		err = env.set(ctx, rest[0], rest[1], stdout, lookupEnv)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s vm %s: %v\n", prog, sub, err)
		code = vmExitFail
	}
	return code
}

// checkLimaHome refuses a private Lima home with _config: Lima merges default
// and override files from it into every instance, which could undo the isolation.
func (e *vmEnv) checkLimaHome() error {
	cfg := filepath.Join(e.dataDir, "lima", "_config")
	if _, err := os.Stat(cfg); err == nil {
		return fmt.Errorf("%s exists; remove it, Lima merges its files into every instance", cfg)
	}
	return nil
}

func warnImageOverride(w io.Writer) {
	if lima.ImageOverrideFromEnv() != "" {
		_, _ = fmt.Fprintln(w, "warning: image override active ("+lima.ImageSHAEnv+" is set); for testing only")
	}
}

func (e *vmEnv) limits(lookupEnv vmruntime.Lookup) (lima.Limits, error) {
	return vmruntime.Limits(lookupEnv, e.hostMemMiB, e.hostCPUs, e.homeDir, e.dataDir)
}

func (e *vmEnv) load() (vmruntime.Settings, error) {
	s, err := vmruntime.Load(e.settingsPath)
	if errors.Is(err, vmruntime.ErrNotFound) {
		return s, errors.New("not initialised; run `levelrail vm init` first")
	}
	return s, err
}

// vmInfo is the subset of `limactl list --json` we use.
type vmInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (e *vmEnv) instance(ctx context.Context) (*vmInfo, error) {
	out, err := e.runner.Run(ctx, "list", "--json")
	if err != nil {
		return nil, err
	}
	// limactl prints one JSON object per line.
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var v vmInfo
		if err := json.Unmarshal(line, &v); err != nil {
			return nil, fmt.Errorf("limactl list: %w", err)
		}
		if v.Name == vmInstance {
			return &v, nil
		}
	}
	return nil, nil
}

func pickPort(free localmode.PortFree, from int, used map[int]bool) (int, error) {
	for p := from; p < from+vmPortProbes; p++ {
		if !used[p] && free(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free loopback port near %d", from)
}

// sidecar describes the control plane unit; the binary is copied after first boot.
func vmSidecar(s vmruntime.Settings) guest.Sidecar {
	unit := fmt.Sprintf(`[Unit]
Description=Levelrail control plane
After=docker.service network-online.target
Wants=docker.service
ConditionPathExists=%[1]s

[Service]
ExecStart=%[1]s
Restart=on-failure
Environment=APP_LOCAL_MODE=1
Environment=APP_DESKTOP_MODE=1
Environment=APP_DESKTOP_ALLOWED_PORTS=%[5]d,%[6]d
Environment=APP_DESKTOP_TOKEN_FILE=%[2]s
Environment=APP_DATA_DIR=/var/lib/levelrail/data
Environment=APP_HTTP_ADDR=127.0.0.1:%[3]d
Environment=APP_INGRESS_HTTP_ADDR=127.0.0.1:%[4]d
Environment=APP_INGRESS_HTTP_REDIRECT=false
Environment=APP_PUBLIC_HOST=localhost
Environment=APP_CONTAINER_RUNTIME_SOCKET=unix:///var/run/docker.sock

[Install]
WantedBy=multi-user.target
`, vmGuestBin, vmGuestToken, lima.DefaultDashboardGuest, lima.DefaultIngressGuest, s.DashboardPort, s.IngressPort)
	return guest.Sidecar{BinaryPath: vmGuestBin, UnitName: vmGuestUnit, UnitContent: unit}
}

func (e *vmEnv) render(s vmruntime.Settings, l lima.Limits) ([]byte, error) {
	ls := s.ToLima()
	ls.Sidecar = vmSidecar(s)
	return lima.Render(ls, l, lima.RenderOptions{Arch: e.arch, ImageSHA256Override: lima.ImageOverrideFromEnv()})
}

func (e *vmEnv) ensureToken() error {
	if _, err := os.Stat(e.tokenPath()); err == nil {
		return nil
	}
	b := make([]byte, vmTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	if err := os.MkdirAll(e.dataDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(e.tokenPath(), []byte(hex.EncodeToString(b)+"\n"), 0o600)
}

func (e *vmEnv) init(ctx context.Context, f vmFlags, stdout io.Writer, lookupEnv vmruntime.Lookup) error {
	if err := e.checkLimaHome(); err != nil {
		return err
	}
	warnImageOverride(stdout)
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	if inst != nil {
		return errors.New("the VM already exists; use `vm set` to change settings")
	}
	s, err := vmruntime.Load(e.settingsPath)
	if errors.Is(err, vmruntime.ErrNotFound) {
		if s, err = vmruntime.SeedFromEnv(lookupEnv); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for _, o := range []struct {
		v   int
		dst *int
	}{{f.memory, &s.MemoryMiB}, {f.cpus, &s.CPUs}, {f.disk, &s.DiskGiB}, {f.dashPort, &s.DashboardPort}, {f.ingPort, &s.IngressPort}} {
		if o.v > 0 {
			*o.dst = o.v
		}
	}
	s.Applied = nil
	if err := e.choosePorts(&s); err != nil {
		return err
	}
	l, err := e.limits(lookupEnv)
	if err != nil {
		return err
	}
	yaml, err := e.render(s, l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(e.dataDir, "lima"), 0o700); err != nil {
		return err
	}
	if err := e.ensureToken(); err != nil {
		return err
	}
	tmpl := filepath.Join(e.dataDir, "levelrail.yaml")
	if err := os.WriteFile(tmpl, yaml, 0o600); err != nil {
		return err
	}
	if err := vmruntime.Save(e.settingsPath, s); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Creating the VM (%d MiB, %d CPUs, %d GiB disk)...\n", s.MemoryMiB, s.CPUs, s.DiskGiB)
	if _, err := e.runner.Run(ctx, "create", "--tty=false", "--name="+vmInstance, tmpl); err != nil {
		return err
	}
	if f.noStart {
		_, _ = fmt.Fprintln(stdout, "Created. Boot it with `levelrail vm start`.")
		return nil
	}
	return e.start(ctx, f.bin, stdout, lookupEnv)
}

// choosePorts fills unset ports and replaces taken ones, telling the caller via the settings.
func (e *vmEnv) choosePorts(s *vmruntime.Settings) error {
	used := map[int]bool{}
	if s.DashboardPort == 0 || !e.portFree(s.DashboardPort) {
		p, err := localmode.ChoosePorts(0, e.portFree)
		if err != nil {
			return err
		}
		s.DashboardPort = p.Dashboard
	}
	used[s.DashboardPort] = true
	if s.IngressPort == 0 || used[s.IngressPort] || !e.portFree(s.IngressPort) {
		p, err := pickPort(e.portFree, vmIngressFrom, used)
		if err != nil {
			return err
		}
		s.IngressPort = p
	}
	return nil
}

func (e *vmEnv) guestRead(ctx context.Context, path string) ([]byte, error) {
	return e.runner.Run(ctx, "shell", vmInstance, "--", "sudo", "cat", path)
}

// isolationRaw adds the freshness fields the guest writes on top of guest.Status.
type isolationRaw struct {
	BootID    string `json:"boot_id"`
	CheckedAt string `json:"checked_at"`
}

// checkFresh requires the status to come from this boot, so a file left on the
// disk by an earlier boot never counts as a pass.
func (e *vmEnv) checkFresh(ctx context.Context, raw []byte) error {
	var r isolationRaw
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, r.CheckedAt); err != nil {
		return errors.New("isolation status has no valid checked_at")
	}
	boot, err := e.runner.Run(ctx, "shell", vmInstance, "--", "cat", "/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	if r.BootID == "" || r.BootID != strings.TrimSpace(string(boot)) {
		return errors.New("isolation status is stale (boot id mismatch or missing)")
	}
	return nil
}

// waitIsolation polls the guest self-check; a fail or stale result returns at once.
func (e *vmEnv) waitIsolation(ctx context.Context, deadline time.Time) (guest.Status, error) {
	for {
		raw, err := e.guestRead(ctx, guest.StatusPath)
		if err == nil {
			st, perr := guest.ParseStatus(raw)
			if perr == nil {
				if ferr := e.checkFresh(ctx, raw); ferr != nil {
					return st, ferr
				}
				if !st.Passed() {
					return st, errors.New("guest isolation check failed")
				}
				return st, nil
			}
		}
		if time.Now().After(deadline) {
			return guest.Status{}, errors.New("guest isolation status did not appear in time")
		}
		e.sleep(vmPollInterval)
	}
}

func (e *vmEnv) start(ctx context.Context, bin string, stdout io.Writer, lookupEnv vmruntime.Lookup) error {
	if err := e.checkLimaHome(); err != nil {
		return err
	}
	warnImageOverride(stdout)
	s, err := e.load()
	if err != nil {
		return err
	}
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	if inst == nil {
		return errors.New("no VM found; run `levelrail vm init`")
	}
	timeout, err := vmruntime.BootTimeout(lookupEnv)
	if err != nil {
		return err
	}
	l, err := e.limits(lookupEnv)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	bctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if inst.Status != "Running" {
		if err := e.prepareStart(bctx, &s, l, stdout); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, "Starting the VM...")
		if _, err := e.runner.Run(bctx, "start", "--tty=false", vmInstance); err != nil {
			e.failClosed(ctx)
			return fmt.Errorf("start failed, VM stopped: %w", err)
		}
	}
	if _, err := e.waitIsolation(bctx, deadline); err != nil {
		e.failClosed(ctx)
		return fmt.Errorf("%w; the VM was stopped (limit %s)", err, timeout)
	}
	if err := e.installBinary(bctx, bin); err != nil {
		e.failClosed(ctx)
		return fmt.Errorf("%w; the VM was stopped", err)
	}
	url := vmruntime.DashboardURL(s.DashboardPort)
	if err := e.waitDashboard(bctx, s.DashboardPort, deadline); err != nil {
		return fmt.Errorf("%w (the VM is running; check `levelrail vm status`)", err)
	}
	s.Applied = &vmruntime.Applied{MemoryMiB: s.MemoryMiB, CPUs: s.CPUs, DiskGiB: s.DiskGiB, DashboardPort: s.DashboardPort, IngressPort: s.IngressPort}
	if err := vmruntime.Save(e.settingsPath, s); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Dashboard: %s\nDesktop token file: %s\n", url, e.tokenPath())
	return nil
}

// prepareStart re-renders the instance config so stored settings take effect.
func (e *vmEnv) prepareStart(ctx context.Context, s *vmruntime.Settings, l lima.Limits, stdout io.Writer) error {
	oldD, oldI := s.DashboardPort, s.IngressPort
	if err := e.choosePorts(s); err != nil {
		return err
	}
	if s.DashboardPort != oldD || s.IngressPort != oldI {
		_, _ = fmt.Fprintf(stdout, "A configured port was taken; using dashboard %d and ingress %d instead.\n", s.DashboardPort, s.IngressPort)
		if err := vmruntime.Save(e.settingsPath, *s); err != nil {
			return err
		}
	}
	yaml, err := e.render(*s, l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.instanceDir(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(e.instanceDir(), "lima.yaml"), yaml, 0o600); err != nil {
		return err
	}
	if s.Applied != nil && s.DiskGiB > s.Applied.DiskGiB {
		_, err = e.runner.Run(ctx, "edit", "--tty=false", "--disk="+strconv.Itoa(s.DiskGiB), vmInstance)
	}
	return err
}

// failClosed stops the VM on its own context, since the boot one may be expired.
func (e *vmEnv) failClosed(ctx context.Context) {
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	_, _ = e.runner.Run(sctx, "stop", "--force", vmInstance)
}

func (e *vmEnv) installBinary(ctx context.Context, bin string) error {
	if bin == "" {
		if _, err := e.runner.Run(ctx, "shell", vmInstance, "--", "test", "-x", vmGuestBin); err == nil {
			return nil
		}
		return errors.New("no control plane in the guest; pass --control-plane-bin PATH (a linux build)")
	}
	steps := [][]string{
		{"copy", bin, vmInstance + ":/tmp/levelrail-cp"},
		{"shell", vmInstance, "--", "sudo", "install", "-m", "0755", "/tmp/levelrail-cp", vmGuestBin},
		{"shell", vmInstance, "--", "sudo", "install", "-d", "-m", "0700", filepath.Dir(vmGuestToken)},
	}
	for _, a := range steps {
		if _, err := e.runner.Run(ctx, a...); err != nil {
			return err
		}
	}
	if _, err := e.runner.Run(ctx, "copy", e.tokenPath(), vmInstance+":/tmp/desktop-token"); err != nil {
		return err
	}
	for _, a := range [][]string{
		{"shell", vmInstance, "--", "sudo", "install", "-m", "0600", "/tmp/desktop-token", vmGuestToken},
		{"shell", vmInstance, "--", "sudo", "systemctl", "restart", vmGuestUnit},
	} {
		if _, err := e.runner.Run(ctx, a...); err != nil {
			return err
		}
	}
	return nil
}

func (e *vmEnv) waitDashboard(ctx context.Context, port int, deadline time.Time) error {
	for {
		if err := e.probe(ctx, port); err == nil {
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("dashboard did not answer on port %d in time", port)
		}
		e.sleep(vmPollInterval)
	}
}

// httpProbe dials loopback but sends the desktop Host name, which is all the
// control plane accepts.
func httpProbe(ctx context.Context, port int) error {
	p := strconv.Itoa(port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+p+"/", nil)
	if err != nil {
		return err
	}
	req.Host = vmruntime.DashboardHost + ":" + p
	c := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (e *vmEnv) stop(ctx context.Context, stdout io.Writer) error {
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	if inst == nil {
		return errors.New("no VM found; run `levelrail vm init`")
	}
	if inst.Status != "Running" {
		_, _ = fmt.Fprintln(stdout, "The VM is not running.")
		return nil
	}
	if _, err := e.runner.Run(ctx, "stop", vmInstance); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Stopped.")
	return nil
}

func (e *vmEnv) shell(ctx context.Context) error {
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	if inst == nil || inst.Status != "Running" {
		return errors.New("the VM is not running; run `levelrail vm start`")
	}
	return e.runner.Interactive(ctx, "shell", vmInstance)
}

var vmNotes = []string{
	"containers with network_mode host are denied in the VM (firewall output chain; deployer rejection is a follow-up)",
	"the VM user has passwordless sudo inside the guest; the VM is the boundary",
}

type vmStatusReport struct {
	State         string        `json:"state"`
	MemoryMiB     int           `json:"memory_mib"`
	CPUs          int           `json:"cpus"`
	DiskGiB       int           `json:"disk_gib"`
	DashboardURL  string        `json:"dashboard_url"`
	IngressPort   int           `json:"ingress_port"`
	Forwards      []string      `json:"forwards"`
	Shares        []string      `json:"shares"`
	Pending       []string      `json:"pending_restart"`
	EnvDiffers    []string      `json:"env_differs,omitempty"`
	Isolation     string        `json:"isolation"`
	IsolationInfo []guest.Check `json:"isolation_checks,omitempty"`
	DiskUsage     string        `json:"disk_usage,omitempty"`
	ImageOverride bool          `json:"image_override"`
	Notes         []string      `json:"notes"`
}

func (e *vmEnv) status(ctx context.Context, jsonOut bool, stdout io.Writer, lookupEnv vmruntime.Lookup) error {
	s, err := e.load()
	if err != nil {
		return err
	}
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	r := vmStatusReport{
		State: "not created", MemoryMiB: s.MemoryMiB, CPUs: s.CPUs, DiskGiB: s.DiskGiB,
		DashboardURL: vmruntime.DashboardURL(s.DashboardPort), IngressPort: s.IngressPort,
		Forwards: []string{fmt.Sprintf("127.0.0.1:%d -> dashboard (%s)", s.DashboardPort, vmruntime.DashboardHost), fmt.Sprintf("127.0.0.1:%d -> app ingress", s.IngressPort)},
		Shares:   []string{}, Pending: s.Pending(), Isolation: "unknown (VM not running)",
		ImageOverride: lima.ImageOverrideFromEnv() != "", Notes: vmNotes,
	}
	if r.Pending == nil {
		r.Pending = []string{}
	}
	seed, serr := vmruntime.SeedFromEnv(lookupEnv)
	if serr == nil {
		for _, d := range []struct {
			name, envName string
			env, stored   int
		}{
			{"memory", vmruntime.EnvMemoryMiB, seed.MemoryMiB, s.MemoryMiB},
			{"cpus", vmruntime.EnvCPUs, seed.CPUs, s.CPUs},
			{"disk", vmruntime.EnvDiskGiB, seed.DiskGiB, s.DiskGiB},
		} {
			if v, ok := lookupEnv(d.envName); ok && strings.TrimSpace(v) != "" && d.env != d.stored {
				r.EnvDiffers = append(r.EnvDiffers, d.name)
			}
		}
	}
	if inst != nil {
		r.State = strings.ToLower(inst.Status)
	}
	if inst != nil && inst.Status == "Running" {
		r.Isolation = "no result"
		if raw, rerr := e.guestRead(ctx, guest.StatusPath); rerr == nil {
			if st, perr := guest.ParseStatus(raw); perr == nil {
				r.Isolation = st.Result
				r.IsolationInfo = st.Checks
				if !st.Passed() {
					r.Isolation = "fail"
				}
			} else {
				r.Isolation = "unreadable"
			}
		}
		if out, derr := e.runner.Run(ctx, "shell", vmInstance, "--", "df", "-h", "--output=used,size,pcent", "/"); derr == nil {
			r.DiskUsage = strings.Join(strings.Fields(strings.ReplaceAll(string(out), "\n", " ")), " ")
		}
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	_, _ = fmt.Fprintf(stdout, "State:      %s\nMemory:     %d MiB\nCPUs:       %d\nDisk:       %d GiB\nDashboard:  %s\nIsolation:  %s\n", r.State, r.MemoryMiB, r.CPUs, r.DiskGiB, r.DashboardURL, r.Isolation)
	for _, c := range r.IsolationInfo {
		_, _ = fmt.Fprintf(stdout, "  check %-24s %s\n", c.Name, c.Result)
	}
	for _, f := range r.Forwards {
		_, _ = fmt.Fprintf(stdout, "Forward:    %s\n", f)
	}
	_, _ = fmt.Fprintln(stdout, "Shares:     none")
	if r.DiskUsage != "" {
		_, _ = fmt.Fprintf(stdout, "Disk use:   %s\n", r.DiskUsage)
	}
	if r.ImageOverride {
		_, _ = fmt.Fprintln(stdout, "Warning:    image override active (testing only)")
	}
	for _, n := range r.Notes {
		_, _ = fmt.Fprintf(stdout, "Note:       %s\n", n)
	}
	if len(r.Pending) > 0 {
		_, _ = fmt.Fprintf(stdout, "Pending:    %s (applies on next start)\n", strings.Join(r.Pending, ", "))
	}
	for _, d := range r.EnvDiffers {
		_, _ = fmt.Fprintf(stdout, "Note:       env differs from the stored %s value; env only seeds, use `vm set`\n", d)
	}
	return nil
}

func (e *vmEnv) set(ctx context.Context, key, value string, stdout io.Writer, lookupEnv vmruntime.Lookup) error {
	s, err := e.load()
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("%s: %q is not a whole number", key, value)
	}
	next := s
	switch key {
	case "memory":
		next.MemoryMiB = n
	case "cpus":
		next.CPUs = n
	case "disk":
		if n < s.DiskGiB {
			return fmt.Errorf("disk can only grow (currently %d GiB)", s.DiskGiB)
		}
		next.DiskGiB = n
	case "dashboard-port":
		next.DashboardPort = n
	case "ingress-port":
		next.IngressPort = n
	default:
		return fmt.Errorf("unknown key %q (memory, cpus, disk, dashboard-port, ingress-port)", key)
	}
	l, err := e.limits(lookupEnv)
	if err != nil {
		return err
	}
	if err := next.Validate(l); err != nil {
		return err
	}
	if err := vmruntime.Save(e.settingsPath, next); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "%s set to %d.\n", key, n)
	if inst, ierr := e.instance(ctx); ierr == nil && inst != nil && inst.Status == "Running" {
		_, _ = fmt.Fprintln(stdout, "Restart needed: run `levelrail vm stop` then `levelrail vm start`.")
	} else {
		_, _ = fmt.Fprintln(stdout, "Applies on the next `levelrail vm start`.")
	}
	return nil
}

func vmHostArch() string {
	if runtime.GOARCH == "amd64" {
		return "x86_64"
	}
	return "aarch64"
}
