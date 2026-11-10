package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func isAPINotFound(err error) bool {
	var apiErr *apiclient.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func noGitSourceHint(prog, name string) string {
	return fmt.Sprintf("\n%[2]s has no git source yet (deployed from an image, an upload or not at all). Attach one without recreating the app:\n"+
		"  %[1]s apps git-source plan %[2]s --repo https://github.com/org/repo   preview what is kept and the suggested build\n"+
		"  %[1]s apps git-source set %[2]s --repo https://github.com/org/repo --branch main --deploy\n", prog, name)
}

func runAppsGitSourcePlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source plan", "print the plan as JSON to stdout and nothing else", stderr)
	var repoURL, branch, connectionID string
	fs.StringVar(&repoURL, "repo", "", "repo URL to preview; empty shows only what the app keeps")
	fs.StringVar(&repoURL, "repo-url", "", "alias for --repo")
	fs.StringVar(&branch, "branch", "", "branch to read (default: the server's default branch)")
	fs.StringVar(&connectionID, "connection", "", "provider connection id whose token reads a private repo (see \"git-connections list\")")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s apps git-source plan <name> [--repo URL --branch B --connection ID] [flags]\n\nPreviews attaching a repo to an existing app: its current source, the env,\ndomains and volumes that stay, the other services of the app and the\nsuggested build. Changes nothing.\n\nExamples:\n  %[1]s apps git-source plan web --repo https://github.com/org/web\n  %[1]s apps git-source plan web --repo https://gitlab.com/org/private --connection gc_123\n  %[1]s apps git-source plan web -o json\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source plan", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	plan, err := client.PlanGitSource(context.Background(), name, apiclient.GitSourcePlanOptions{RepoURL: repoURL, Branch: branch, ConnectionID: connectionID})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan git source for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, plan, func() { printGitSourcePlan(stdout, prog, plan) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printGitSourcePlan(out io.Writer, prog string, p apiclient.GitSourcePlan) {
	_, _ = fmt.Fprintf(out, "app:            %s\n", p.App)
	_, _ = fmt.Fprintf(out, "current source: %s\n", p.CurrentSource)
	if p.CurrentRepo != "" {
		_, _ = fmt.Fprintf(out, "current repo:   %s\n", p.CurrentRepo)
	}
	if p.Preserved.RunningImage != "" {
		_, _ = fmt.Fprintf(out, "running image:  %s (serves until the first build succeeds, stays in history for rollback)\n", p.Preserved.RunningImage)
	}
	_, _ = fmt.Fprintf(out, "kept:           %d env vars, %d secrets, %d domains, %d volumes\n", p.Preserved.EnvVars, p.Preserved.SecretEnv, len(p.Preserved.Domains), len(p.Preserved.Volumes))
	if s := p.Suggestion; s != nil {
		_, _ = fmt.Fprintf(out, "suggested:      build_type=%s", s.BuildType)
		for _, kv := range [][2]string{{"base_directory", s.BaseDirectory}, {"build_path", s.BuildPath}, {"build_command", s.BuildCommand}, {"publish_dir", s.PublishDir}, {"framework", s.Framework}} {
			if kv[1] != "" {
				_, _ = fmt.Fprintf(out, " %s=%q", kv[0], kv[1])
			}
		}
		_, _ = fmt.Fprintln(out)
	}
	if p.DetectError != "" {
		_, _ = fmt.Fprintf(out, "detect:         %s, pick the build by hand or pass --token-secret on set for a private repo\n", p.DetectError)
	}
	if len(p.Warnings) > 0 {
		_, _ = fmt.Fprintf(out, "notes:          %s\n", strings.Join(p.Warnings, ", "))
	}
	for _, m := range p.Group {
		_, _ = fmt.Fprintf(out, "also in app:    %s (%s): %s\n", m.Name, m.CurrentSource, strings.ReplaceAll(m.Effect, "_", " "))
	}
	if s := p.Suggestion; s != nil && p.Branch != "" {
		_, _ = fmt.Fprintf(out, "next:           %s apps git-source set %s --repo <url> --branch %s --build-pack %s --deploy\n", prog, p.App, p.Branch, s.BuildType)
	}
}

func runAppsGitSourceDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source deploy", "print the result as JSON to stdout and nothing else", stderr)
	var branch string
	fs.StringVar(&branch, "branch", "", "branch to build (default: the connected branch)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s apps git-source deploy <name> [--branch B] [flags]\n\nBuilds the newest commit of the connected branch and deploys it. The running\nimage keeps serving until the build succeeds. Follow it with \"apps deploys\".\n\nExamples:\n  %[1]s apps git-source deploy web\n  %[1]s apps git-source deploy web --branch release\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source deploy", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.DeployGitSource(context.Background(), name, branch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy git source for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { _, _ = fmt.Fprint(stdout, gitSourceDeployStartedMsg(res)) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func gitSourceDeployStartedMsg(res apiclient.GitSourceDeployResult) string {
	return fmt.Sprintf("build started for %s from branch %s; the current version serves until it succeeds\n", res.App, res.Branch)
}

// startGitSourceDeploy is set --deploy's follow-up: set already printed its
// own document, so JSON mode only notes the build on stderr.
func startGitSourceDeploy(client *Client, name, branch string, stdout, stderr io.Writer, jsonOut bool) int {
	res, err := client.DeployGitSource(context.Background(), name, branch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy git source for app %q: %w", name, err))
	}
	msg := gitSourceDeployStartedMsg(res)
	if jsonOut {
		_, _ = fmt.Fprint(stderr, msg)
		return exitOK
	}
	_, _ = fmt.Fprint(stdout, msg)
	return exitOK
}
