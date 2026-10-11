package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type sourceDeployFlags struct {
	commit string
	latest bool
	branch string
}

// deployFromSource builds the connected repository at a commit through the
// same endpoint the dashboard's deploy sheet uses.
func deployFromSource(prog string, client *Client, name string, f sourceDeployFlags, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	ctx := context.Background()
	gs, err := client.GetGitSource(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("load git source for app %q (connect one with \"apps git-source set\"): %w", name, err))
	}
	ref := f.commit
	if f.latest {
		cs, cerr := client.GitSourceCommits(ctx, name, f.branch, true)
		if cerr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("resolve latest commit for app %q: %w", name, cerr))
		}
		if cs.Head == nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("resolve latest commit for app %q: branch %q has no commits", name, cs.Branch))
		}
		ref = cs.Head.SHA
	}
	req := buildTriggerRequest{
		RepoURL: gs.RepoURL,
		Ref:     ref,
		Build:   buildTriggerRequestBuild{Type: gs.BuildType, Path: gs.BuildPath, BaseDirectory: gs.BaseDirectory},
	}
	result, err := client.TriggerBuild(ctx, name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy app %q from commit %s: %w", name, shortSHA(ref), err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "deploying %s of app %q, deploy attempt %q (follow with \"%s apps deploys logs %s %s\")\n", shortSHA(ref), name, result.ID, prog, name, result.ID)
	})
}

func runAppsGitSourceCommits(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source commits", "print the commits response as JSON to stdout and nothing else", stderr)
	var branch string
	var refresh bool
	fs.StringVar(&branch, "branch", "", "branch to list (default: the connected branch)")
	fs.BoolVar(&refresh, "refresh", false, "bypass the short provider-side cache")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source commits <name> [flags]\n\nLists the newest commits of the connected branch with what is deployed\nagainst them, plus suggested next steps.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source commits", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	cs, err := client.GitSourceCommits(context.Background(), name, branch, refresh)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list commits for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, cs, func() { printGitCommitsHuman(stdout, cs) })
}

func printGitCommitsHuman(out io.Writer, cs apiclient.GitSourceCommits) {
	state := "no deploy of this branch recorded"
	switch {
	case cs.UpToDate:
		state = "up to date"
	case cs.AheadBy != nil:
		state = fmt.Sprintf("%d commit(s) ahead of what is deployed", *cs.AheadBy)
	}
	_, _ = fmt.Fprintf(out, "branch: %s (%s)\nbuild:  %s\nstatus: %s\n\n", cs.Branch, cs.Provider, cs.Build.Summary, state)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SHA\tAGE\tAUTHOR\tDEPLOY\tMESSAGE")
	for _, c := range cs.Commits {
		age := "-"
		if !c.AuthoredAt.IsZero() {
			age = time.Since(c.AuthoredAt).Round(time.Minute).String()
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", shortSHA(c.SHA), age, dashIfEmpty(c.Author), dashIfEmpty(c.DeployStatus), dashIfEmpty(c.Message))
	}
	_ = tw.Flush()
	for _, s := range cs.Suggestions {
		_, _ = fmt.Fprintf(out, "\n[%s] %s: %s\n", s.Severity, s.ID, s.Reason)
	}
}
