package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/buildernode"
	"github.com/GLINCKER/levelrail/internal/vmruntime"
)

const builderUsage = `Usage:
  %[1]s builder join --server URL --token T [--name N] [--agent-addr HOST:PORT] [--ca-fingerprint SHA256] [limit flags]
  %[1]s builder start [--agent-bin PATH] [--no-watch] [limit flags]
  %[1]s builder stop
  %[1]s builder status [--json]
  %[1]s builder doctor [--json]
  %[1]s builder logout

Turns this machine into a build-only node: a private Lima VM runs the agent and BuildKit,
and the agent dials out to the control plane over mTLS. Needs a builder join token
(%[1]s nodes join-token --builder). Limits are stored at join and can be changed at start.

Limit flags (env seeds the defaults):
  --max-cpu N              VM CPUs (env APP_BUILDER_MAX_CPU)
  --max-mem MiB            VM memory (env APP_BUILDER_MAX_MEM_MIB)
  --idle-stop DURATION     stop the VM after this long idle, 0 disables (env APP_BUILDER_IDLE_STOP)
  --pause-on-battery       pause builds while on battery (env APP_BUILDER_PAUSE_ON_BATTERY)

Doctor limits: APP_BUILDER_MIN_RAM_MIB, APP_BUILDER_MIN_DISK_GIB, APP_BUILDER_MAX_CLOCK_SKEW.
Watch limits: APP_BUILDER_POLL_INTERVAL, APP_BUILDER_IDLE_LOAD. Agent port: APP_BUILDER_AGENT_PORT.
`

type builderFlags struct {
	server, token, name, agentAddr, ca, agentBin string
	maxCPU, maxMem                               int
	idle                                         time.Duration
	battery, noWatch, jsonOut                    bool
}

func parseBuilderFlags(prog, sub string, args []string, stderr io.Writer, lookup func(string) (string, bool)) (builderFlags, error) {
	var f builderFlags
	lim, err := buildernode.LookupLimits(lookup)
	if err != nil {
		return f, err
	}
	fs := flag.NewFlagSet(prog+" builder "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	limits := func() {
		fs.IntVar(&f.maxCPU, "max-cpu", lim.MaxCPU, "VM CPUs (env APP_BUILDER_MAX_CPU)")
		fs.IntVar(&f.maxMem, "max-mem", lim.MaxMemMiB, "VM memory in MiB (env APP_BUILDER_MAX_MEM_MIB)")
		fs.DurationVar(&f.idle, "idle-stop", lim.IdleStop, "stop the VM after this long idle (env APP_BUILDER_IDLE_STOP)")
		fs.BoolVar(&f.battery, "pause-on-battery", lim.PauseOnBattery, "pause builds on battery (env APP_BUILDER_PAUSE_ON_BATTERY)")
	}
	switch sub {
	case "join":
		fs.StringVar(&f.server, "server", "", "control plane URL")
		fs.StringVar(&f.token, "token", "", "builder join token")
		fs.StringVar(&f.name, "name", "", "node name (default: hostname)")
		fs.StringVar(&f.agentAddr, "agent-addr", "", "control plane agent address host:port (default: server host and APP_BUILDER_AGENT_PORT)")
		fs.StringVar(&f.ca, "ca-fingerprint", "", "pin the control plane CA by SHA-256 fingerprint")
		limits()
	case "start":
		fs.StringVar(&f.agentBin, "agent-bin", "", "linux levelrail-agent binary to copy into the guest")
		fs.BoolVar(&f.noWatch, "no-watch", false, "return after boot instead of enforcing idle stop and battery pause")
		limits()
	case "status", "doctor":
		fs.BoolVar(&f.jsonOut, "json", false, "print the result as JSON")
	}
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return f, nil
}

// builderStarted records which limit flags the user actually passed to start.
func builderOverrides(prog string, args []string) map[string]bool {
	set := map[string]bool{}
	fs := flag.NewFlagSet(prog, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int("max-cpu", 0, "")
	fs.Int("max-mem", 0, "")
	fs.Duration("idle-stop", 0, "")
	fs.Bool("pause-on-battery", false, "")
	fs.String("agent-bin", "", "")
	fs.Bool("no-watch", false, "")
	_ = fs.Parse(args)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

func runBuilder(prog string, args []string, stdout, stderr io.Writer, lookup func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprintf(stdout, builderUsage, prog)
		return exitOK
	}
	sub := args[0]
	switch sub {
	case "join", "start", "stop", "status", "doctor", "logout":
	default:
		_, _ = fmt.Fprintf(stderr, "%s builder: unknown command %q (join, start, stop, status, doctor, logout)\n", prog, sub)
		return exitUsage
	}
	f, err := parseBuilderFlags(prog, sub, args[1:], stderr, lookup)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		_, _ = fmt.Fprintf(stderr, "%s builder %s: %v\n", prog, sub, err)
		return exitUsage
	}
	if sub == "start" && !vmExperimental(lookup) {
		_, _ = fmt.Fprintf(stderr, "%s builder start: experimental. Enable it with APP_VM_EXPERIMENTAL=on.\n", prog)
		return exitUsage
	}
	dir, err := builderDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s builder: %v\n", prog, err)
		return vmExitFail
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store := buildernode.NewFileStore(dir)
	switch sub {
	case "join":
		_, err = buildernode.Join(dir, store, buildernode.JoinInput{Server: f.server, Token: f.token, NodeName: f.name, AgentAddr: f.agentAddr, CAFingerprint: f.ca,
			Limits: buildernode.Limits{MaxCPU: f.maxCPU, MaxMemMiB: f.maxMem, IdleStop: f.idle, PauseOnBattery: f.battery}}, lookup)
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "Joined. Run `%s builder doctor`, then `%s builder start --agent-bin PATH`.\n", prog, prog)
		}
	case "logout":
		if err = runBuilderStop(ctx, store, stdout, lookup); err == nil {
			err = buildernode.Logout(dir, store)
		}
		if err == nil {
			_, _ = fmt.Fprintln(stdout, "Logged out. Remove the node in the dashboard to revoke its certificate.")
		}
	case "doctor":
		err = runBuilderDoctor(ctx, dir, f.jsonOut, stdout, lookup)
	case "status":
		err = runBuilderStatus(ctx, dir, store, f.jsonOut, stdout, lookup)
	case "stop":
		err = runBuilderStop(ctx, store, stdout, lookup)
	case "start":
		err = runBuilderStart(ctx, prog, dir, store, f, builderOverrides(prog, args[1:]), stdout, lookup)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s builder %s: %v\n", prog, sub, err)
		return vmExitFail
	}
	return exitOK
}

func newBuilderHost(store buildernode.CredStore, stdout io.Writer, lookup func(string) (string, bool)) (*buildernode.Host, error) {
	env, err := newVMEnv(lookup)
	if err != nil {
		return nil, err
	}
	return &buildernode.Host{
		Runner:      env.runner,
		Store:       store,
		InstanceDir: filepath.Join(env.limaDir(), buildernode.Instance),
		Render: func(cpus, mem int) ([]byte, error) {
			s, serr := vmruntime.SeedFromEnv(lookup)
			if serr != nil {
				return nil, serr
			}
			s.CPUs, s.MemoryMiB = cpus, mem
			if perr := env.choosePorts(&s); perr != nil {
				return nil, perr
			}
			l, lerr := env.limits(lookup)
			if lerr != nil {
				return nil, lerr
			}
			return env.render(s, l)
		},
		OnBattery: onBattery,
		Sleep:     time.Sleep,
		Env:       lookup,
		Out:       stdout,
	}, nil
}

func runBuilderStart(ctx context.Context, prog, dir string, store buildernode.CredStore, f builderFlags, set map[string]bool, stdout io.Writer, lookup func(string) (string, bool)) error {
	c, err := buildernode.LoadConfig(dir)
	if err != nil {
		return err
	}
	if set["max-cpu"] {
		c.MaxCPU = f.maxCPU
	}
	if set["max-mem"] {
		c.MaxMemMiB = f.maxMem
	}
	if set["idle-stop"] {
		c.IdleStop = ""
		if f.idle > 0 {
			c.IdleStop = f.idle.String()
		}
	}
	if set["pause-on-battery"] {
		c.PauseOnBattery = f.battery
	}
	if err := buildernode.SaveConfig(dir, c); err != nil {
		return err
	}
	h, err := newBuilderHost(store, stdout, lookup)
	if err != nil {
		return err
	}
	if err := h.Start(ctx, c, f.agentBin); err != nil {
		return err
	}
	if f.noWatch || (c.IdleStop == "" && !c.PauseOnBattery) {
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "Watching idle and power state; leave this running or use `%s builder stop`.\n", prog)
	return h.Supervise(ctx, c)
}

func runBuilderStop(ctx context.Context, store buildernode.CredStore, stdout io.Writer, lookup func(string) (string, bool)) error {
	h, err := newBuilderHost(store, stdout, lookup)
	if err != nil {
		if errors.Is(err, errVMUnsupported) {
			return nil
		}
		return err
	}
	return h.Stop(ctx)
}

func runBuilderStatus(ctx context.Context, dir string, store buildernode.CredStore, asJSON bool, stdout io.Writer, lookup func(string) (string, bool)) error {
	h, err := newBuilderHost(store, stdout, lookup)
	if err != nil {
		return err
	}
	s, err := h.Status(ctx, dir)
	if err != nil {
		return err
	}
	remote := fetchBuilderView(ctx, s, lookup)
	if asJSON {
		return json.NewEncoder(stdout).Encode(builderStatusView{Status: s, Remote: remote})
	}
	if !s.Joined {
		_, _ = fmt.Fprintln(stdout, "Not joined. Run `builder join` first.")
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "Server: %s\nNode: %s\nVM: %s\nAgent: %s\nLimits: %d CPU, %d MiB\n", s.Server, s.Node, s.VM, s.Agent, s.MaxCPU, s.MaxMem)
	if remote != nil {
		_, _ = fmt.Fprintf(stdout, "Status: %s\nTier: %s\nPlatform: %s\nSlots: %d/%d\nLast seen: %s\nAvg build: %s\n",
			remote.Status, remote.Tier, fmtPlatform(remote.BuilderResource), remote.ActiveBuilds, remote.SlotsTotal, fmtSeen(remote.LastSeenAt), fmtBuildMS(remote.AvgBuildMS))
	}
	return nil
}

// builderStatusView adds the control plane's view of this builder to the local status.
type builderStatusView struct {
	buildernode.Status
	Remote *apiclient.BuilderDetailResource `json:"remote,omitempty"`
}

// fetchBuilderView is best effort: without an API token or a reachable server only local fields show.
func fetchBuilderView(ctx context.Context, s buildernode.Status, lookup func(string) (string, bool)) *apiclient.BuilderDetailResource {
	if !s.Joined || s.Server == "" || s.Node == "" {
		return nil
	}
	token := resolveToken("", lookup, "levelrail", resolveProfile("", lookup))
	if token == "" {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	d, err := NewClient(s.Server, token).GetBuilder(cctx, s.Node)
	if err != nil {
		return nil
	}
	return &d
}

func runBuilderDoctor(ctx context.Context, dir string, asJSON bool, stdout io.Writer, lookup func(string) (string, bool)) error {
	probe, err := vmruntimeDuration(lookup, buildernode.EnvProbeTimeout, buildernode.DefaultProbe)
	if err != nil {
		return err
	}
	var runner buildernode.Runner
	if env, eerr := newVMEnv(lookup); eerr == nil {
		runner = env.runner
	} else {
		runner = failRunner{eerr}
	}
	de := buildernode.DoctorEnv{
		Arch: runtime.GOARCH, HostMemMiB: hostMemMiB(), DiskFreeGiB: freeDiskGiB(dir2(dir)),
		LimaPath: lookupLimactl, Runner: runner, ServerTime: serverTime(probe), Now: time.Now, Env: lookup,
	}
	if c, cerr := buildernode.LoadConfig(dir); cerr == nil {
		de.ConfigServer = c.Server
	}
	checks, err := buildernode.Doctor(ctx, de)
	if err != nil {
		return err
	}
	if asJSON {
		if err := json.NewEncoder(stdout).Encode(checks); err != nil {
			return err
		}
	} else {
		for _, c := range checks {
			mark := "ok  "
			if !c.OK {
				mark = "FAIL"
			}
			_, _ = fmt.Fprintf(stdout, "%s %-14s %s\n", mark, c.Name, c.Detail)
			if c.Fix != "" {
				_, _ = fmt.Fprintf(stdout, "     fix: %s\n", c.Fix)
			}
		}
	}
	if !buildernode.Passed(checks) {
		return errors.New("one or more checks failed")
	}
	return nil
}

type failRunner struct{ err error }

func (f failRunner) Run(context.Context, ...string) ([]byte, error) { return nil, f.err }

// dir2 returns the nearest existing directory so a statfs works before the first join.
func dir2(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || filepath.Dir(d) == d {
			return d
		}
	}
}

func vmruntimeDuration(lookup func(string) (string, bool), name string, def time.Duration) (time.Duration, error) {
	raw, ok := lookup(name)
	if !ok || raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", name, raw)
	}
	return d, nil
}
