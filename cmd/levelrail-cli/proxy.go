package main

import (
	"context"
	"fmt"
	"io"
)

// runProxy handles "proxy [--domain D | --app NAME] [--proxy KIND] [--verify]"
// and the managed route subcommands.
func runProxy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "setup", "status", "apply", "verify", "disable":
			return runProxyIntegration(prog, args[0], args[1:], stdout, stderr, lookupEnv)
		}
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "proxy", "print the guide as JSON to stdout and nothing else", stderr)
	var domain, kind, app string
	var verify bool
	fs.StringVar(&domain, "domain", "", "hostname to serve the dashboard on, for example console.example.com")
	fs.StringVar(&app, "app", "", "build the snippet for this app's domain (upstream: the ingress HTTP port) instead of the dashboard")
	fs.StringVar(&kind, "proxy", "", "traefik, nginx or caddy (default: the one detected on this server)")
	fs.BoolVar(&verify, "verify", false, "check that the domain already reaches this dashboard")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, proxyUsage(prog)); fs.PrintDefaults() }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	g, err := client.GetReverseProxyGuide(context.Background(), domain, kind, app, verify)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("reverse proxy guide: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, g, func() {
		if len(g.Holders) == 0 {
			_, _ = fmt.Fprintln(stdout, "Nothing else publishes port 80 or 443 on this server.")
		}
		for _, h := range g.Holders {
			_, _ = fmt.Fprintf(stdout, "port %d is published by %s (%s)\n", h.Port, h.Container, h.Image)
		}
		if g.Plan == nil {
			if domain == "" && app == "" {
				_, _ = fmt.Fprintf(stdout, "\nRun again with --domain console.example.com or --app NAME for the configuration, or %s proxy setup to let this server manage the routes.\n", prog)
			}
			return
		}
		_, _ = fmt.Fprintf(stdout, "\nPut %s behind %s:\n", g.Plan.Domain, g.Plan.Proxy)
		for i, s := range g.Plan.Steps {
			_, _ = fmt.Fprintf(stdout, "%d. %s\n", i+1, s)
		}
		_, _ = fmt.Fprintf(stdout, "\n%s\n", g.Plan.Snippet)
		if g.Check != nil {
			state := "not reachable yet"
			if g.Check.Reachable {
				state = "reachable: the domain serves this dashboard"
			}
			_, _ = fmt.Fprintf(stdout, "Check: %s. %s\n", state, g.Check.Detail)
		}
	})
}

func proxyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s proxy [--domain D | --app NAME] [--proxy traefik|nginx|caddy] [--verify]
  %[1]s proxy setup [--confirm] [--dynamic-dir DIR]
  %[1]s proxy status
  %[1]s proxy apply
  %[1]s proxy verify [--domain D]
  %[1]s proxy disable

Without a subcommand: shows what holds ports 80 and 443 and the configuration to paste
into that proxy, for the dashboard (--domain) or an app's domain (--app).

setup detects a Traefik that owns ports 80 and 443 and, with --confirm, lets this server
write one route file per domain into Traefik's watched directory. Without --confirm it
only prints what would change.

Flags:
`, prog)
}
