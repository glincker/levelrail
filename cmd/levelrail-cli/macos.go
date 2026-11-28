package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/glincker/levelrail/internal/localmode"
	"github.com/glincker/levelrail/internal/macosrt"
	"github.com/glincker/levelrail/internal/vmruntime"
	"github.com/glincker/levelrail/kit/diskspace"
)

// Tunables seed from env; defaults are the documented values.
const (
	envMacMinFreeGiB   = "APP_MACOS_MIN_FREE_GIB"
	envMacMinVersion   = "APP_MACOS_MIN_VERSION"
	envLaunchdThrottle = "APP_LAUNCHD_THROTTLE_SECONDS"
	defMacMinFreeGiB   = 40
	defMacMinVersion   = 13
	launchdUsageText   = `Usage:
  %[1]s launchd install <control-plane|agent> --binary PATH [--arg A]... [--env K=V]...
  %[1]s launchd uninstall <control-plane|agent>
  %[1]s launchd status [--json]
  %[1]s launchd print <control-plane|agent> --binary PATH [--arg A]... [--env K=V]...

Manages per-user launchd units (~/Library/LaunchAgents) on macOS. print writes the plist to
stdout and changes nothing. Secrets are refused in --env: put them in the service env file.
Respawn gap: APP_LAUNCHD_THROTTLE_SECONDS (30). Logs: ~/Library/Logs/Levelrail.
`
)

func hostMacDeps(lookup func(string) (string, bool)) macosrt.Deps {
	data := ""
	var ports []int
	if p, err := vmruntime.DefaultPath(); err == nil {
		data = filepath.Dir(p)
		if s, err := vmruntime.Load(p); err == nil {
			for _, port := range []int{s.DashboardPort, s.IngressPort} {
				if port > 0 {
					ports = append(ports, port)
				}
			}
		}
	}
	if data == "" {
		data, _ = os.UserHomeDir()
	}
	out := func(name string, args ...string) (string, error) {
		b, err := exec.CommandContext(context.Background(), name, args...).Output() //nolint:gosec // fixed argv
		return strings.TrimSpace(string(b)), err
	}
	return macosrt.Deps{
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, DataDir: data, Ports: ports,
		LookPath:      exec.LookPath,
		Sysctl:        func(n string) (string, error) { return out("sysctl", "-n", n) },
		Version:       func() (string, error) { return out("sw_vers", "-productVersion") },
		FreeBytes:     diskspace.Free,
		PortFree:      localmode.LoopbackFree,
		MinFreeGiB:    envInt(lookup, envMacMinFreeGiB, defMacMinFreeGiB),
		MinMacOSMajor: envInt(lookup, envMacMinVersion, defMacMinVersion),
	}
}

func macDoctorReport(d macosrt.Deps) systemDoctorResource {
	rep := systemDoctorResource{OK: true}
	for _, r := range macosrt.Run(d) {
		rep.Checks = append(rep.Checks, doctorCheckResource{Code: r.Code, Name: r.Name, Group: "macos",
			Status: r.Status, Message: r.Message, Fix: r.Fix})
		if r.Status == macosrt.StatusFail {
			rep.OK = false
		}
	}
	return rep
}

// runDoctorMacOS is offline: it checks this Mac can run the managed VM.
func runDoctorMacOS(prog string, args []string, stdout, stderr io.Writer, lookup func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" doctor macos", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rep := macDoctorReport(hostMacDeps(lookup))
	if *asJSON {
		if err := writeJSONValue(stdout, rep); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return vmExitFail
		}
	} else {
		printSystemDoctorHuman(stdout, rep)
	}
	if !rep.OK {
		return exitUsage
	}
	return exitOK
}

type launchdFlags struct {
	binary string
	args   multiFlag
	env    multiFlag
	asJSON bool
}

func (f launchdFlags) spec(role macosrt.Role, home string, lookup func(string) (string, bool)) (macosrt.Spec, error) {
	env := map[string]string{}
	for _, kv := range f.env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return macosrt.Spec{}, fmt.Errorf("--env %q must be K=V", kv)
		}
		env[k] = v
	}
	return macosrt.Spec{
		Role: role, Binary: f.binary, Args: f.args, Env: env,
		LogDir:          filepath.Join(home, "Library", "Logs", "Levelrail"),
		ThrottleSeconds: envInt(lookup, envLaunchdThrottle, macosrt.DefaultThrottleSeconds),
	}, nil
}

type launchctlRunner struct{}

func (launchctlRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "launchctl", args...).CombinedOutput() //nolint:gosec // fixed binary, argv built from labels
}

func runLaunchd(prog string, args []string, stdout, stderr io.Writer, lookup func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprintf(stdout, launchdUsageText, prog)
		return exitOK
	}
	sub, rest := args[0], args[1:]
	var f launchdFlags
	fs := flag.NewFlagSet(prog+" launchd "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&f.binary, "binary", "", "absolute path of the program to run")
	fs.Var(&f.args, "arg", "program argument (repeatable)")
	fs.Var(&f.env, "env", "K=V environment entry, no secrets (repeatable)")
	fs.BoolVar(&f.asJSON, "json", false, "status: print JSON")
	var roleArg string
	if sub != "status" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		roleArg, rest = rest[0], rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return exitUsage
	}
	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s launchd: %v\n", prog, err)
		return vmExitFail
	}
	var roles []macosrt.Role
	if sub == "status" {
		roles = []macosrt.Role{macosrt.RoleControlPlane, macosrt.RoleAgent}
	} else {
		role, rerr := macosrt.ParseRole(roleArg)
		if rerr != nil {
			_, _ = fmt.Fprintf(stderr, "%s launchd %s: %v\n", prog, sub, rerr)
			return exitUsage
		}
		roles = []macosrt.Role{role}
	}
	mgr := macosrt.Manager{Dir: filepath.Join(home, "Library", "LaunchAgents"), UID: os.Getuid(), Runner: launchctlRunner{}}
	ctx := context.Background()
	switch sub {
	case "print", "install":
		spec, err := f.spec(roles[0], home, lookup)
		if err == nil {
			err = spec.Validate()
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s launchd %s: %v\n", prog, sub, err)
			return exitUsage
		}
		if sub == "print" {
			raw, _ := spec.Plist()
			_, _ = stdout.Write(raw)
			return exitOK
		}
		if runtime.GOOS != "darwin" {
			_, _ = fmt.Fprintf(stderr, "%s launchd install: macOS only\n", prog)
			return exitUsage
		}
		p, err := mgr.Install(ctx, spec)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s launchd install: %v\n", prog, err)
			return vmExitFail
		}
		_, _ = fmt.Fprintf(stdout, "Installed and loaded %s (%s)\n", macosrt.Label(roles[0]), p)
	case "uninstall":
		removed, err := mgr.Uninstall(ctx, roles[0])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s launchd uninstall: %v\n", prog, err)
			return vmExitFail
		}
		if !removed {
			_, _ = fmt.Fprintln(stdout, "Nothing to remove.")
		} else {
			_, _ = fmt.Fprintf(stdout, "Unloaded and removed %s\n", macosrt.Label(roles[0]))
		}
	case "status":
		var states []macosrt.State
		for _, r := range roles {
			states = append(states, mgr.Status(ctx, r))
		}
		if f.asJSON {
			b, _ := json.MarshalIndent(states, "", "  ")
			_, _ = fmt.Fprintln(stdout, string(b))
			return exitOK
		}
		for _, s := range states {
			_, _ = fmt.Fprintf(stdout, "%-28s installed=%t loaded=%t\n", s.Label, s.Installed, s.Loaded)
		}
	default:
		_, _ = fmt.Fprintf(stderr, "%s launchd: unknown command %q (install, uninstall, status, print)\n", prog, sub)
		return exitUsage
	}
	return exitOK
}
