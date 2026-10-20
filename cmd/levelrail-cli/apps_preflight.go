package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const preflightFailExit = 1

// runAppsPreflight implements "apps preflight <name> [--require-env A,B]":
// POST /api/v1/apps/{name}/preflight, read-only checks of DNS, ports, disk,
// memory, image, git source, env, mounts and GPU. Exits non-zero on a fail.
func runAppsPreflight(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps preflight", "print the report as JSON to stdout and nothing else", stderr)
	var requireEnv string
	fs.StringVar(&requireEnv, "require-env", "", "comma separated env var names that must be set")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps preflight <name> [--require-env A,B] [flags]\n\nRuns read-only pre-deploy checks against the app's stored configuration.\nExits 1 when any check fails.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps preflight", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	var required []string
	for _, k := range strings.Split(requireEnv, ",") {
		if k = strings.TrimSpace(k); k != "" {
			required = append(required, k)
		}
	}
	report, err := client.PreflightApp(context.Background(), name, required)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preflight app %q: %w", name, err))
	}
	if code := writeScheduledTaskResult(stdout, stderr, of, report, func() { printPreflightHuman(stdout, report) }); code != exitOK {
		return code
	}
	if report.Status == "fail" {
		return preflightFailExit
	}
	return exitOK
}

func printPreflightHuman(out io.Writer, r apiclient.PreflightReport) {
	_, _ = fmt.Fprintf(out, "preflight: %s\n\n", r.Status)
	for _, c := range r.Checks {
		_, _ = fmt.Fprintf(out, "[%s] %s: %s\n", c.Status, c.Name, c.Reason)
		if c.Fix != "" && c.Status != "pass" {
			_, _ = fmt.Fprintf(out, "       fix: %s\n", c.Fix)
		}
	}
}
