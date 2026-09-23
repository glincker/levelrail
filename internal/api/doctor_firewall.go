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
// follow: a stuck firewall-tool invocation must never hang the whole
// doctor response.
func doctorCheckFirewallCtx(ctx context.Context) doctorCheckResource {
	checkCtx, cancel := context.WithTimeout(ctx, doctorPingTimeout)
	defer cancel()
	return doctorCheckFirewall(checkCtx, exec.LookPath, runRealFirewallCommand)
}

// firewallCommandRunner abstracts exec.CommandContext so tests can
// supply fixture output instead of shelling out to a real firewall
// binary, the same "fake-exec" seam internal/telemetry/hostpatch.go's
// own commandRunner establishes for a different package's system-
// command check.
type firewallCommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runRealFirewallCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // name/args are fixed literals chosen by this file, never caller input
}

// doctorFirewallDocsPath is the one guide every firewall backend's
// warn/fail Fix points at: the hardening checklist already covers ufw,
// firewalld, and iptables/nftables in one place.
const doctorFirewallDocsPath = "/security#fresh-box-hardening-checklist"

// doctorCheckFirewall is a read-only report of the local host's
// firewall status, trying each supported backend in turn (ufw,
// firewalld, nftables, iptables) and returning the first one actually
// installed. This check never mutates firewall state; the only place
// this codebase ever writes a firewall rule is install.sh's own opt-in
// LEVELRAIL_CONFIGURE_UFW step, never the running control plane. No
// supported tool installed (common on many distributions, and outside
// this platform's own Linux-only scope on anything else) is reported as
// informational, never a failure: this platform has no way to know
// whether an operator is relying on something it can't introspect
// (cloud security groups) instead.
func doctorCheckFirewall(ctx context.Context, lookPath func(string) (string, error), run firewallCommandRunner) doctorCheckResource {
	if check, found := doctorCheckUFW(ctx, lookPath, run); found {
		return check
	}
	if check, found := doctorCheckFirewalld(ctx, lookPath, run); found {
		return check
	}
	if check, found := doctorCheckNftables(ctx, lookPath, run); found {
		return check
	}
	if check, found := doctorCheckIptables(ctx, lookPath, run); found {
		return check
	}
	return doctorCheckResource{
		Code: "firewall", Name: "Firewall", Status: doctorStatusUnknown,
		Message: "no supported firewall tool (ufw, firewalld, nftables, iptables) found; if you rely on a different firewall (cloud security groups), this is expected",
	}
}

// doctorFirewallExitErrorUnknown reports the "found the tool, but the
// status command itself failed" case the same way across every
// backend below: usually a privilege problem, never something this
// check can distinguish further.
func doctorFirewallExitErrorUnknown(name, statusCmd string, err error) doctorCheckResource {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return doctorCheckResource{Code: "firewall", Name: name, Status: doctorStatusUnknown, Message: statusCmd + " exited non-zero, may need elevated privileges"}
	}
	return doctorCheckResource{Code: "firewall", Name: name, Status: doctorStatusUnknown, Message: err.Error()}
}

func doctorCheckUFW(ctx context.Context, lookPath func(string) (string, error), run firewallCommandRunner) (doctorCheckResource, bool) {
	const name = "Firewall (ufw)"
	if _, err := lookPath("ufw"); err != nil {
		return doctorCheckResource{}, false
	}

	out, err := run(ctx, "ufw", "status", "verbose")
	if err != nil {
		return doctorFirewallExitErrorUnknown(name, "ufw status", err), true
	}

	text := string(out)
	if !strings.Contains(text, "Status: active") {
		return doctorCheckResource{
			Code: "firewall", Name: name, Status: doctorStatusWarn,
			Message:  "ufw installed but inactive",
			Fix:      "sudo ufw allow 80,443/tcp && sudo ufw enable",
			DocsPath: doctorFirewallDocsPath,
		}, true
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
		return doctorCheckResource{
			Code: "firewall", Name: name, Status: doctorStatusWarn,
			Message:  "active, but default incoming policy is \"" + defaultIncoming + "\", not deny/reject",
			Fix:      "sudo ufw default deny incoming && sudo ufw allow 80,443/tcp",
			DocsPath: doctorFirewallDocsPath,
		}, true
	}
	return doctorCheckResource{Code: "firewall", Name: name, Status: doctorStatusOK, Message: "active, default incoming policy: " + defaultIncoming}, true
}

func doctorCheckFirewalld(ctx context.Context, lookPath func(string) (string, error), run firewallCommandRunner) (doctorCheckResource, bool) {
	const name = "Firewall (firewalld)"
	if _, err := lookPath("firewall-cmd"); err != nil {
		return doctorCheckResource{}, false
	}

	out, err := run(ctx, "firewall-cmd", "--state")
	if err != nil {
		return doctorFirewallExitErrorUnknown(name, "firewall-cmd --state", err), true
	}
	if strings.TrimSpace(string(out)) != "running" {
		return doctorCheckResource{
			Code: "firewall", Name: name, Status: doctorStatusWarn,
			Message:  "firewalld installed but not running",
			Fix:      "sudo systemctl enable --now firewalld && sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload",
			DocsPath: doctorFirewallDocsPath,
		}, true
	}

	// --query-service exits 0 with "yes" on stdout when open, non-zero
	// (still with output, ignored here) when not: only the happy path
	// needs distinguishing, everything else means "not confirmed open".
	httpOut, _ := run(ctx, "firewall-cmd", "--query-service=http")
	httpsOut, _ := run(ctx, "firewall-cmd", "--query-service=https")
	httpOpen := strings.TrimSpace(string(httpOut)) == "yes"
	httpsOpen := strings.TrimSpace(string(httpsOut)) == "yes"
	if !httpOpen || !httpsOpen {
		return doctorCheckResource{
			Code: "firewall", Name: name, Status: doctorStatusWarn,
			Message:  "firewalld running, but the http and/or https service isn't open",
			Fix:      "sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload",
			DocsPath: doctorFirewallDocsPath,
		}, true
	}
	return doctorCheckResource{Code: "firewall", Name: name, Status: doctorStatusOK, Message: "running, http and https services open"}, true
}

func doctorCheckNftables(ctx context.Context, lookPath func(string) (string, error), run firewallCommandRunner) (doctorCheckResource, bool) {
	const name = "Firewall (nftables)"
	if _, err := lookPath("nft"); err != nil {
		return doctorCheckResource{}, false
	}

	out, err := run(ctx, "nft", "list", "ruleset")
	if err != nil {
		return doctorFirewallExitErrorUnknown(name, "nft list ruleset", err), true
	}

	text := string(out)
	const fix = "sudo nft add rule inet filter input tcp dport { 80, 443 } accept"
	if strings.TrimSpace(text) == "" {
		return doctorCheckResource{
			Code: "firewall", Name: name, Status: doctorStatusWarn,
			Message: "nftables installed but no rules are loaded", Fix: fix, DocsPath: doctorFirewallDocsPath,
		}, true
	}
	if strings.Contains(text, "80") && strings.Contains(text, "443") {
		return doctorCheckResource{Code: "firewall", Name: name, Status: doctorStatusOK, Message: "rules loaded, ports 80/443 referenced in the ruleset"}, true
	}
	return doctorCheckResource{
		Code: "firewall", Name: name, Status: doctorStatusWarn,
		Message: "nftables has rules loaded, but ports 80/443 weren't found in them", Fix: fix, DocsPath: doctorFirewallDocsPath,
	}, true
}

func doctorCheckIptables(ctx context.Context, lookPath func(string) (string, error), run firewallCommandRunner) (doctorCheckResource, bool) {
	const name = "Firewall (iptables)"
	if _, err := lookPath("iptables"); err != nil {
		return doctorCheckResource{}, false
	}

	out, err := run(ctx, "iptables", "-S")
	if err != nil {
		return doctorFirewallExitErrorUnknown(name, "iptables -S", err), true
	}

	text := string(out)
	if strings.Contains(text, "--dport 80") && strings.Contains(text, "--dport 443") {
		return doctorCheckResource{Code: "firewall", Name: name, Status: doctorStatusOK, Message: "rules present, ports 80/443 referenced"}, true
	}
	return doctorCheckResource{
		Code: "firewall", Name: name, Status: doctorStatusWarn,
		Message: "iptables is this host's firewall tool, but no explicit rule for ports 80/443 was found",
		Fix: "sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT && sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT " +
			"(persist with iptables-persistent or your distribution's equivalent)",
		DocsPath: doctorFirewallDocsPath,
	}, true
}
