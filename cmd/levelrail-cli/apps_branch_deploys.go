package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runAppsBranchDeploys(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBranchDeploysUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBranchDeploysUsage(prog))
		return exitOK
	case "list":
		return runAppsBranchDeploysList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsBranchDeploysSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsBranchDeploysDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps branch-deploys subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsBranchDeploysUsage(prog))
		return exitUsage
	}
}

func appsBranchDeploysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps branch-deploys list <name> [flags]                                       list an app's branch deploy policy and live branch deploys
  %[1]s apps branch-deploys set <name> --mode off|all|patterns [--branch PATTERN]... [flags]   choose which branches get a branch deploy
  %[1]s apps branch-deploys delete <name> <slug> [flags]                              remove one branch deploy now

A branch deploy gives a branch its own stable URL and rebuilds on every push to
that branch. It is removed when the branch is deleted, or after the idle TTL.
The production branch never gets one. --branch is an exact name or a glob where
"*" matches any characters (for example "feature/*"), and repeats.
`, prog)
}

func runAppsBranchDeploysList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps branch-deploys list", "print the branch deploys as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps branch-deploys list <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps branch-deploys list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	out, err := client.ListBranchDeploys(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list branch deploys for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, out, func() { printBranchDeploysHuman(stdout, out) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printBranchDeploysHuman(stdout io.Writer, out apiclient.BranchDeploys) {
	_, _ = fmt.Fprintf(stdout, "mode: %s", out.Settings.Mode)
	if len(out.Settings.Patterns) > 0 {
		_, _ = fmt.Fprintf(stdout, " (%s)", strings.Join(out.Settings.Patterns, ", "))
	}
	_, _ = fmt.Fprintln(stdout)
	if len(out.Deploys) == 0 {
		_, _ = fmt.Fprintln(stdout, "no branch deploys")
		return
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "BRANCH\tSLUG\tSTATUS\tCOMMIT\tURL")
	for _, d := range out.Deploys {
		sha := d.HeadSHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.Branch, d.Slug, d.Status, sha, d.URL)
	}
	_ = tw.Flush()
}

func runAppsBranchDeploysSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps branch-deploys set", "print the resulting branch deploys as JSON to stdout and nothing else", stderr)
	var mode string
	var branches stringList
	fs.StringVar(&mode, "mode", "", "off, all or patterns (required)")
	fs.Var(&branches, "branch", "exact branch name or glob like \"feature/*\"; repeat for several (patterns mode)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps branch-deploys set <name> --mode off|all|patterns [--branch PATTERN]... [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps branch-deploys set", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if mode == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps branch-deploys set requires --mode\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	out, err := client.SetBranchDeploySettings(context.Background(), name, mode, branches)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set branch deploys for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, out, func() { printBranchDeploysHuman(stdout, out) })
}

func runAppsBranchDeploysDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps branch-deploys delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps branch-deploys delete <name> <slug> [flags]\n\nRemoves one branch deploy by its slug, as shown by \"apps branch-deploys list\".\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	cmd := twoArgCmd{prog: prog, cmdLabel: "apps branch-deploys delete", argsLabel: "an app name and a branch deploy slug"}
	client, name, slug, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := client.DeleteBranchDeploy(context.Background(), name, slug); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete branch deploy %q for app %q: %w", slug, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "branch deploy %q removed from app %q\n", slug, name)
	})
}
