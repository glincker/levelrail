package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runImportRepo implements "import repo": today only the read-only plan.
func runImportRepo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "plan" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s import repo plan <repo-url> [--branch B] [--dir D] [--json]\n", prog)
		return exitUsage
	}
	return runImportRepoPlan(prog, args[1:], stdout, stderr, lookupEnv)
}

func runImportRepoPlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "import repo plan", "print the plan as JSON to stdout and nothing else", stderr)
	var branch, dir string
	fs.StringVar(&branch, "branch", "", "branch to inspect (default: the repository's default branch)")
	fs.StringVar(&dir, "dir", "", "only plan packages under this directory")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s import repo plan <repo-url> [flags]\n\nLists one proposed app per workspace package: name, directory, port, build\nkind, dependencies and warnings. Nothing is created.\n\nFlags:\n  --branch B  branch to inspect\n  --dir D     only plan packages under this directory\n  --token, --api-url, --profile, --json, --output, --query   as for every command\n", prog)
	}
	client, repo, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "import repo plan", "repository URL"}, lookupEnv)
	if !ok {
		return exitCode
	}
	plan, err := client.PlanImportRepo(context.Background(), apiclient.ImportRepoPlanRequest{RepoURL: repo, Branch: branch, BaseDirectory: dir})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan repository import: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, plan, func() { printRepoPlan(stdout, plan) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRepoPlan(out io.Writer, p apiclient.RepoPlan) {
	if len(p.Packages) == 0 {
		_, _ = fmt.Fprintln(out, "No workspace packages found.")
	} else {
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tDIRECTORY\tPORT\tBUILD\tDEPENDS ON\tWARNINGS")
		for _, r := range p.Packages {
			port := "-"
			if r.Port != nil {
				port = strconv.Itoa(*r.Port)
			}
			codes := make([]string, 0, len(r.Warnings))
			for _, w := range r.Warnings {
				codes = append(codes, w.Code)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, r.Directory, port, r.BuildKind, dashJoin(r.DependsOn), dashJoin(codes))
		}
		_ = tw.Flush()
	}
	_, _ = fmt.Fprintf(out, "\n%d packages, %d warnings (%d packages flagged)\n", p.Totals.Packages, p.Totals.Warnings, p.Totals.Flagged)
	for _, w := range p.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", w.Message)
	}
	for _, r := range p.Packages {
		for _, w := range r.Warnings {
			_, _ = fmt.Fprintf(out, "warning: %s: %s\n", r.Name, strings.TrimSpace(w.Message))
		}
	}
}
