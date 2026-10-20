package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func gitConnectionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s git-providers connections [--provider P] [flags]            list every GitLab, Bitbucket and Gitea account
  %[1]s git-providers add <gitlab|bitbucket|gitea> [flags]           register another account's OAuth application
  %[1]s git-providers rename <id> <label> [flags]                    rename a connection
  %[1]s git-providers default <id> [flags]                           make a connection its provider's default
  %[1]s git-providers disconnect --id ID [flags]                     disconnect one account, others stay
  %[1]s git-providers branches --id ID --repo REPO [flags]           list a repository's branches
  %[1]s git-providers use-as-source --id ID --repo REPO --app-name NAME [flags]

Flags for add: --instance-url URL (GitLab, Gitea), --client-id ID, --client-secret SECRET,
--label LABEL, --reuse ID (copy the OAuth application of another connection).

Authorizing a registered account is a browser redirect: open the printed
authorize path on your control plane, or press Authorize in the dashboard.
REPO is a project id for GitLab and owner/name for Bitbucket and Gitea.
`, prog)
}

type gitConnArgs struct {
	prog       string
	cmd        string
	stdout     io.Writer
	stderr     io.Writer
	lookupEnv  func(string) (string, bool)
	jsonHelp   string
	usageLines string
}

func runGitConnectionsCmd(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "git-providers " + sub
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, gitConnectionsUsage(prog))
		fs.PrintDefaults()
	}
	provider := fs.String("provider", "", "only connections of this provider")
	id := fs.String("id", "", "connection id")
	repo := fs.String("repo", "", "repository: a project id for GitLab, owner/name otherwise")
	instance := fs.String("instance-url", "", "instance URL (GitLab and Gitea)")
	clientID := fs.String("client-id", "", "OAuth application client id (Bitbucket: the consumer key)")
	clientSecret := fs.String("client-secret", "", "OAuth application client secret")
	name := fs.String("label", "", "display label")
	reuse := fs.String("reuse", "", "reuse the OAuth application of this connection id")
	req := bindUseAsSourceFlags(fs)

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rest := fs.Args()
	need := func(n int, what string) bool {
		if len(rest) != n {
			_, _ = fmt.Fprintf(stderr, "%s: %s requires %s\n\n", prog, label, what)
			fs.Usage()
			return false
		}
		return true
	}
	fail := func(verb string, err error) int {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", verb, err))
	}

	switch sub {
	case "connections":
		conns, err := client.ListGitConnections(ctx, *provider)
		if err != nil {
			return fail("list git connections", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, conns, func() { printGitConnectionsHuman(stdout, conns) })
	case "add":
		if !need(1, "a provider") {
			return exitUsage
		}
		c, err := client.CreateGitConnection(ctx, apiclient.CreateGitConnectionRequest{
			Provider: rest[0], InstanceURL: *instance, ClientID: *clientID, ClientSecret: *clientSecret, Label: *name, ReuseConnectionID: *reuse,
		})
		if err != nil {
			return fail("add git connection", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, c, func() {
			_, _ = fmt.Fprintf(stdout, "registered %s (%s). Authorize it in a browser: %s%s\n", c.Label, c.ID, apiURLFlag, c.AuthorizePath)
		})
	case "rename":
		if !need(2, "<id> <label>") {
			return exitUsage
		}
		c, err := client.RenameGitConnection(ctx, rest[0], rest[1])
		if err != nil {
			return fail("rename git connection", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, c, func() { _, _ = fmt.Fprintf(stdout, "renamed to %s\n", c.Label) })
	case "default":
		if !need(1, "<id>") {
			return exitUsage
		}
		c, err := client.SetDefaultGitConnection(ctx, rest[0])
		if err != nil {
			return fail("set default git connection", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, c, func() { _, _ = fmt.Fprintf(stdout, "%s is now the default %s connection\n", c.Label, c.Provider) })
	case "disconnect":
		if *id == "" {
			_, _ = fmt.Fprintf(stderr, "%s: %s requires --id\n\n", prog, label)
			fs.Usage()
			return exitUsage
		}
		res, err := client.DisconnectGitConnection(ctx, *id)
		if err != nil {
			return fail("disconnect git connection", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			_, _ = fmt.Fprintf(stdout, "disconnected %s. %s\n", *id, res.Note)
			if len(res.AffectedSources) > 0 {
				_, _ = fmt.Fprintf(stdout, "needs reconnect: %s\n", strings.Join(res.AffectedSources, ", "))
			}
		})
	case "branches":
		if *id == "" || *repo == "" {
			_, _ = fmt.Fprintf(stderr, "%s: %s requires --id and --repo\n\n", prog, label)
			fs.Usage()
			return exitUsage
		}
		branches, err := client.ListGitConnectionBranches(ctx, *id, *repo)
		if err != nil {
			return fail("list branches", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, branches, func() { printGitAppBranchesTable(stdout, branches) })
	default:
		if *id == "" || *repo == "" || req.AppName == "" {
			_, _ = fmt.Fprintf(stderr, "%s: %s requires --id, --repo and --app-name\n\n", prog, label)
			fs.Usage()
			return exitUsage
		}
		req.Repo = *repo
		res, err := client.UseGitConnectionRepoAsSource(ctx, *id, *req)
		if err != nil {
			return fail(fmt.Sprintf("use %s as source for app %q", *repo, req.AppName), err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() { printGitSourceHuman(stdout, res) })
	}
}

func printGitConnectionsHuman(out io.Writer, conns []apiclient.GitProviderConnectionResource) {
	if len(conns) == 0 {
		_, _ = fmt.Fprintln(out, "no GitLab, Bitbucket or Gitea accounts are connected")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tPROVIDER\tLABEL\tACCOUNT\tHOST\tDEFAULT\tSTATE\tSOURCES")
	for _, c := range conns {
		state := c.Connection.Status
		switch {
		case !c.Authorized:
			state = "not authorized"
		case state == "":
			state = "ok"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%v\t%s\t%d\n", c.ID, c.Provider, c.Label, c.AccountName, c.Host, c.IsDefault, state, c.SourceCount)
	}
	_ = tw.Flush()
	for _, c := range conns {
		if !c.Authorized || c.Connection.Status == "needs_reauthorize" || c.Connection.Status == "revoked" {
			_, _ = fmt.Fprintf(out, "%s: needs authorization. Press Reconnect in the dashboard, or open %s in a browser.\n", c.Label, c.AuthorizePath)
		}
	}
}
