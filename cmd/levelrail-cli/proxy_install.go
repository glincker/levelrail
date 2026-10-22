package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runProxyInstall handles "proxy install [--provider traefik] [--confirm]".
func runProxyInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "proxy install", "print the result as JSON to stdout and nothing else", stderr)
	var confirm bool
	var provider string
	fs.BoolVar(&confirm, "confirm", false, "create and start the proxy container; without it install is a dry run")
	fs.StringVar(&provider, "provider", "traefik", "proxy to install (traefik)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, proxyUsage(prog)); fs.PrintDefaults() }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.InstallProxy(context.Background(), apiclient.ProxyInstallRequest{Provider: provider, Confirm: confirm})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("proxy install: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printProxyInstall(stdout, prog, res) })
}

func printProxyInstall(out io.Writer, prog string, r apiclient.ProxyInstallResult) {
	p := r.Plan
	_, _ = fmt.Fprintf(out, "image %s as container %s, ports %v, restart %s\n", p.Image, p.Container, p.Ports, p.Restart)
	for _, m := range p.Mounts {
		_, _ = fmt.Fprintf(out, "mount %s at %s\n", m.Host, m.Container)
	}
	for _, f := range p.Files {
		_, _ = fmt.Fprintf(out, "write %s:\n%s\n", f.Path, f.Content)
	}
	if r.DryRun {
		_, _ = fmt.Fprintf(out, "Dry run. Run %s proxy install --confirm to apply.\n", prog)
		return
	}
	_, _ = fmt.Fprintf(out, "result: %s. %s\n", r.Outcome, r.Detail)
}
