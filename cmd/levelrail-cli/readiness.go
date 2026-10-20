package main

import (
	"context"
	"fmt"
	"io"
	"sort"
)

// runReadiness implements "readiness": GET /api/v1/system/readiness, who owns
// ports 80 and 443, Docker, disk, memory, firewall and DNS, with the install
// mode and the exact next step.
func runReadiness(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "readiness", "print the readiness report as JSON to stdout and nothing else", stderr)
	domain := fs.String("domain", "", "also check that this `domain` resolves to this server")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s readiness [--domain example.com] [flags]\n\nChecks whether this server is ready: owners of ports 80 and 443 (Traefik, nginx,\nCaddy), Docker version and rootless mode, exposed Docker API, disk, memory,\nfirewall and the DNS of the intended domain. Prints the recommended install\nmode (own ports or behind your existing proxy) and the exact next step.\nExits 1 when a check fails.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	report, err := client.GetServerReadiness(context.Background(), *domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("server readiness: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, report, func() { printReadiness(stdout, report) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if report.Blocked {
		return exitCheckFailed
	}
	return exitOK
}

func printReadiness(out io.Writer, r serverReadiness) {
	for _, c := range r.Checks {
		_, _ = fmt.Fprintf(out, "[%s] %s: %s\n", c.Status, c.Name, c.Detail)
		if c.Fix != "" && c.Status != "pass" {
			_, _ = fmt.Fprintf(out, "       fix: %s\n", c.Fix)
		}
	}
	ports := make([]int, 0, len(r.Holders))
	for p := range r.Holders {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	for _, p := range ports {
		_, _ = fmt.Fprintf(out, "port %d is held by %s\n", p, r.Holders[p])
	}
	_, _ = fmt.Fprintf(out, "\nrecommended mode: %s\n%s\n\nNext step:\n  %s\n", r.Mode, r.Summary, r.NextStep)
}
