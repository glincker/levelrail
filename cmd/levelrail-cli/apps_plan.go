package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsPlan prints the reviewable deploy plan for a repository. It never
// creates anything; applying is a separate action.
func runAppsPlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps plan", "print the plan as JSON to stdout and nothing else", stderr)
	var ref, dir string
	fs.StringVar(&ref, "ref", "", "branch, tag or commit to inspect (default: the repository default branch)")
	fs.StringVar(&dir, "dir", "", "app directory inside the repository, for a monorepo")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps plan <git-url> [--ref R] [--dir D] [flags]\n\nShows the detected stack, a draft spec, env variable names (never values),\nports and warnings for a repository. Nothing is created.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, repo, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps plan", "git URL"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.PlanRepoView(context.Background(), apiclient.RepoPlanViewRequest{RepoURL: repo, Ref: ref, BaseDirectory: dir})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan repository: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printRepoPlanView(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRepoPlanView(out io.Writer, p apiclient.RepoPlanView) {
	line := func(format string, v ...any) { _, _ = fmt.Fprintf(out, format+"\n", v...) }
	line("Stack:      %s %s (%s confidence)", dash(p.Stack.Runtime), dash(p.Stack.Framework), p.Stack.Confidence)
	for _, e := range p.Stack.Evidence {
		line("  - [%s] %s", e.Signal, e.Detail)
	}
	line("Build:      %s", p.Build.Type)
	if p.Build.BuildCommand != "" {
		line("Build cmd:  %s", p.Build.BuildCommand)
	}
	if p.Build.StartCommand != "" {
		line("Start cmd:  %s", p.Build.StartCommand)
	}
	if p.Port.Value > 0 {
		line("Port:       %d (%s)", p.Port.Value, p.Port.Evidence)
	} else {
		line("Port:       -")
	}
	line("Health:     %s", p.Health.Path)
	if len(p.Env) > 0 {
		line("\nEnvironment (values are never shown):")
		for _, e := range p.Env {
			tags := []string{}
			if e.Required {
				tags = append(tags, "required")
			}
			if e.LooksSecret {
				tags = append(tags, "secret")
			}
			line("  - %s %s(%s)", e.Name, tagList(tags), e.Source)
		}
	}
	line("\nDraft spec:\n%s", p.Spec.YAML)
	for _, w := range p.Warnings {
		line("warning: %s", w.Message)
	}
	line("\nNothing was created. To apply: %s %s", p.Next.Method, p.Next.Path)
}
