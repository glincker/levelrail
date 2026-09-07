package main

import (
	"context"
	"fmt"
	"io"
)

// domainsMaintenanceJSONUsage is every domains maintenance subcommand's
// --json flag description: identical across get/set/clear since each
// returns the same maintenance-state shape.
const domainsMaintenanceJSONUsage = "print the maintenance state as JSON to stdout and nothing else"

// runDomainsMaintenance dispatches "domains maintenance <verb> [flags]"
// to one of get/set/clear, mirroring runDomainsBasicAuth's own
// top-level dispatch shape for a different per-domain toggle.
func runDomainsMaintenance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsMaintenanceUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsMaintenanceUsage(prog))
		return exitOK
	case "get":
		return runDomainsMaintenanceGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsMaintenanceSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsMaintenanceClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains maintenance subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsMaintenanceUsage(prog))
		return exitUsage
	}
}

func domainsMaintenanceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains maintenance get <app> <domain> [flags]     show a domain's maintenance state
  %[1]s domains maintenance set <app> <domain> [flags]     enable maintenance mode
  %[1]s domains maintenance clear <app> <domain> [flags]   disable maintenance mode

While enabled, the embedded Caddy ingress serves a fixed "down for
maintenance" response for <domain> instead of proxying to its
container, without stopping the container itself. <domain> must
already be one of <app>'s configured domains (see "%[1]s apps get <app>"
or "%[1]s domains list").

Run "%[1]s domains maintenance <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsMaintenanceGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance get", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance get <app> <domain> [flags]\n\nShows a domain's currently configured maintenance state.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains maintenance get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	m, err := client.GetDomainMaintenance(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get maintenance state for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printDomainMaintenanceHuman(stdout, m) })
}

func runDomainsMaintenanceSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance set", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance set <app> <domain> [flags]\n\nEnables maintenance mode on <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains maintenance set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	m, err := client.SetDomainMaintenance(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("enable maintenance mode for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printDomainMaintenanceHuman(stdout, m) })
}

func runDomainsMaintenanceClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance clear", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance clear <app> <domain> [flags]\n\nDisables maintenance mode on <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains maintenance clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	m, err := client.ClearDomainMaintenance(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable maintenance mode for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, m, func() {
		_, _ = fmt.Fprintf(stdout, "maintenance mode disabled for domain %q\n", domain)
	})
}

func printDomainMaintenanceHuman(out io.Writer, m domainMaintenanceResource) {
	status := "disabled"
	if m.Enabled {
		status = "enabled"
	}
	_, _ = fmt.Fprintf(out, "domain: %s\nstatus: %s\n", m.Domain, status)
}
