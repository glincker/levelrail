package main

import (
	"fmt"
	"io"
)

// runFirewall dispatches "firewall <verb> [flags]" to one of
// list/allow/deny/delete: managing the declarative host firewall rules
// internal/api/firewall_rules.go exposes (Settings -> Firewall in the
// web UI), reconciled onto the control plane's local ufw by
// internal/reconcile/firewall.
func runFirewall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, firewallUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, firewallUsage(prog))
		return exitOK
	case "status":
		return runFirewallHost(prog, "status", args[1:], stdout, stderr, lookupEnv)
	case "enable":
		return runFirewallHost(prog, "enable", args[1:], stdout, stderr, lookupEnv)
	case "disable":
		return runFirewallHost(prog, "disable", args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runFirewallList(prog, args[1:], stdout, stderr, lookupEnv)
	case "allow":
		return runFirewallAllow(prog, args[1:], stdout, stderr, lookupEnv)
	case "deny":
		return runFirewallDeny(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runFirewallDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown firewall subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, firewallUsage(prog))
		return exitUsage
	}
}

func firewallUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s firewall status [flags]                                          show whether the host firewall (ufw) is on and which ports it needs
  %[1]s firewall enable [--dry-run] [flags]                              allow SSH, 80, 443 and the control plane ports, then turn ufw on
  %[1]s firewall disable [--dry-run] [flags]                             turn ufw off
  %[1]s firewall list [flags]                                            list configured firewall rules
  %[1]s firewall allow --port N [--protocol tcp|udp] [--source-cidr CIDR] [--label TEXT] [flags]   add an allow rule
  %[1]s firewall deny --port N [--protocol tcp|udp] [--source-cidr CIDR] [--label TEXT] [flags]    add a deny rule
  %[1]s firewall delete <id> [flags]                                      remove a firewall rule

A deny rule, or an allow rule scoped to a source CIDR, targeting a port
this control plane itself needs (the management API, agent connections,
or ingress) is refused rather than applied.

Run "%[1]s firewall <subcommand> -h" for a subcommand's own flags.
`, prog)
}
