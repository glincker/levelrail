package main

import (
	"context"
	"fmt"
	"io"
)

// runFirewallAllow implements "firewall allow": POST
// /api/v1/firewall-rules with action=allow.
func runFirewallAllow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runFirewallCreate(prog, "allow", "allow", args, stdout, stderr, lookupEnv)
}

// runFirewallDeny implements "firewall deny": POST /api/v1/firewall-rules
// with action=deny. A deny rule, or an allow rule scoped to a source
// CIDR, targeting a port this control plane itself needs is refused by
// the server with a clear error, same as the dashboard.
func runFirewallDeny(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runFirewallCreate(prog, "deny", "deny", args, stdout, stderr, lookupEnv)
}

func runFirewallCreate(prog, verb, action string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall "+verb, "print the created firewall rule as JSON to stdout and nothing else", stderr)
	var port int
	var protocol, sourceCIDR, label string
	fs.IntVar(&port, "port", 0, "port number, 1-65535 (required)")
	fs.StringVar(&protocol, "protocol", "tcp", "\"tcp\" or \"udp\"")
	fs.StringVar(&sourceCIDR, "source-cidr", "", "restrict the rule to this source CIDR (default: any source)")
	fs.StringVar(&label, "label", "", "optional note describing what this rule is for")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, firewallCreateUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if port < 1 || port > 65535 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--port is required and must be between 1 and 65535"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateFirewallRule(context.Background(), createFirewallRuleRequest{
		Port: port, Protocol: protocol, SourceCIDR: sourceCIDR, Action: action, Label: label,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s firewall rule for port %d: %w", verb, port, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "firewall rule %q (%s %d/%s) created\n", created.ID, created.Action, created.Port, created.Protocol)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func firewallCreateUsage(prog, verb string) string {
	return fmt.Sprintf(`Usage:
  %[1]s firewall %[5]s --port N [flags]

Adds a %[5]s rule. A rule targeting a port this control plane itself
needs (the management API, agent connections, or ingress) is refused.

Flags:
  --port int               port number, 1-65535 (required)
  --protocol string        "tcp" or "udp" (default "tcp")
  --source-cidr string     restrict the rule to this source CIDR (default: any source)
  --label string           optional note describing what this rule is for
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the created firewall rule as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, verb)
}
