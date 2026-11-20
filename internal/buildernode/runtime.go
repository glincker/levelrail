package buildernode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Guest paths. The identity file lives inside the VM so the mTLS key never touches the host disk.
const (
	guestAgentBin = "/usr/local/bin/levelrail-agent"
	guestIdentity = "/var/lib/levelrail-agent/identity.json"
	guestEnvFile  = "/etc/levelrail-agent/agent.env"
	guestUnit     = "levelrail-agent.service"
	guestUnitPath = "/etc/systemd/system/" + guestUnit
)

const agentUnit = `[Unit]
Description=Levelrail build-only agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
EnvironmentFile=` + guestEnvFile + `
ExecStart=` + guestAgentBin + `
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

// Host is the VM side of a builder. Every outside effect is injected.
type Host struct {
	Runner      Runner
	Store       CredStore
	InstanceDir string
	Render      func(cpus, memMiB int) ([]byte, error)
	OnBattery   func(ctx context.Context) (bool, error)
	Sleep       func(time.Duration)
	Env         Lookup
	Out         io.Writer
}

func (h *Host) sh(ctx context.Context, args ...string) ([]byte, error) {
	return h.Runner.Run(ctx, append([]string{"shell", Instance, "--"}, args...)...)
}

// Start boots the VM and the agent. agentBin is a linux agent build, copied in when given.
func (h *Host) Start(ctx context.Context, c Config, agentBin string) error {
	st, err := VMState(ctx, h.Runner)
	if err != nil {
		return err
	}
	if st != "Running" {
		yaml, rerr := h.Render(c.MaxCPU, c.MaxMemMiB)
		if rerr != nil {
			return rerr
		}
		if err := os.MkdirAll(h.InstanceDir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(h.InstanceDir, "lima.yaml"), yaml, 0o600); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(h.Out, "Starting the builder VM...")
		if _, err := h.Runner.Run(ctx, "start", "--tty=false", Instance); err != nil {
			_, _ = h.Runner.Run(ctx, "stop", Instance)
			return fmt.Errorf("start failed, VM stopped: %w", err)
		}
	}
	if err := h.installAgent(ctx, agentBin); err != nil {
		return err
	}
	used, err := h.writeEnv(ctx, c)
	if err != nil {
		return err
	}
	for _, a := range [][]string{{"sudo", "systemctl", "daemon-reload"}, {"sudo", "systemctl", "enable", guestUnit}, {"sudo", "systemctl", "restart", guestUnit}} {
		if _, err := h.sh(ctx, a...); err != nil {
			return fmt.Errorf("agent service: %w", err)
		}
	}
	if used {
		// The token is one-time and spent once the agent enrolls, so drop the host copy.
		if err := h.Store.Delete(); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(h.Out, "Builder agent started.")
	return nil
}

func (h *Host) installAgent(ctx context.Context, bin string) error {
	if bin == "" {
		if _, err := h.sh(ctx, "test", "-x", guestAgentBin); err != nil {
			return errors.New("no agent in the guest; pass --agent-bin PATH (a linux levelrail-agent build)")
		}
		return nil
	}
	if _, err := h.Runner.Run(ctx, "copy", bin, Instance+":/tmp/levelrail-agent"); err != nil {
		return err
	}
	_, err := h.sh(ctx, "sudo", "install", "-m", "0755", "/tmp/levelrail-agent", guestAgentBin)
	return err
}

// writeEnv installs the agent env file; it reports whether the join token went into it.
func (h *Host) writeEnv(ctx context.Context, c Config) (bool, error) {
	var b strings.Builder
	line := func(k, v string) { b.WriteString(k + "=" + v + "\n") }
	line("APP_CONTROL_PLANE_ADDR", c.AgentAddr)
	line("APP_NODE_NAME", c.NodeName)
	line("APP_AGENT_IDENTITY_FILE", guestIdentity)
	if c.CAFingerprint != "" {
		line("APP_CA_FINGERPRINT", c.CAFingerprint)
	}
	used := false
	if _, err := h.sh(ctx, "sudo", "test", "-f", guestIdentity); err != nil {
		tok, lerr := h.Store.Load()
		if lerr != nil {
			return false, lerr
		}
		if tok == "" {
			return false, errors.New("this builder is not enrolled and no join token is stored; run `levelrail builder join` again")
		}
		line("APP_JOIN_TOKEN", tok)
		used = true
	}
	dir, err := os.MkdirTemp("", "builder-env-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	envPath, unitPath := filepath.Join(dir, "agent.env"), filepath.Join(dir, "agent.service")
	if err := os.WriteFile(envPath, []byte(b.String()), 0o600); err != nil {
		return false, err
	}
	if err := os.WriteFile(unitPath, []byte(agentUnit), 0o600); err != nil {
		return false, err
	}
	steps := [][]string{
		{"copy", envPath, Instance + ":/tmp/agent.env"},
		{"copy", unitPath, Instance + ":/tmp/agent.service"},
	}
	for _, a := range steps {
		if _, err := h.Runner.Run(ctx, a...); err != nil {
			return false, err
		}
	}
	for _, a := range [][]string{
		{"sudo", "mkdir", "-p", "/etc/levelrail-agent", "/var/lib/levelrail-agent"},
		{"sudo", "install", "-m", "0600", "/tmp/agent.env", guestEnvFile},
		{"sudo", "install", "-m", "0644", "/tmp/agent.service", guestUnitPath},
		{"rm", "-f", "/tmp/agent.env", "/tmp/agent.service"},
	} {
		if _, err := h.sh(ctx, a...); err != nil {
			return false, err
		}
	}
	return used, nil
}

// Stop stops the agent, then the VM. A VM that does not exist is already stopped.
func (h *Host) Stop(ctx context.Context) error {
	st, err := VMState(ctx, h.Runner)
	if err != nil || st != "Running" {
		return err
	}
	_, _ = h.sh(ctx, "sudo", "systemctl", "stop", guestUnit)
	_, err = h.Runner.Run(ctx, "stop", Instance)
	return err
}

// Status is the host view of the builder.
type Status struct {
	Joined  bool   `json:"joined"`
	Server  string `json:"server,omitempty"`
	Node    string `json:"node,omitempty"`
	VM      string `json:"vm"`
	Agent   string `json:"agent"`
	MaxCPU  int    `json:"max_cpu,omitempty"`
	MaxMem  int    `json:"max_mem_mib,omitempty"`
	Battery bool   `json:"pause_on_battery"`
	IdleMin string `json:"idle_stop,omitempty"`
}

// Status reads config, VM and agent state.
func (h *Host) Status(ctx context.Context, dir string) (Status, error) {
	var s Status
	c, err := LoadConfig(dir)
	if err == nil {
		s = Status{Joined: true, Server: c.Server, Node: c.NodeName, MaxCPU: c.MaxCPU, MaxMem: c.MaxMemMiB, Battery: c.PauseOnBattery, IdleMin: c.IdleStop}
	} else if !errors.Is(err, ErrNotJoined) {
		return s, err
	}
	st, err := VMState(ctx, h.Runner)
	if err != nil {
		return s, err
	}
	s.VM, s.Agent = st, "stopped"
	if s.VM == "" {
		s.VM = "absent"
	}
	if st == "Running" {
		if out, aerr := h.sh(ctx, "systemctl", "is-active", guestUnit); aerr == nil {
			s.Agent = strings.TrimSpace(string(out))
		}
	}
	return s, nil
}

// Supervise enforces pause-on-battery and the idle auto-stop until ctx ends or it stops the VM.
func (h *Host) Supervise(ctx context.Context, c Config) error {
	poll, err := durEnv(h.Env, EnvPollInterval, DefaultPoll)
	if err != nil {
		return err
	}
	idleLoad, err := floatEnv(h.Env, EnvIdleLoad, DefaultIdleLoad)
	if err != nil {
		return err
	}
	var idleFor time.Duration
	idleStop, _ := time.ParseDuration(c.IdleStop)
	paused := false
	for ctx.Err() == nil {
		if c.PauseOnBattery && h.OnBattery != nil {
			bat, berr := h.OnBattery(ctx)
			if berr == nil && bat != paused {
				verb := "start"
				if bat {
					verb = "stop"
				}
				if _, err := h.sh(ctx, "sudo", "systemctl", verb, guestUnit); err != nil {
					return err
				}
				paused = bat
			}
		}
		if idleStop > 0 && !paused {
			if busy, lerr := h.busy(ctx, idleLoad); lerr == nil && !busy {
				idleFor += poll
			} else {
				idleFor = 0
			}
			if idleFor >= idleStop {
				_, _ = fmt.Fprintf(h.Out, "Idle for %s, stopping the builder VM.\n", idleStop)
				return h.Stop(ctx)
			}
		}
		h.Sleep(poll)
	}
	return nil
}

func (h *Host) busy(ctx context.Context, threshold float64) (bool, error) {
	out, err := h.sh(ctx, "cat", "/proc/loadavg")
	if err != nil {
		return true, err
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return true, errors.New("empty loadavg")
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return true, err
	}
	return v > threshold, nil
}

func floatEnv(env Lookup, name string, def float64) (float64, error) {
	raw, ok := env(name)
	raw = strings.TrimSpace(raw)
	if !ok || raw == "" {
		return def, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number, got %q", name, raw)
	}
	return v, nil
}
