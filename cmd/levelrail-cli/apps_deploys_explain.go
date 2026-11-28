package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runAppsDeploysExplain implements "apps deploys explain <name> [deploy-id]":
// the reason, up to three evidence lines and the proposed fix as a diff. It
// never changes anything; the apply command is printed for the operator.
func runAppsDeploysExplain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys explain", "print the explanation as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysExplainUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys explain requires an app name and an optional deploy id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, deployID := rest[0], ""
	if len(rest) == 2 && rest[1] != "latest" {
		deployID = rest[1]
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	d, err := client.DiagnoseApp(context.Background(), name, deployID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("explain deploy for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, d, func() { printDeployExplanation(stdout, prog, name, d) })
}

const maxExplainEvidence = 3

func printDeployExplanation(out io.Writer, prog, name string, d apiclient.DiagnosisResource) {
	deploy := ""
	if d.DeployAttemptID != "" {
		deploy = " --deploy " + d.DeployAttemptID
	}
	if len(d.Causes) == 0 {
		_, _ = fmt.Fprintf(out, "reason: %s\n", d.Explanation)
		_, _ = fmt.Fprintf(out, "next:   %s\n", d.Suggestion)
		return
	}
	c := d.Causes[0]
	_, _ = fmt.Fprintf(out, "reason: %s (%s)\n  %s\n", c.Title, c.Code, c.Explanation)
	if len(c.Evidence) > 0 {
		_, _ = fmt.Fprintln(out, "evidence:")
		for i, e := range c.Evidence {
			if i == maxExplainEvidence {
				break
			}
			_, _ = fmt.Fprintf(out, "  %s\n", e.Excerpt)
		}
	}
	for _, f := range c.Fixes {
		_, _ = fmt.Fprintf(out, "fix %d [%s]: %s\n", f.N, f.Kind, f.Label)
		var inputs []string
		for _, ch := range f.Changes {
			if ch.NeedsInput {
				_, _ = fmt.Fprintf(out, "  %s: (you supply the value)\n", ch.Field)
				inputs = append(inputs, " --input "+ch.Field+"=VALUE")
				continue
			}
			_, _ = fmt.Fprintf(out, "  - %s: %s\n  + %s: %s\n", ch.Field, dashIfEmpty(ch.From), ch.Field, ch.To)
		}
		if f.Hint != "" {
			_, _ = fmt.Fprintf(out, "  %s\n", f.Hint)
		}
		if f.Kind == "manual" {
			continue
		}
		redeploy := ""
		if f.Redeploy {
			redeploy = " --redeploy"
		}
		_, _ = fmt.Fprintf(out, "  apply: %s apps diagnose %s%s --apply-fix %d%s%s\n", prog, name, deploy, f.N, strings.Join(inputs, ""), redeploy)
	}
	_, _ = fmt.Fprintf(out, "Nothing was changed. Retry without a fix: %s apps deploy %s\n", prog, name)
}

func appsDeploysExplainUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys explain <name> [deploy-id] [flags]

Says why a deploy failed in plain words: the reason, up to three log lines
as evidence (secrets masked) and a typed fix shown as a diff. Nothing is
applied; the exact command to apply the fix is printed. Covers wrong port,
missing env var, out of memory, missing start command, wrong Node version
and missing build-time public variables, among others.
`, prog)
}
