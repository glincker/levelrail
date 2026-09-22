package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runGiteaApp dispatches "gitea-app <verb> [flags]" to one of
// repos/branches/use-as-source: internal/api/gitea_app_repos.go's three
// routes, the Gitea counterpart of runGitHubApp/runGitLabApp/
// runBitbucketApp. Connecting the Gitea OAuth Application itself stays
// dashboard-only, same reasoning as the other three providers.
func runGiteaApp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, giteaAppUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, giteaAppUsage(prog))
		return exitOK
	case "status":
		return runGiteaAppStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "disconnect":
		return runGiteaAppDisconnect(prog, args[1:], stdout, stderr, lookupEnv)
	case "repos":
		return runGiteaAppRepos(prog, args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runGiteaAppBranches(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-as-source":
		return runGiteaAppUseAsSource(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown gitea-app subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, giteaAppUsage(prog))
		return exitUsage
	}
}

func giteaAppUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s gitea-app status [flags]                                        show the connection status
  %[1]s gitea-app disconnect [flags]                                    forget the stored connection (local only)
  %[1]s gitea-app repos [flags]                                         list repos the connected account can access
  %[1]s gitea-app branches <owner> <repo> [flags]                       list a repo's branches
  %[1]s gitea-app use-as-source <owner> <repo> --app-name NAME [flags]  connect a repo as an app's git source

Connecting the Gitea App itself is dashboard-only (a real browser
redirect through your Gitea instance's OAuth2 authorize flow); once
connected, these subcommands browse and use its repos from the CLI, or
check/forget the connection.

Run "%[1]s gitea-app <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runGiteaAppStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[giteaAppStatusResource]{
		cmdLabel:  "gitea-app status",
		jsonUsage: "print the connection status as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s gitea-app status [flags]\n\nShows the Gitea App connection status.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (giteaAppStatusResource, error) {
			return c.GetGiteaAppStatus(ctx)
		},
		errVerb: "get gitea app status",
		print:   printGiteaAppStatusHuman,
	})
}

func printGiteaAppStatusHuman(out io.Writer, s giteaAppStatusResource) {
	_, _ = fmt.Fprintf(out, "connected: %v\n", s.Connected)
	if !s.Connected {
		return
	}
	_, _ = fmt.Fprintf(out, "authorized:   %v\n", s.Authorized)
	_, _ = fmt.Fprintf(out, "instance_url: %s\n", s.InstanceURL)
	_, _ = fmt.Fprintf(out, "client_id:    %s\n", s.ClientID)
}

func runGiteaAppDisconnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "gitea-app disconnect", "print {\"disconnected\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s gitea-app disconnect [flags]\n\nForgets the stored Gitea App connection. Does not revoke the token\nor delete the Application on Gitea's own side.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DisconnectGiteaApp(context.Background()); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect gitea app: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"disconnected": true}, func() {
		_, _ = fmt.Fprintln(stdout, "gitea app disconnected")
	})
}

func runGiteaAppRepos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]giteaAppRepoResource]{
		cmdLabel:  "gitea-app repos",
		jsonUsage: "print repos as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s gitea-app repos [flags]\n\nLists every repository the connected Gitea account can access.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) ([]giteaAppRepoResource, error) {
			return c.ListGiteaAppRepos(ctx)
		},
		errVerb: "list gitea app repos",
		print:   printGiteaAppReposTable,
	})
}

func printGiteaAppReposTable(out io.Writer, repos []giteaAppRepoResource) {
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

func runGiteaAppBranches(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runTwoArgList(prog, args, stdout, stderr, lookupEnv, twoArgListParams[[]gitAppBranchResource]{
		cmdLabel:  "gitea-app branches",
		jsonUsage: "print branches as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s gitea-app branches <owner> <repo> [flags]\n\nLists a repo's branches.\n\nFlags:\n", prog),
		argsLabel: "an owner and a repo",
		fetch: func(c *Client, ctx context.Context, owner, repo string) ([]gitAppBranchResource, error) {
			return c.ListGiteaAppBranches(ctx, owner, repo)
		},
		errFmt: "list branches for %s/%s: %w",
		print:  printGitAppBranchesTable,
	})
}

func runGiteaAppUseAsSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runTwoArgUseAsSource(prog, args, stdout, stderr, lookupEnv, twoArgUseAsSourceParams[gitSourceResource]{
		cmdLabel:  "gitea-app use-as-source",
		usageText: fmt.Sprintf("Usage:\n  %s gitea-app use-as-source <owner> <repo> --app-name NAME [flags]\n\nConnects the repo as NAME's git source and registers a push webhook.\n\nFlags:\n", prog),
		argsLabel: "an owner and a repo",
		fetch: func(c *Client, ctx context.Context, owner, repo string, req useRepoAsSourceRequest) (gitSourceResource, error) {
			return c.UseGiteaRepoAsSource(ctx, owner, repo, req)
		},
		errFmt: "use %s/%s as source for app %q: %w",
		print:  printGitSourceHuman,
	})
}
