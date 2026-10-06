package main

import (
	"context"
	"fmt"
	"io"
)

// runFirewallHost implements "firewall status|enable|disable": the host ufw
// switch behind /api/v1/firewall/host (Settings -> Firewall in the dashboard).
func runFirewallHost(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall "+verb, "print the firewall status as JSON to stdout and nothing else", stderr)
	var dryRun bool
	if verb != "status" {
		fs.BoolVar(&dryRun, "dry-run", false, "show the commands that would run without changing the firewall")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, firewallHostUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var res hostFirewallResource
	var err error
	if verb == "status" {
		res, err = client.HostFirewallStatus(context.Background())
	} else {
		res, err = client.SetHostFirewall(context.Background(), verb == "enable", dryRun)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("firewall %s: %w", verb, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, res, func() { printHostFirewall(stdout, verb, dryRun, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printHostFirewall(out io.Writer, verb string, dryRun bool, res hostFirewallResource) {
	switch {
	case !res.Installed:
		_, _ = fmt.Fprintln(out, "ufw is not installed on this host")
	case res.Active:
		_, _ = fmt.Fprintf(out, "firewall: on (default incoming: %s)\n", res.DefaultIncoming)
	default:
		_, _ = fmt.Fprintln(out, "firewall: off")
	}
	if verb == "status" {
		_, _ = fmt.Fprintln(out, "required ports:")
		for _, p := range res.Required {
			_, _ = fmt.Fprintf(out, "  %d/%s\n", p.Port, p.Protocol)
		}
		return
	}
	if dryRun {
		_, _ = fmt.Fprintln(out, "dry run, would run:")
		for _, c := range res.Commands {
			_, _ = fmt.Fprintf(out, "  %s\n", c)
		}
	}
}

func firewallHostUsage(prog, verb string) string {
	flags := ""
	if verb != "status" {
		flags = "  --dry-run                show the commands that would run, change nothing\n"
	}
	return fmt.Sprintf(`Usage:
  %[1]s firewall %[2]s [flags]

Needs the root ability. Enabling allows SSH first, then the ports the
control plane needs, and only then turns ufw on, so it cannot lock you out.
Set APP_FIREWALL_SSH_PORTS on the control plane if SSH is not on port 22.

Flags:
%[3]s  --token string          API token (default: %[4]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[5]s env var, then %[6]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, verb, flags, envAPIToken, envAPIURL, defaultAPIURL)
}
