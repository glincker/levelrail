package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runGitHubApp dispatches "github-app <verb> [flags]" to one of
// repos/branches/use-as-source: internal/api/github_app_repos.go and
// github_app_use_as_source.go's three routes, wired into the CLI for the
// first time. The initial OAuth/manifest connect flow
// (GET/PUT/DELETE /api/v1/github-app, register/callback/installed)
// stays dashboard-only deliberately: it's a real, full-page browser
// navigation through GitHub's own manifest flow, not something a
// scriptable client can drive.
func runGitHubApp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, githubAppUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, githubAppUsage(prog))
		return exitOK
	case "repos":
		return runGitHubAppRepos(prog, args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runGitHubAppBranches(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-as-source":
		return runGitHubAppUseAsSource(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown github-app subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, githubAppUsage(prog))
		return exitUsage
	}
}

func githubAppUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s github-app repos [flags]                                          list repos the connected installation can access
  %[1]s github-app branches <owner> <repo> [flags]                        list a repo's branches
  %[1]s github-app use-as-source <owner> <repo> --app-name NAME [flags]   connect a repo as an app's git source

Connecting the GitHub App itself is dashboard-only (it's a real browser
redirect through GitHub's manifest flow); once connected, these
subcommands browse and use its repos from the CLI.

Run "%[1]s github-app <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runGitHubAppRepos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "github-app repos", "print repos as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s github-app repos [flags]\n\nLists every repository the connected GitHub App installation can access.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	repos, err := client.ListGitHubAppRepos(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list github app repos: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, repos, func() { printGitHubAppReposTable(stdout, repos) })
}

func printGitHubAppReposTable(out io.Writer, repos []gitHubAppRepoResource) {
	if len(repos) == 0 {
		_, _ = fmt.Fprintln(out, "no repositories")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FULL_NAME\tPRIVATE\tDEFAULT_BRANCH")
	for _, r := range repos {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\n", r.FullName, r.Private, r.DefaultBranch)
	}
	_ = tw.Flush()
}

func runGitHubAppBranches(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "github-app branches", "print branches as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s github-app branches <owner> <repo> [flags]\n\nLists a repo's branches.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "github-app branches", "an owner and a repo", 2)
	if !ok {
		return exitUsage
	}
	owner, repo := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	branches, err := client.ListGitHubAppBranches(context.Background(), owner, repo)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list branches for %s/%s: %w", owner, repo, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, branches, func() { printGitAppBranchesTable(stdout, branches) })
}

func printGitAppBranchesTable(out io.Writer, branches []gitAppBranchResource) {
	if len(branches) == 0 {
		_, _ = fmt.Fprintln(out, "no branches")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCOMMIT_SHA")
	for _, b := range branches {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", b.Name, b.CommitSHA)
	}
	_ = tw.Flush()
}

func runGitHubAppUseAsSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "github-app use-as-source", "print the resulting git source as JSON to stdout and nothing else", stderr)
	req := bindUseAsSourceFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s github-app use-as-source <owner> <repo> --app-name NAME [flags]\n\nConnects the repo as NAME's git source and registers a push webhook\nusing the installation token.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "github-app use-as-source", "an owner and a repo", 2)
	if !ok {
		return exitUsage
	}
	owner, repo := rest[0], rest[1]
	if req.AppName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: github-app use-as-source requires --app-name\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.UseGitHubRepoAsSource(context.Background(), owner, repo, *req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("use %s/%s as source for app %q: %w", owner, repo, req.AppName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		printGitSourceHuman(stdout, result.GitSourceResource)
		_, _ = fmt.Fprintf(stdout, "webhook_registered: %v\n", result.WebhookRegistered)
		if result.WebhookError != "" {
			_, _ = fmt.Fprintf(stdout, "webhook_error:       %s\n", result.WebhookError)
		}
	})
}

// bindUseAsSourceFlags registers the four flags every provider's own
// use-as-source subcommand shares (app-name/branch/build-type/build-path),
// the same request shape internal/api's useRepoAsSourceRequest defines
// for all three providers.
func bindUseAsSourceFlags(fs *flag.FlagSet) *useRepoAsSourceRequest {
	req := &useRepoAsSourceRequest{}
	fs.StringVar(&req.AppName, "app-name", "", "app to connect this repo to (required)")
	fs.StringVar(&req.Branch, "branch", "", "branch to deploy on push (default: the repo's default branch)")
	fs.StringVar(&req.BuildType, "build-type", "", "dockerfile, railpack, or static (default: dockerfile)")
	fs.StringVar(&req.BuildPath, "build-path", "", "path within the repo to build from")
	return req
}
