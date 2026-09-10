package main

import (
	"context"
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
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]gitHubAppRepoResource]{
		cmdLabel:  "github-app repos",
		jsonUsage: "print repos as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s github-app repos [flags]\n\nLists every repository the connected GitHub App installation can access.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) ([]gitHubAppRepoResource, error) {
			return c.ListGitHubAppRepos(ctx)
		},
		errVerb: "list github app repos",
		print:   printGitHubAppReposTable,
	})
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
	return runTwoArgList(prog, args, stdout, stderr, lookupEnv, twoArgListParams[[]gitAppBranchResource]{
		cmdLabel:  "github-app branches",
		jsonUsage: "print branches as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s github-app branches <owner> <repo> [flags]\n\nLists a repo's branches.\n\nFlags:\n", prog),
		argsLabel: "an owner and a repo",
		fetch: func(c *Client, ctx context.Context, owner, repo string) ([]gitAppBranchResource, error) {
			return c.ListGitHubAppBranches(ctx, owner, repo)
		},
		errFmt: "list branches for %s/%s: %w",
		print:  printGitAppBranchesTable,
	})
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
	return runTwoArgUseAsSource(prog, args, stdout, stderr, lookupEnv, twoArgUseAsSourceParams[useGitHubRepoAsSourceResponse]{
		cmdLabel:  "github-app use-as-source",
		usageText: fmt.Sprintf("Usage:\n  %s github-app use-as-source <owner> <repo> --app-name NAME [flags]\n\nConnects the repo as NAME's git source and registers a push webhook\nusing the installation token.\n\nFlags:\n", prog),
		argsLabel: "an owner and a repo",
		fetch: func(c *Client, ctx context.Context, owner, repo string, req useRepoAsSourceRequest) (useGitHubRepoAsSourceResponse, error) {
			return c.UseGitHubRepoAsSource(ctx, owner, repo, req)
		},
		errFmt: "use %s/%s as source for app %q: %w",
		print: func(out io.Writer, result useGitHubRepoAsSourceResponse) {
			printGitSourceHuman(out, result.GitSourceResource)
			_, _ = fmt.Fprintf(out, "webhook_registered: %v\n", result.WebhookRegistered)
			if result.WebhookError != "" {
				_, _ = fmt.Fprintf(out, "webhook_error:       %s\n", result.WebhookError)
			}
		},
	})
}
