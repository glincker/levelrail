package main

import (
	"context"
	"fmt"
	"io"
)

// runProxy handles "proxy [--domain D] [--proxy KIND] [--verify]".
func runProxy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "proxy", "print the guide as JSON to stdout and nothing else", stderr)
	var domain, kind string
	var verify bool
	fs.StringVar(&domain, "domain", "", "hostname to serve the dashboard on, for example console.example.com")
	fs.StringVar(&kind, "proxy", "", "traefik, nginx or caddy (default: the one detected on this server)")
	fs.BoolVar(&verify, "verify", false, "check that the domain already reaches this dashboard")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s proxy [--domain D] [--proxy traefik|nginx|caddy] [--verify]\n\nShows what holds ports 80 and 443 and, with --domain, the exact configuration that puts the dashboard behind that proxy.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	g, err := client.GetReverseProxyGuide(context.Background(), domain, kind, verify)
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
			if domain == "" {
				_, _ = fmt.Fprintf(stdout, "\nRun again with --domain console.example.com for the configuration.\n")
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
