package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsGitSource dispatches "apps git-source <verb> [flags]" to one of
// get/set/delete, mirroring runAppsLogDrain's own dispatch shape. Wires
// internal/api/git_sources.go's three routes into the CLI for the first
// time: connecting a repo for auto-deploy-on-push was dashboard-only
// before this. Multi-service fan-out (Services/AdditionalServices on the
// wire type) stays dashboard-only; use "apps deploy-spec" for that case
// from the CLI.
func runAppsGitSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsGitSourceUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsGitSourceUsage(prog))
		return exitOK
	case "get":
		return runAppsGitSourceGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsGitSourceSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsGitSourceDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps git-source subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsGitSourceUsage(prog))
		return exitUsage
	}
}

func appsGitSourceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps git-source get <name> [flags]                              show an app's connected repo
  %[1]s apps git-source set <name> --repo-url URL [flags]              connect (or edit) a repo for auto-deploy-on-push
  %[1]s apps git-source delete <name> [flags]                           disconnect an app's repo

Run "%[1]s apps git-source <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsGitSourceGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps git-source get", "print the git source as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source get <name> [flags]\n\nShows an app's connected repo, if any.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, stderr, prog, "apps git-source get", lookupEnv)
	if !ok {
		return exitCode
	}

	gs, err := client.GetGitSource(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get git source for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, gs); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printGitSourceHuman(stdout, gs)
	return exitOK
}

func runAppsGitSourceSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps git-source set", "print the git source as JSON to stdout and nothing else", stderr)
	var repoURL, branch, buildType, buildPath, token string
	fs.StringVar(&repoURL, "repo-url", "", "repo URL to connect (required)")
	fs.StringVar(&branch, "branch", "", "branch to deploy on push (default: the server's default branch)")
	fs.StringVar(&buildType, "build-type", "", "dockerfile, railpack, or static (default: dockerfile)")
	fs.StringVar(&buildPath, "build-path", "", "path within the repo to build from")
	fs.StringVar(&token, "token-secret", "", "personal access token for a private repo; empty on an update leaves the stored token unchanged")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source set <name> --repo-url URL [flags]\n\nConnects a repo for auto-deploy-on-push, or edits an existing\nconnection. Multi-service fan-out (an app.yaml services: map) is\ndashboard-only; use \"apps deploy-spec\" from the CLI for that case.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, stderr, prog, "apps git-source set", lookupEnv)
	if !ok {
		return exitCode
	}
	if repoURL == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps git-source set requires --repo-url\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	gs, err := client.SetGitSource(context.Background(), name, setGitSourceRequest{
		RepoURL:   repoURL,
		Branch:    branch,
		BuildType: buildType,
		BuildPath: buildPath,
		Token:     token,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set git source for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, gs); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printGitSourceHuman(stdout, gs)
	return exitOK
}

func runAppsGitSourceDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps git-source delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source delete <name> [flags]\n\nDisconnects an app's repo: pushes stop triggering an auto-deploy.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, stderr, prog, "apps git-source delete", lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.DeleteGitSource(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete git source for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "git source disconnected for app %q\n", name)
	return exitOK
}

func printGitSourceHuman(out io.Writer, gs gitSourceResource) {
	_, _ = fmt.Fprintf(out, "app:            %s\n", gs.ServiceName)
	_, _ = fmt.Fprintf(out, "repo_url:       %s\n", gs.RepoURL)
	_, _ = fmt.Fprintf(out, "branch:         %s\n", gs.Branch)
	_, _ = fmt.Fprintf(out, "build_type:     %s\n", gs.BuildType)
	if gs.BuildPath != "" {
		_, _ = fmt.Fprintf(out, "build_path:     %s\n", gs.BuildPath)
	}
	_, _ = fmt.Fprintf(out, "has_token:      %t\n", gs.HasToken)
	_, _ = fmt.Fprintf(out, "webhook_url:    %s\n", gs.WebhookURL)
	_, _ = fmt.Fprintf(out, "preview_enabled: %t\n", gs.PreviewEnabled)
}
