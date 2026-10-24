package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/localmode"
)

const (
	devReadyTimeout   = 90 * time.Second
	devReadyPoll      = 500 * time.Millisecond
	devStopTimeout    = 40 * time.Second
	devChildGrace     = 30 * time.Second
	devRestartExit    = 75
	devLogFileName    = "control-plane.log"
	devLogTailLines   = 20
	envDevDataDir     = "APP_DEV_DATA_DIR"
	envDevControlPlan = "APP_DEV_CONTROL_PLANE_BIN"
)

type devOptions struct {
	sub      string
	port     int
	dataDir  string
	noOpen   bool
	reset    bool
	bin      string
	jsonOut  bool
	helpOnly bool
}

func parseDevArgs(prog string, args []string, stderr io.Writer) (devOptions, error) {
	var o devOptions
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		o.sub, args = args[0], args[1:]
	}
	switch o.sub {
	case "", "start", "stop", "status":
	default:
		return o, fmt.Errorf("unknown dev command %q (use stop or status)", o.sub)
	}
	fs := flag.NewFlagSet(prog+" dev", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&o.port, "port", 0, "dashboard port on 127.0.0.1 (default: 8080, or the next free port)")
	fs.StringVar(&o.dataDir, "data-dir", "", "where the local instance keeps its data (default: a per-user directory, env "+envDevDataDir+")")
	fs.BoolVar(&o.noOpen, "no-open", false, "do not open the dashboard in a browser")
	fs.BoolVar(&o.reset, "reset", false, "delete the local instance's data and start fresh")
	fs.StringVar(&o.bin, "bin", "", "control plane binary to run (default: next to this binary, then PATH, env "+envDevControlPlan+")")
	fs.BoolVar(&o.jsonOut, "json", false, "status: print the result as JSON")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, devUsage(prog)) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			o.helpOnly = true
			return o, nil
		}
		return o, fmt.Errorf("parse flags: %w", err)
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if o.port < 0 || o.port > 65535 {
		return o, fmt.Errorf("--port %d is out of range", o.port)
	}
	return o, nil
}

func devUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s dev [flags]       run a private instance on this machine, open its dashboard
  %[1]s dev status        show whether it is running and where
  %[1]s dev stop          shut it down

Runs the control plane bound to 127.0.0.1 with its own data directory, generates
an admin sign-in (printed once, kept in the data directory with mode 0600) and
serves deployed apps at <app>.localhost over plain http. Needs Docker running.
Ctrl-C stops it cleanly.

Flags:
  --port N          dashboard port (default 8080, or the next free port)
  --data-dir DIR    data directory (default: per-user, env %[2]s)
  --no-open         do not open the dashboard in a browser
  --reset           delete the local instance's data first
  --bin PATH        control plane binary (default: next to this binary, then PATH, env %[3]s)
  --json            status: print JSON
`, prog, envDevDataDir, envDevControlPlan)
}

func defaultDevDataDir(prog string, lookupEnv func(string) (string, bool)) (string, error) {
	if v, ok := lookupEnv(envDevDataDir); ok && strings.TrimSpace(v) != "" {
		return filepath.Abs(v)
	}
	stem := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(prog), ".exe"), "-cli")
	if x, ok := lookupEnv("XDG_DATA_HOME"); ok && strings.TrimSpace(x) != "" {
		return filepath.Join(x, stem+"-dev"), nil
	}
	home, ok := lookupEnv("HOME")
	if !ok || strings.TrimSpace(home) == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
	}
	return filepath.Join(home, ".local", "share", stem+"-dev"), nil
}

// controlPlaneBinary finds the control plane executable: explicit setting, then beside the CLI, then PATH.
func controlPlaneBinary(prog, flagBin string, lookupEnv func(string) (string, bool), self string, exists func(string) bool, lookPath func(string) (string, error)) (string, error) {
	explicit := strings.TrimSpace(flagBin)
	if v, ok := lookupEnv(envDevControlPlan); explicit == "" && ok {
		explicit = strings.TrimSpace(v)
	}
	if explicit != "" {
		if !exists(explicit) {
			return "", fmt.Errorf("control plane binary %q does not exist", explicit)
		}
		return explicit, nil
	}
	stem := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(prog), ".exe"), "-cli")
	name := stem
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if self != "" {
		if p := filepath.Join(filepath.Dir(self), name); exists(p) {
			return p, nil
		}
	}
	if p, err := lookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("control plane binary %q not found next to %s or on PATH; install it or pass --bin", name, prog)
}

type devEnvConfig struct {
	DataDir    string
	Ports      localmode.Ports
	Admin      localmode.Admin
	DockerSock string
}

// devChildEnv layers the local-instance settings over base and drops settings that would defeat them.
func devChildEnv(base []string, c devEnvConfig) []string {
	drop := map[string]bool{
		"APP_DEV_MODE": true, "APP_PUBLIC_HOST": true, "APP_DASHBOARD_URL": true, "APP_ALLOW_INSECURE_LOGIN": true,
		"APP_HTTP_ADDR": true, "APP_INGRESS_HTTP_ADDR": true, "APP_INGRESS_HTTPS_ADDR": true, "APP_DATA_DIR": true,
		"APP_ADMIN_USERNAME": true, "APP_ADMIN_PASSWORD": true, "APP_INGRESS_HTTP_REDIRECT": true, "APP_AGENT_ADDR": true,
		localmode.EnvEnabled: true, localmode.EnvSupervised: true, "APP_CONTAINER_RUNTIME_SOCKET": true,
	}
	out := make([]string, 0, len(base)+12)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			out = append(out, kv)
		}
	}
	loop := func(port int) string { return "127.0.0.1:" + strconv.Itoa(port) }
	return append(out,
		localmode.EnvEnabled+"=1",
		localmode.EnvSupervised+"=1",
		"APP_DATA_DIR="+c.DataDir,
		"APP_HTTP_ADDR="+loop(c.Ports.Dashboard),
		"APP_INGRESS_HTTP_ADDR="+loop(c.Ports.HTTP),
		"APP_INGRESS_HTTPS_ADDR="+loop(c.Ports.HTTPS),
		"APP_AGENT_ADDR="+loop(c.Ports.Agent),
		"APP_INGRESS_HTTP_REDIRECT=false",
		"APP_PUBLIC_HOST=localhost",
		"APP_ALLOW_INSECURE_LOGIN=true",
		"APP_ADMIN_USERNAME="+c.Admin.Username,
		"APP_ADMIN_PASSWORD="+c.Admin.Password,
		"APP_CONTAINER_RUNTIME_SOCKET=unix://"+c.DockerSock,
	)
}

func runDev(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	o, err := parseDevArgs(prog, args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev: %v\n", prog, err)
		return exitUsage
	}
	if o.helpOnly {
		return exitOK
	}
	if o.dataDir == "" {
		if o.dataDir, err = defaultDevDataDir(prog, lookupEnv); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s dev: %v\n", prog, err)
			return exitValidation
		}
		if o.sub == "stop" || o.sub == "status" {
			if dir, ok := followCustomDevDir(o.dataDir); ok {
				o.dataDir = dir
			}
		}
	}
	switch o.sub {
	case "stop":
		return devStop(prog, o, stdout, stderr)
	case "status":
		return devStatus(prog, o, stdout, stderr)
	}
	return devStart(prog, o, stdout, stderr, lookupEnv)
}

func devStart(prog string, o devOptions, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if st, ok, _ := localmode.ReadState(o.dataDir); ok && processAlive(st.PID) {
		_, _ = fmt.Fprintf(stderr, "%s dev: already running (pid %d) at %s; use \"%s dev stop\" first\n", prog, st.PID, st.Dashboard, prog)
		return exitValidation
	}
	sock, err := localmode.DetectDocker(localmode.RealDockerEnv(runtime.GOOS))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev: %v\n", prog, err)
		return exitValidation
	}
	self, _ := os.Executable()
	bin, err := controlPlaneBinary(prog, o.bin, lookupEnv, self, func(p string) bool { _, e := os.Stat(p); return e == nil }, exec.LookPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev: %v\n", prog, err)
		return exitValidation
	}
	if o.reset {
		if err := localmode.Wipe(o.dataDir); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s dev: reset: %v\n", prog, err)
			return exitValidation
		}
	}
	if err := os.MkdirAll(o.dataDir, 0o750); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev: create data directory: %v\n", prog, err)
		return exitValidation
	}
	if def, derr := defaultDevDataDir(prog, lookupEnv); derr == nil {
		if err := recordCustomDevDir(def, o.dataDir); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s dev: warning: could not record data dir for stop: %v\n", prog, err)
		}
	}
	ports, err := localmode.ChoosePorts(o.port, localmode.LoopbackFree)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev: %v\n", prog, err)
		return exitValidation
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sup := &devSupervisor{prog: prog, opts: o, bin: bin, sock: sock, ports: ports, stdout: stdout, stderr: stderr}
	return sup.run(ctx)
}

type devSupervisor struct {
	prog           string
	opts           devOptions
	bin, sock      string
	ports          localmode.Ports
	stdout, stderr io.Writer
}

func (s *devSupervisor) run(ctx context.Context) int {
	first := true
	for {
		admin, created, err := localmode.LoadOrCreateAdmin(s.opts.dataDir, nil)
		if err != nil {
			_, _ = fmt.Fprintf(s.stderr, "%s dev: %v\n", s.prog, err)
			return exitValidation
		}
		code, restart := s.runOnce(ctx, admin, created, first)
		first = false
		if !restart {
			localmode.ClearState(s.opts.dataDir)
			return code
		}
		if localmode.ResetRequested(s.opts.dataDir) {
			_, _ = fmt.Fprintln(s.stdout, "reset requested from the dashboard: wiping local data")
			if err := localmode.Wipe(s.opts.dataDir); err != nil {
				_, _ = fmt.Fprintf(s.stderr, "%s dev: reset: %v\n", s.prog, err)
				return exitValidation
			}
		}
		if err := os.MkdirAll(s.opts.dataDir, 0o750); err != nil {
			_, _ = fmt.Fprintf(s.stderr, "%s dev: %v\n", s.prog, err)
			return exitValidation
		}
	}
}

// runOnce runs one control plane process; restart is true when it exited asking to be relaunched.
func (s *devSupervisor) runOnce(ctx context.Context, admin localmode.Admin, created, first bool) (code int, restart bool) {
	logPath := filepath.Join(s.opts.dataDir, devLogFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // path is under the operator's own data dir
	if err != nil {
		_, _ = fmt.Fprintf(s.stderr, "%s dev: open log: %v\n", s.prog, err)
		return exitValidation, false
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, s.bin) //nolint:gosec // the binary is the operator's own control plane
	cmd.Env = devChildEnv(os.Environ(), devEnvConfig{DataDir: s.opts.dataDir, Ports: s.ports, Admin: admin, DockerSock: s.sock})
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = devChildGrace
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(s.stderr, "%s dev: start control plane: %v\n", s.prog, err)
		return exitNetwork, false
	}
	var waitErr error
	exited := make(chan struct{})
	go func() { waitErr = cmd.Wait(); close(exited) }()

	url := localmode.DashboardURL(s.ports.Dashboard)
	if err := waitDevReady(ctx, url, exited); err != nil {
		if ctx.Err() == nil {
			_, _ = fmt.Fprintf(s.stderr, "%s dev: control plane did not become ready: %v\n%s\nfull log: %s\n", s.prog, err, tailFile(logPath, devLogTailLines), logPath)
			_ = cmd.Process.Signal(os.Interrupt)
			<-exited
			return exitNetwork, false
		}
		<-exited
		return exitOK, false
	}
	st := localmode.State{PID: os.Getpid(), DataDir: s.opts.dataDir, Dashboard: url, HTTPPort: s.ports.HTTP, HTTPSPort: s.ports.HTTPS, Started: time.Now().UTC()}
	if err := localmode.WriteState(st); err != nil {
		_, _ = fmt.Fprintf(s.stderr, "%s dev: %v\n", s.prog, err)
	}
	if first || created {
		printDevSummary(s.stdout, s.prog, st, admin, created, logPath)
	}
	if first && !s.opts.noOpen {
		if err := openBrowser(context.WithoutCancel(ctx), url+"/login"); err != nil {
			_, _ = fmt.Fprintf(s.stdout, "open %s in your browser (could not launch one: %v)\n", url, err)
		}
	}

	<-exited
	werr := waitErr
	if ctx.Err() != nil {
		_, _ = fmt.Fprintln(s.stdout, "stopped")
		return exitOK, false
	}
	var ee *exec.ExitError
	if errors.As(werr, &ee) && ee.ExitCode() == devRestartExit {
		return exitOK, true
	}
	_, _ = fmt.Fprintf(s.stderr, "%s dev: control plane exited: %v\n%s\nfull log: %s\n", s.prog, werr, tailFile(logPath, devLogTailLines), logPath)
	return exitNetwork, false
}

func printDevSummary(w io.Writer, prog string, st localmode.State, a localmode.Admin, created bool, logPath string) {
	_, _ = fmt.Fprintf(w, "\nLocal instance is running.\n  dashboard:  %s\n  data dir:   %s\n  apps:       %s (plain http on loopback)\n  log:        %s\n",
		st.Dashboard, st.DataDir, localmode.AppURL("<app>.localhost", st.HTTPPort), logPath)
	if created {
		_, _ = fmt.Fprintf(w, "  sign in:    %s / %s   (shown once, kept in %s)\n", a.Username, a.Password, localmode.AdminFilePath(st.DataDir))
	} else {
		_, _ = fmt.Fprintf(w, "  sign in:    %s, password in %s\n", a.Username, localmode.AdminFilePath(st.DataDir))
	}
	_, _ = fmt.Fprintf(w, "\nPress Ctrl-C to stop, or run \"%s dev stop\" from another terminal.\n", prog)
}

func waitDevReady(ctx context.Context, base string, exited <-chan struct{}) error {
	deadline := time.NewTimer(devReadyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(devReadyPoll)
	defer tick.Stop()
	for {
		if devHealthy(ctx, base) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted: %w", ctx.Err())
		case <-exited:
			return errors.New("the process exited during startup")
		case <-deadline.C:
			return fmt.Errorf("no response from %s after %s", base, devReadyTimeout)
		case <-tick.C:
		}
	}
}

func devHealthy(ctx context.Context, base string) bool {
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, base+"/api/v1/brand", nil)
	if err != nil {
		return false
	}
	resp, err := devHTTPClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func tailFile(path string, n int) string {
	raw, err := os.ReadFile(path) //nolint:gosec // path is under the operator's own data dir
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

type devStatusResult struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Dashboard string `json:"dashboard_url,omitempty"`
	AppsPort  int    `json:"apps_http_port,omitempty"`
	DataDir   string `json:"data_dir"`
	AdminFile string `json:"admin_file,omitempty"`
}

func devStatus(prog string, o devOptions, stdout, stderr io.Writer) int {
	res := devStatusResult{DataDir: o.dataDir}
	st, ok, err := localmode.ReadState(o.dataDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev status: %v\n", prog, err)
		return exitNetwork
	}
	if ok && processAlive(st.PID) {
		res = devStatusResult{Running: true, PID: st.PID, Dashboard: st.Dashboard, AppsPort: st.HTTPPort, DataDir: st.DataDir, AdminFile: localmode.AdminFilePath(st.DataDir)}
	}
	if o.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s dev status: %v\n", prog, err)
			return exitNetwork
		}
		return exitOK
	}
	if !res.Running {
		_, _ = fmt.Fprintf(stdout, "not running (data dir %s)\nstart it with: %s dev\n", res.DataDir, prog)
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "running (pid %d)\n  dashboard: %s\n  apps:      %s\n  data dir:  %s\n  sign in:   password in %s\n",
		res.PID, res.Dashboard, localmode.AppURL("<app>.localhost", res.AppsPort), res.DataDir, res.AdminFile)
	return exitOK
}

func devStop(prog string, o devOptions, stdout, stderr io.Writer) int {
	st, ok, err := localmode.ReadState(o.dataDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev stop: %v\n", prog, err)
		return exitNetwork
	}
	if !ok || !processAlive(st.PID) {
		localmode.ClearState(o.dataDir)
		_, _ = fmt.Fprintf(stdout, "not running (data dir %s)\nif it was started with a custom directory, pass the same --data-dir\n", o.dataDir)
		return exitOK
	}
	if err := terminateProcess(st.PID); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s dev stop: %v\n", prog, err)
		return exitNetwork
	}
	deadline := time.Now().Add(devStopTimeout)
	for time.Now().Before(deadline) {
		if !processAlive(st.PID) {
			_, _ = fmt.Fprintln(stdout, "stopped")
			return exitOK
		}
		time.Sleep(devReadyPoll)
	}
	_, _ = fmt.Fprintf(stderr, "%s dev stop: pid %d is still shutting down after %s\n", prog, st.PID, devStopTimeout)
	return exitNetwork
}

var devHTTPClient = &http.Client{Timeout: 2 * time.Second}

func openBrowser(ctx context.Context, url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url) //nolint:gosec // fixed binary, loopback url built by this package
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // fixed binary, loopback url built by this package
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url) //nolint:gosec // fixed binary, loopback url built by this package
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

const devLastDirFile = "last-custom-dir"

// recordCustomDevDir leaves a pointer in the default dir when an instance
// starts elsewhere, so stop and status without --data-dir can find it.
func recordCustomDevDir(defaultDir, dir string) error {
	ptr := filepath.Join(defaultDir, devLastDirFile)
	if filepath.Clean(dir) == filepath.Clean(defaultDir) {
		if err := os.Remove(ptr); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear data dir pointer: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(defaultDir, 0o750); err != nil {
		return fmt.Errorf("create default data dir: %w", err)
	}
	if err := os.WriteFile(ptr, []byte(dir), 0o600); err != nil {
		return fmt.Errorf("write data dir pointer: %w", err)
	}
	return nil
}

// followCustomDevDir returns the recorded custom dir only while an
// instance there is still alive, and only when the default has none.
func followCustomDevDir(defaultDir string) (string, bool) {
	if st, ok, _ := localmode.ReadState(defaultDir); ok && processAlive(st.PID) {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(defaultDir, devLastDirFile)) //nolint:gosec // fixed file name under the per-user default dir
	if err != nil {
		return "", false
	}
	dir := strings.TrimSpace(string(b))
	if dir == "" || !filepath.IsAbs(dir) {
		return "", false
	}
	if st, ok, _ := localmode.ReadState(dir); ok && processAlive(st.PID) {
		return dir, true
	}
	return "", false
}
