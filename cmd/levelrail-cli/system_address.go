package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSystem dispatches "system <verb>".
func runSystem(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, systemUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, systemUsage(prog))
		return exitOK
	case "address":
		return runSystemAddress(prog, args[1:], stdout, stderr, lookupEnv)
	case "restart":
		return runSystemRestart(prog, args[1:], stdout, stderr, lookupEnv)
	case "health":
		return runSystemHealth(prog, args[1:], stdout, stderr, lookupEnv)
	case "integrity":
		return runSystemIntegrity(prog, args[1:], stdout, stderr, lookupEnv)
	case "networks":
		return runSystemNetworks(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown system subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, systemUsage(prog))
		return exitUsage
	}
}

func systemUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s system address [--domain NAME] [flags]   show this server's public IPv4 and IPv6
  %[1]s system address --ipv4 IP [--ipv6 IP]     override the detected addresses
  %[1]s system address --clear                   go back to detection
  %[1]s system restart [--wait] [--yes]          restart the control plane process
  %[1]s system restart status                    show the restart state and boot id
  %[1]s system health [--fix ID]                 show control plane saturation, optionally apply a fix
  %[1]s system integrity [--check [--full]]      database integrity state and restorable snapshots
  %[1]s system integrity --restore NAME|latest   stage a verified snapshot for the next start
  %[1]s system networks [--orphans]             list app networks, owners and pool usage
  %[1]s system networks prune --dry-run|--yes    remove orphaned, empty app networks

With --domain, also prints the exact DNS records to create for that name.
`, prog)
}

func runSystemAddress(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system address", "print the server address as JSON", stderr)
	var domain, ipv4, ipv6 string
	var clearOverride bool
	fs.StringVar(&domain, "domain", "", "a hostname to print the DNS records for")
	fs.StringVar(&ipv4, "ipv4", "", "override the public IPv4 address")
	fs.StringVar(&ipv6, "ipv6", "", "override the public IPv6 address")
	fs.BoolVar(&clearOverride, "clear", false, "remove the override and use detection again")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, systemUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if clearOverride && (ipv4 != "" || ipv6 != "") {
		_, _ = fmt.Fprintf(stderr, "%s: --clear cannot be combined with --ipv4 or --ipv6\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if clearOverride || ipv4 != "" || ipv6 != "" {
		if _, err := client.SetServerAddress(ctx, apiclient.ServerAddressPair{IPv4: ipv4, IPv6: ipv6}); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set server address: %w", err))
		}
	}
	addr, err := client.GetServerAddress(ctx, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get server address: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, addr, func() { printServerAddress(stdout, addr) })
}

func printServerAddress(out io.Writer, a apiclient.ServerAddress) {
	show := func(v string) string {
		if v == "" {
			return "(none)"
		}
		return v
	}
	_, _ = fmt.Fprintf(out, "IPv4:   %s\nIPv6:   %s\nsource: %s\n", show(a.IPv4), show(a.IPv6), a.Source)
	if a.Override.IPv4 != "" || a.Override.IPv6 != "" {
		_, _ = fmt.Fprintf(out, "detected: %s %s\n", show(a.Detected.IPv4), show(a.Detected.IPv6))
	}
	if a.BehindNAT {
		_, _ = fmt.Fprintln(out, "note: the public IPv4 is not on a local interface, so this server is behind NAT or a cloud 1:1 mapping; forward ports 80 and 443")
	}
	if a.TLSTerminatedUpstream {
		_, _ = fmt.Fprintln(out, "note: TLS is terminated by a proxy in front; point DNS at that proxy, not at this server")
	}
	if a.Domain != "" {
		printDNSRecords(out, a)
	}
}
