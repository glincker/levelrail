package api

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// doctorCheckFirewallCtx wraps doctorCheckFirewall with the real
// exec.LookPath/exec.CommandContext dependencies and a bounded timeout,
// the shape handleSystemDoctor's other checks (doctorPingTimeout) all
// follow: a stuck ufw invocation must never hang the whole doctor
// response.
func doctorCheckFirewallCtx(ctx context.Context) doctorCheckResource {
	checkCtx, cancel := context.WithTimeout(ctx, doctorPingTimeout)
	defer cancel()
	return doctorCheckFirewall(checkCtx, exec.LookPath, runRealFirewallCommand)
}

// firewallCommandRunner abstracts exec.CommandContext so tests can
// supply fixture output instead of shelling out to a real ufw binary,
// the same "fake-exec" seam internal/telemetry/hostpatch.go's own
// commandRunner establishes for a different package's system-command
// check.
type firewallCommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runRealFirewallCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // name/args are fixed literals chosen by this file, never caller input
}

// doctorCheckFirewall is a read-only report of the local host's UFW
// (Uncomplicated Firewall) status: whether it's installed, active, and
// its default incoming policy. This check never mutates firewall
// state; the only place this codebase ever writes a firewall rule is
// install.sh's own opt-in LEVELRAIL_CONFIGURE_UFW step, never the
// running control plane. ufw not installed (common on many
// distributions, and outside this platform's own Linux-only scope on
// anything else) is reported as informational, never a failure: this
// platform has no way to know whether an operator is relying on a
// different firewall (cloud security groups, firewalld, iptables
// directly) instead.
func doctorCheckFirewall(ctx context.Context, lookPath func(string) (string, error), run firewallCommandRunner) doctorCheckResource {
	const code, name = "firewall", "Firewall (ufw)"

	if _, err := lookPath("ufw"); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "ufw not installed; if you rely on a different firewall (cloud security groups, firewalld), this is expected"}
	}

	out, err := run(ctx, "ufw", "status", "verbose")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "ufw status exited non-zero, may need elevated privileges"}
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: err.Error()}
	}

	text := string(out)
	active := strings.Contains(text, "Status: active")
	if !active {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: "ufw installed but inactive"}
	}

	defaultIncoming := "unknown"
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Default:") {
			continue
		}
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "(incoming)," && i > 0 {
				defaultIncoming = fields[i-1]
			}
		}
	}

	if defaultIncoming != "deny" && defaultIncoming != "reject" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: "active, but default incoming policy is \"" + defaultIncoming + "\", not deny/reject"}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "active, default incoming policy: " + defaultIncoming}
}
