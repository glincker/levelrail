package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsGitSourceCompose shows, or with --repo connects, a compose app's repository.
func runAppsGitSourceCompose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source compose", "print the compose git source as JSON to stdout and nothing else", stderr)
	repo := fs.String("repo", "", "repository URL (http, https or ssh); omit to show the current source")
	branch := fs.String("branch", "", "branch whose pushes redeploy the stack (default main)")
	path := fs.String("path", "", "compose file path relative to the repository root (default compose.yaml)")
	trigger := fs.String("trigger-mode", "", "push (default) or the other trigger modes git-source set accepts")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source compose <name> [--repo URL --branch B --path docker/compose.yml] [flags]\n\nDeploys an existing compose app from a compose file in a git repository.\nPushes to the branch redeploy the stack. The webhook secret is shown once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source compose", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	var gs apiclient.GitSourceResource
	var err error
	if *repo == "" {
		gs, err = client.GetComposeSource(context.Background(), name)
	} else {
		gs, err = client.SetComposeSource(context.Background(), name, apiclient.SetComposeSourceRequest{RepoURL: *repo, Branch: *branch, ComposePath: *path, TriggerMode: *trigger})
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("compose git source for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, gs, func() {
		printGitSourceHuman(stdout, gs)
		_, _ = fmt.Fprintf(stdout, "compose_path: %s\n", gs.ComposePath)
		if gs.WebhookSecret != "" {
			_, _ = fmt.Fprintf(stdout, "webhook_secret: %s (shown once, will not be shown again)\n", gs.WebhookSecret)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runAppsGitSourceComposeDeploy deploys the branch head of a compose app's repository now.
func runAppsGitSourceComposeDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source compose-deploy", "print the deploy result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source compose-deploy <name> [flags]\n\nDeploys the compose file at the branch head now. A protected environment\nrefuses this; push to the branch so the deploy goes through approval.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source compose-deploy", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.DeployComposeSource(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy compose app %q from git: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "compose app %q deployed from git: %d service(s)\n", name, len(res.Services))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
