package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const envHistoryCLILimit = 50

func runAppsEnvCheck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env check", "print the findings as JSON to stdout and nothing else", stderr)
	fix := fs.Bool("fix", false, "apply every automatic fix, then print what is left")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env check <name> [--fix] [flags]\n\nFinds empty variables, plain variables shadowed by a secret or database binding,\nnames that differ only by case, required secrets with no value, and bindings to\ndatabases that no longer exist. --fix removes what can be removed safely.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps env check takes exactly one app name\n", prog)
		return exitUsage
	}
	name := fs.Arg(0)
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.CheckEnv(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if *fix {
		res, err = applyEnvFixes(ctx, client, name, res)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printEnvFindings(stdout, res) })
}

func applyEnvFixes(ctx context.Context, client *Client, name string, res apiclient.EnvCheckResource) (apiclient.EnvCheckResource, error) {
	for _, f := range res.Findings {
		if f.Fix.Type != "auto" {
			continue
		}
		next, err := client.FixEnv(ctx, name, f.Kind, f.Key)
		if err != nil {
			return res, fmt.Errorf("fix %s on %s: %w", f.Kind, f.Key, err)
		}
		res = next
	}
	return res, nil
}

func printEnvFindings(out io.Writer, res apiclient.EnvCheckResource) {
	if len(res.Findings) == 0 {
		_, _ = fmt.Fprintln(out, "no problems found")
		return
	}
	for _, f := range res.Findings {
		detail := ""
		if f.Detail != "" {
			detail = " (" + f.Detail + ")"
		}
		_, _ = fmt.Fprintf(out, "%-7s %-18s %s%s  fix: %s\n", f.Severity, f.Kind, f.Key, detail, f.Fix.Type)
	}
}

func runAppsEnvHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env history", "print the history as JSON to stdout and nothing else", stderr)
	limit := fs.Int("limit", envHistoryCLILimit, "how many changes to show, newest first")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env history <name> [flags]\n\nLists which variables were added, changed or removed, by whom and when.\nValues are never recorded.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps env history takes exactly one app name\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	changes, err := client.ListEnvHistory(context.Background(), fs.Arg(0), *limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	return writeScheduledTaskResult(stdout, stderr, of, changes, func() {
		if len(changes) == 0 {
			_, _ = fmt.Fprintln(stdout, "no recorded changes")
			return
		}
		for _, c := range changes {
			_, _ = fmt.Fprintf(stdout, "%s  %-8s %-24s %s\n", c.At, c.Action, c.Key, c.Actor)
		}
	})
}
