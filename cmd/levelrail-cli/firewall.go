package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runFirewall dispatches "firewall <verb> [args] [flags]" to status/sync,
// the same multi-verb dispatch shape runNodes uses.
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
		return runFirewallStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "sync":
		return runFirewallSync(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown firewall subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, firewallUsage(prog))
		return exitUsage
	}
}

func firewallUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s firewall status [flags]   show every port this platform wants open, and whether ufw currently allows it
  %[1]s firewall sync [flags]     apply a firewall sync right now instead of waiting for the next background reconcile tick

Run "%[1]s firewall <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runFirewallStatus implements "firewall status": GET /api/v1/system/firewall
// (internal/api), AbilityRead. Read-only, use "firewall sync" to apply.
func runFirewallStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall status", "print the firewall status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, firewallStatusUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetFirewallStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get firewall status: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, status, func() { printFirewallStatusHuman(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func firewallStatusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s firewall status [flags]

Shows every port this platform currently wants open, an app's HostPort
tagged "app:<name>" or a database's public access port tagged
"db:<name>", and whether ufw is actually allowing it right now. Any
leftover levelrail-tagged rule no longer wanted is listed separately as
EXTRA (stale); "%[1]s firewall sync" removes those automatically.

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the firewall status as JSON to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// printFirewallStatusHuman prints "firewall status" output: an
// installed/active summary line first (doctor_firewall.go's own
// not-installed/inactive wording, since this and "doctor" report on the
// same ufw state from two different endpoints), then the wanted-ports
// table, then an EXTRA section only when stale rules exist.
func printFirewallStatusHuman(out io.Writer, s firewallStatusResource) {
	switch {
	case !s.Installed:
		_, _ = fmt.Fprintln(out, "ufw is not installed on this control plane; if you rely on a different firewall (cloud security groups, firewalld), this is expected")
	case !s.Active:
		_, _ = fmt.Fprintln(out, "ufw is installed but inactive; no ports are managed until it's enabled")
	}

	printFirewallRulesTable(out, s.Rules)

	if len(s.Extra) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\nEXTRA (stale) rules, no longer wanted:")
	printFirewallRulesTable(out, s.Extra)
}

func printFirewallRulesTable(out io.Writer, rules []firewallRuleResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PORT\tPROTO\tOWNER\tOPEN")
	for _, r := range rules {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%t\n", r.Port, r.Proto, r.Owner, r.Open)
	}
	_ = tw.Flush()
}

// runFirewallSync implements "firewall sync": POST /api/v1/system/firewall/sync
// (internal/api), AbilityRoot, the same tier "audit-purge" and "secrets
// rotate-master-key" use since it's a fleet-wide action on the actual
// host firewall, not one app's own resources.
func runFirewallSync(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall sync", "print the sync result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, firewallSyncUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.SyncFirewall(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("sync firewall: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printFirewallSyncResultHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if of.Format == outputTable && len(result.Errors) > 0 {
		return exitAPIError
	}
	return exitOK
}

func printFirewallSyncResultHuman(out io.Writer, r firewallSyncResource) {
	printFirewallStatusHuman(out, r.FirewallStatusResource)
	_, _ = fmt.Fprintf(out, "\napplied: %d, removed: %d\n", r.Applied, r.Removed)
	if len(r.Errors) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "errors:")
	for _, e := range r.Errors {
		_, _ = fmt.Fprintf(out, "  %s\n", e)
	}
}

func firewallSyncUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s firewall sync [flags]

Applies a firewall sync right now instead of waiting for the next
background reconcile tick: opens every port this platform wants open
and removes any leftover levelrail-tagged rule no longer wanted.
Requires an admin/root-scoped token, the same as "%[1]s audit-purge".

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the sync result as JSON to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
