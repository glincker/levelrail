package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runProxyIntegration handles "proxy setup|status|apply|verify|disable".
func runProxyIntegration(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "proxy "+sub, "print the result as JSON to stdout and nothing else", stderr)
	var confirm bool
	var dir, domain string
	switch sub {
	case "setup":
		fs.BoolVar(&confirm, "confirm", false, "apply the changes; without it setup is a dry run")
		fs.StringVar(&dir, "dynamic-dir", "", "host directory Traefik watches, when it cannot be detected")
	case "verify":
		fs.StringVar(&domain, "domain", "", "verify only this domain")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, proxyUsage(prog)); fs.PrintDefaults() }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.ProxyIntegration
	var err error
	switch sub {
	case "setup":
		res, err = client.SetupProxyIntegration(ctx, apiclient.ProxySetupRequest{Confirm: confirm, DynamicDir: dir})
	case "status":
		res, err = client.GetProxyIntegration(ctx)
	case "apply":
		res, err = client.ApplyProxyIntegration(ctx)
	case "verify":
		res, err = client.VerifyProxyIntegration(ctx, domain)
	case "disable":
		res, err = disableProxyIntegration(ctx, client)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("proxy %s: %w", sub, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printProxyIntegration(stdout, prog, sub, res) })
}

// disableProxyIntegration keeps the saved values so the next pass can find
// and remove its own files.
func disableProxyIntegration(ctx context.Context, client *apiclient.Client) (apiclient.ProxyIntegration, error) {
	cur, err := client.GetProxyIntegration(ctx)
	if err != nil {
		return apiclient.ProxyIntegration{}, err
	}
	s := cur.Settings
	s.Integration = "off"
	if _, err := client.UpdateProxyIntegrationSettings(ctx, s); err != nil {
		return apiclient.ProxyIntegration{}, err
	}
	return client.GetProxyIntegration(ctx)
}

func printProxyIntegration(out io.Writer, prog, sub string, r apiclient.ProxyIntegration) {
	d := r.Detected
	if d.Kind == "none" || d.Kind == "" {
		_, _ = fmt.Fprintln(out, "proxy: none detected")
	} else {
		_, _ = fmt.Fprintf(out, "proxy: %s in container %s (%s), ports %v\n", d.Kind, d.Container, d.Image, d.PublishedPorts)
		_, _ = fmt.Fprintf(out, "  dir %s, entrypoints %s/%s, resolver %s, upstream %s\n", orDash(d.DynamicDir), orDash(d.EntrypointHTTP), orDash(d.EntrypointHTTPS), orDash(d.CertResolver), orDash(d.UpstreamHost))
	}
	for _, m := range d.Missing {
		_, _ = fmt.Fprintf(out, "  missing: %s\n", m)
	}
	_, _ = fmt.Fprintf(out, "integration: %s\n", r.Settings.Integration)
	if sub == "setup" {
		verb := "changed"
		if r.DryRun {
			verb = "would change"
		}
		if len(r.Changes) == 0 {
			_, _ = fmt.Fprintln(out, "setup: nothing to change")
		}
		for _, c := range r.Changes {
			_, _ = fmt.Fprintf(out, "setup %s %s\n", verb, c)
		}
	}
	for _, dom := range r.Domains {
		line := fmt.Sprintf("%s -> %s %s [%s]", dom.Domain, dom.Target, dom.App, dom.State)
		if dom.CheckedAt != "" {
			line += fmt.Sprintf(" reachable=%v", dom.Reachable)
			if dom.Certificate != nil {
				line += fmt.Sprintf(" cert=%s until %s valid=%v", dom.Certificate.Issuer, dom.Certificate.NotAfter, dom.Certificate.Valid)
			}
		}
		if dom.LastError != "" {
			line += ": " + dom.LastError
		}
		_, _ = fmt.Fprintln(out, strings.TrimSpace(line))
	}
	for _, s := range r.Steps {
		_, _ = fmt.Fprintf(out, "[%s] %s: %s\n", s.State, s.ID, s.Detail)
	}
	if sub == "setup" && r.DryRun {
		_, _ = fmt.Fprintf(out, "\nRun %s proxy setup --confirm to apply.\n", prog)
	}
}
