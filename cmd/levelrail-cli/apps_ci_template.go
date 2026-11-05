package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runAppsCITemplate serves "apps ci template <app> [provider]": the example pipeline for one CI, or the list of providers.
func runAppsCITemplate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps ci template", "print the templates as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "usage: %s apps ci template <app> [provider]\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetCITemplates(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ci templates for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if len(rest) == 1 {
			for _, t := range res.Templates {
				_, _ = fmt.Fprintf(stdout, "%-10s %-28s auth: %s\n", t.ID, t.Target, t.Auth)
			}
			return
		}
		for _, t := range res.Templates {
			if t.ID != rest[1] {
				continue
			}
			_, _ = fmt.Fprint(stdout, t.Content)
			if t.Auth == "token" {
				_, _ = fmt.Fprintf(stderr, "note: this file needs a stored token, create one with: %s tokens create --name ci-%s --abilities %s --expires-in-days 90\n", prog, rest[0], strings.Join(res.DeployerAbilities, ","))
			}
			if res.NeedsDomain {
				_, _ = fmt.Fprintf(stderr, "note: no dashboard URL is set yet, set one in Settings so CI can reach this control plane\n")
			}
			return
		}
		_, _ = fmt.Fprintf(stderr, "unknown provider %q, run %s apps ci template %s to list them\n", rest[1], prog, rest[0])
	})
}
