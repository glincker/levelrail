package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runBitbucketApp dispatches "bitbucket-app <verb> [flags]" to one of
// repos/branches/use-as-source: internal/api/bitbucket_app_repos.go's
// three routes, the Bitbucket counterpart of runGitHubApp/runGitLabApp.
// Connecting the Bitbucket OAuth consumer itself stays dashboard-only,
// same reasoning as the other two providers.
func runBitbucketApp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, bitbucketAppUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, bitbucketAppUsage(prog))
		return exitOK
	case "status":
		return runBitbucketAppStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "disconnect":
		return runBitbucketAppDisconnect(prog, args[1:], stdout, stderr, lookupEnv)
	case "repos":
		return runBitbucketAppRepos(prog, args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runBitbucketAppBranches(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-as-source":
		return runBitbucketAppUseAsSource(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown bitbucket-app subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, bitbucketAppUsage(prog))
		return exitUsage
	}
}

func bitbucketAppUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s bitbucket-app status [flags]                                               show the connection status
  %[1]s bitbucket-app disconnect [flags]                                           forget the stored connection (local only)
  %[1]s bitbucket-app repos [flags]                                                list repos the connected account can access
  %[1]s bitbucket-app branches <workspace> <repo-slug> [flags]                     list a repo's branches
  %[1]s bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [flags]   connect a repo as an app's git source

Connecting the Bitbucket App itself is dashboard-only (a real browser
redirect through Bitbucket's OAuth consumer flow); once connected, these
subcommands browse and use its repos from the CLI, or check/forget the
connection.

Run "%[1]s bitbucket-app <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runBitbucketAppStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[bitbucketAppStatusResource]{
		cmdLabel:  "bitbucket-app status",
		jsonUsage: "print the connection status as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s bitbucket-app status [flags]\n\nShows the Bitbucket App connection status.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (bitbucketAppStatusResource, error) {
			return c.GetBitbucketAppStatus(ctx)
		},
		errVerb: "get bitbucket app status",
		print:   printBitbucketAppStatusHuman,
	})
}

func printBitbucketAppStatusHuman(out io.Writer, s bitbucketAppStatusResource) {
	_, _ = fmt.Fprintf(out, "connected: %v\n", s.Connected)
	if !s.Connected {
		return
	}
	_, _ = fmt.Fprintf(out, "authorized: %v\n", s.Authorized)
	_, _ = fmt.Fprintf(out, "key:        %s\n", s.Key)
}

func runBitbucketAppDisconnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bitbucket-app disconnect", "print {\"disconnected\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s bitbucket-app disconnect [flags]\n\nForgets the stored Bitbucket App connection. Does not revoke the token\nor delete the consumer on Bitbucket's own side.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DisconnectBitbucketApp(context.Background()); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect bitbucket app: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"disconnected": true}, func() {
		_, _ = fmt.Fprintln(stdout, "bitbucket app disconnected")
	})
}

func runBitbucketAppRepos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]bitbucketAppRepoResource]{
		cmdLabel:  "bitbucket-app repos",
		jsonUsage: "print repos as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s bitbucket-app repos [flags]\n\nLists every repository the connected Bitbucket account can access.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) ([]bitbucketAppRepoResource, error) {
			return c.ListBitbucketAppRepos(ctx)
		},
		errVerb: "list bitbucket app repos",
		print:   printBitbucketAppReposTable,
	})
}

func printBitbucketAppReposTable(out io.Writer, repos []bitbucketAppRepoResource) {
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

func runBitbucketAppBranches(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runTwoArgList(prog, args, stdout, stderr, lookupEnv, twoArgListParams[[]gitAppBranchResource]{
		cmdLabel:  "bitbucket-app branches",
		jsonUsage: "print branches as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s bitbucket-app branches <workspace> <repo-slug> [flags]\n\nLists a repo's branches.\n\nFlags:\n", prog),
		argsLabel: "a workspace and a repo slug",
		fetch: func(c *Client, ctx context.Context, workspace, repoSlug string) ([]gitAppBranchResource, error) {
			return c.ListBitbucketAppBranches(ctx, workspace, repoSlug)
		},
		errFmt: "list branches for %s/%s: %w",
		print:  printGitAppBranchesTable,
	})
}

func runBitbucketAppUseAsSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runTwoArgUseAsSource(prog, args, stdout, stderr, lookupEnv, twoArgUseAsSourceParams[gitSourceResource]{
		cmdLabel:  "bitbucket-app use-as-source",
		usageText: fmt.Sprintf("Usage:\n  %s bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [flags]\n\nConnects the repo as NAME's git source and registers a push webhook.\n\nFlags:\n", prog),
		argsLabel: "a workspace and a repo slug",
		fetch: func(c *Client, ctx context.Context, workspace, repoSlug string, req useRepoAsSourceRequest) (gitSourceResource, error) {
			return c.UseBitbucketRepoAsSource(ctx, workspace, repoSlug, req)
		},
		errFmt: "use %s/%s as source for app %q: %w",
		print:  printGitSourceHuman,
	})
}
