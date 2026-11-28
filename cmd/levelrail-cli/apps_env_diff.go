package main

import (
	"context"
	"fmt"
	"io"

	"github.com/glincker/levelrail/internal/apiclient"
)

func printEnvDiff(out io.Writer, r apiclient.EnvDiff) {
	_, _ = fmt.Fprintf(out, "--- %s\n+++ %s\n", r.App, r.Against)
	for _, k := range r.Added {
		_, _ = fmt.Fprintf(out, "+ %s\n", k)
	}
	for _, k := range r.Removed {
		_, _ = fmt.Fprintf(out, "- %s\n", k)
	}
	for _, k := range r.Changed {
		_, _ = fmt.Fprintf(out, "~ %s (differs)\n", k)
	}
	_, _ = fmt.Fprintf(out, "%d same, %d added, %d removed, %d differ, %d secrets not compared\n", r.Same, len(r.Added), len(r.Removed), len(r.Changed), r.SecretUnknown)
}

func runAppsEnvDiff(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env diff", "print the diff as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env diff <app-a> <app-b> [flags]\n\nCompares the env keys of two apps, for example staging against production.\nThe second argument may also be environment:<id> or preview.\nValues are never shown, only which keys are added, removed or differ.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps env diff takes exactly two arguments\n", prog)
		return exitUsage
	}
	nameA, against := fs.Arg(0), fs.Arg(1)

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetEnvDiff(context.Background(), nameA, against)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printEnvDiff(stdout, res) })
}
