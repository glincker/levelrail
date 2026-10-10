package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
)

// runGitHubApp dispatches "github-app <verb> [flags]" to one of
// status/disconnect/repos/branches/use-as-source: internal/api/github_app.go
// and github_app_repos.go/github_app_use_as_source.go's routes, wired
// into the CLI. The initial OAuth/manifest connect (PUT-equivalent,
// register/callback/installed) stays dashboard-only deliberately: it's a
// real, full-page browser navigation through GitHub's own manifest
// flow, not something a scriptable client can drive. status (GET) and
// disconnect (DELETE) need no browser at all, so those are CLI-reachable.
func runGitHubApp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, githubAppUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, githubAppUsage(prog))
		return exitOK
	case "status":
		return runGitHubAppStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "disconnect":
		return runGitHubAppDisconnect(prog, args[1:], stdout, stderr, lookupEnv)
	case "repos":
		return runGitHubAppRepos(prog, args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runGitHubAppBranches(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-as-source":
		return runGitHubAppUseAsSource(prog, args[1:], stdout, stderr, lookupEnv)
	case "register-url":
		return runGitHubAppRegisterURL(prog, args[1:], stdout, stderr, lookupEnv)
	case "installations":
		return runGitHubAppInstallations(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown github-app subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, githubAppUsage(prog))
		return exitUsage
	}
}

func githubAppUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s github-app status [flags]                                         show the connection status
  %[1]s github-app disconnect [flags]                                     forget the stored connection (local only)
  %[1]s github-app repos [flags]                                          list repos every connected installation can access
  %[1]s github-app branches <owner> <repo> [flags]                        list a repo's branches
  %[1]s github-app use-as-source <owner> <repo> --app-name NAME [flags]   connect a repo as an app's git source
  %[1]s github-app register-url [--owner ORG] [--public] [flags]          print the URL that starts App registration in a browser
  %[1]s github-app installations list [flags]                             list every connected account/org
  %[1]s github-app installations add [flags]                              print the URL to install the App on another account/org
  %[1]s github-app installations remove <id> [flags]                      disconnect one account/org

Connecting the GitHub App itself happens in a browser (it's a real redirect
through GitHub's manifest flow): register-url prints the link for a personal
account or an organization. Once connected, these
subcommands browse and use its repos from the CLI, or check/forget the
connection.

Run "%[1]s github-app <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runGitHubAppStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[gitHubAppStatusResource]{
		cmdLabel:  "github-app status",
		jsonUsage: "print the connection status as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s github-app status [flags]\n\nShows the GitHub App connection status.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (gitHubAppStatusResource, error) {
			return c.GetGitHubAppStatus(ctx)
		},
		errVerb: "get github app status",
		print:   printGitHubAppStatusHuman,
	})
}

func printGitHubAppStatusHuman(out io.Writer, s gitHubAppStatusResource) {
	_, _ = fmt.Fprintf(out, "connected: %v\n", s.Connected)
	if !s.Connected {
		return
	}
	_, _ = fmt.Fprintf(out, "installed:           %v\n", s.Installed)
	_, _ = fmt.Fprintf(out, "account_login:       %s\n", s.AccountLogin)
	_, _ = fmt.Fprintf(out, "instance_url:        %s\n", s.InstanceURL)
	_, _ = fmt.Fprintf(out, "installation_status: %s\n", s.InstallationStatus)
}

func runGitHubAppDisconnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "github-app disconnect", "print {\"disconnected\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s github-app disconnect [flags]\n\nForgets the stored GitHub App connection. Does not uninstall or delete\nthe App on GitHub's own side.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DisconnectGitHubApp(context.Background()); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect github app: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"disconnected": true}, func() {
		_, _ = fmt.Fprintln(stdout, "github app disconnected")
	})
}

func runGitHubAppRepos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[gitHubAppRepoListResource]{
		cmdLabel:  "github-app repos",
		jsonUsage: "print {repos, errors} as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s github-app repos [flags]\n\nLists every repository every connected installation can access.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (gitHubAppRepoListResource, error) {
			return c.ListGitHubAppRepos(ctx)
		},
		errVerb: "list github app repos",
		print:   printGitHubAppReposTable,
	})
}

func printGitHubAppReposTable(out io.Writer, list gitHubAppRepoListResource) {
	if len(list.Repos) == 0 {
		_, _ = fmt.Fprintln(out, "no repositories")
	} else {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FULL_NAME\tPRIVATE\tDEFAULT_BRANCH\tACCOUNT_TYPE")
		for _, r := range list.Repos {
			_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\t%s\n", r.FullName, r.Private, r.DefaultBranch, r.AccountType)
		}
		_ = tw.Flush()
	}
	for _, e := range list.Errors {
		_, _ = fmt.Fprintf(out, "warning: could not list repos for %s: %s\n", e.AccountLogin, e.Error)
	}
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

// runGitHubAppInstallations dispatches "github-app installations <verb>"
// to list/add/remove: internal/api/github_app_installations.go's
// routes, wired into the CLI the same way the connection's own
// repos/branches routes are above. "add" never drives GitHub's install
// flow itself (another real, full-page browser redirect), it only
// prints the URL to start it, the same reasoning connecting the App
// itself stays dashboard-only for.
func runGitHubAppInstallations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, githubAppInstallationsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, githubAppInstallationsUsage(prog))
		return exitOK
	case "list":
		return runGitHubAppInstallationsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runGitHubAppInstallationsAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runGitHubAppInstallationsRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown github-app installations subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, githubAppInstallationsUsage(prog))
		return exitUsage
	}
}

func githubAppInstallationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s github-app installations list [flags]        list every connected account/org
  %[1]s github-app installations add [flags]         print the URL to install the App on another account/org
  %[1]s github-app installations remove <id> [flags] disconnect one account/org

Run "%[1]s github-app installations <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runGitHubAppInstallationsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[gitHubAppInstallationListResource]{
		cmdLabel:  "github-app installations list",
		jsonUsage: "print {installations, add_org_url} as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s github-app installations list [flags]\n\nLists every GitHub account/org the App is connected to.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (gitHubAppInstallationListResource, error) {
			return c.ListGitHubAppInstallations(ctx)
		},
		errVerb: "list github app installations",
		print:   printGitHubAppInstallationsTable,
	})
}

func printGitHubAppInstallationsTable(out io.Writer, list gitHubAppInstallationListResource) {
	if len(list.Installations) == 0 {
		_, _ = fmt.Fprintln(out, "no connected accounts")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tACCOUNT_LOGIN\tACCOUNT_TYPE\tCONNECTED_AT")
	for _, inst := range list.Installations {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", inst.ID, inst.AccountLogin, inst.AccountType, inst.ConnectedAt)
	}
	_ = tw.Flush()
}

func runGitHubAppInstallationsAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[gitHubAppInstallationListResource]{
		cmdLabel:  "github-app installations add",
		jsonUsage: "print {installations, add_org_url} as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s github-app installations add [flags]\n\nPrints the URL to install the App on another GitHub account/org.\nDoes not open a browser or drive the install flow itself: visit the\nprinted URL yourself, the same way the App's own initial connect is a\nreal browser redirect, not something this CLI can script.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (gitHubAppInstallationListResource, error) {
			return c.ListGitHubAppInstallations(ctx)
		},
		errVerb: "get github app add-org url",
		print: func(out io.Writer, list gitHubAppInstallationListResource) {
			if list.AddOrgURL == "" {
				_, _ = fmt.Fprintln(out, "no add-org URL on record: reconnect the GitHub App from the dashboard (Settings > GitHub App) to enable this")
				return
			}
			_, _ = fmt.Fprintln(out, list.AddOrgURL)
		},
	})
}

func runGitHubAppInstallationsRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "github-app installations remove", "print {\"removed\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s github-app installations remove <id> [flags]\n\nDisconnects one account/org. Refused (409) while a git source still\npoints at a repo under it; disconnect or move those git sources first.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	idRaw, ok := requireOneArg(fs, stderr, prog, "github-app installations remove", "installation id")
	if !ok {
		return exitUsage
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: github-app installations remove: %q is not a valid id\n", prog, idRaw)
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteGitHubAppInstallation(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove github app installation %d: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"removed": true}, func() {
		_, _ = fmt.Fprintf(stdout, "installation %d removed\n", id)
	})
}
