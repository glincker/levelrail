package main

import (
	"context"
	"fmt"
	"io"
)

// domainsBasicAuthJSONUsage is every domains basic-auth subcommand's
// --json flag description: identical across get/set/clear since each
// returns the same basic-auth-state shape.
const domainsBasicAuthJSONUsage = "print the basic auth state as JSON to stdout and nothing else"

// runDomainsBasicAuth dispatches "domains basic-auth <verb> [flags]" to
// one of get/set/clear, mirroring runAppsLogDrain's own top-level
// dispatch shape for a different per-app sub-resource.
func runDomainsBasicAuth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsBasicAuthUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsBasicAuthUsage(prog))
		return exitOK
	case "get":
		return runDomainsBasicAuthGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsBasicAuthSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsBasicAuthClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains basic-auth subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsBasicAuthUsage(prog))
		return exitUsage
	}
}

func domainsBasicAuthUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains basic-auth get <app> <domain> [flags]                            show a domain's basic auth state
  %[1]s domains basic-auth set <app> <domain> --username USER --password PASS [flags]   protect a domain
  %[1]s domains basic-auth clear <app> <domain> [flags]                          remove protection

Protects one of <app>'s domains with HTTP Basic Auth, enforced by the
embedded Caddy ingress on the next reconcile pass. <domain> must already
be one of <app>'s configured domains (see "%[1]s apps get <app>" or
"%[1]s domains list").

Run "%[1]s domains basic-auth <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsBasicAuthGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains basic-auth get", domainsBasicAuthJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains basic-auth get <app> <domain> [flags]\n\nShows a domain's currently configured basic auth state.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains basic-auth get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	auth, err := client.GetDomainBasicAuth(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get basic auth for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, auth, func() { printDomainBasicAuthHuman(stdout, auth) })
}

func runDomainsBasicAuthSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains basic-auth set", domainsBasicAuthJSONUsage, stderr)
	var usernameFlag, passwordFlag string
	fs.StringVar(&usernameFlag, "username", "", "basic auth username (required)")
	fs.StringVar(&passwordFlag, "password", "", "basic auth password (required the first time a domain is protected; omit to keep the currently stored password while changing only the username)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains basic-auth set <app> <domain> --username USER --password PASS [flags]\n\nProtects <domain> with HTTP Basic Auth.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains basic-auth set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]
	if usernameFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains basic-auth set requires --username\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	auth, err := client.SetDomainBasicAuth(context.Background(), appName, domain, setDomainBasicAuthRequest{
		Username: usernameFlag,
		Password: passwordFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set basic auth for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, auth, func() { printDomainBasicAuthHuman(stdout, auth) })
}

func runDomainsBasicAuthClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains basic-auth clear", domainsBasicAuthJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains basic-auth clear <app> <domain> [flags]\n\nRemoves basic auth protection from <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains basic-auth clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	auth, err := client.ClearDomainBasicAuth(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear basic auth for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, auth, func() {
		_, _ = fmt.Fprintf(stdout, "basic auth removed for domain %q\n", domain)
	})
}

func printDomainBasicAuthHuman(out io.Writer, a domainBasicAuthResource) {
	status := "disabled"
	if a.Enabled {
		status = "enabled"
	}
	_, _ = fmt.Fprintf(out, "domain:      %s\nstatus:      %s\nusername:    %s\nhas_password: %v\n", a.Domain, status, a.Username, a.HasPassword)
}
