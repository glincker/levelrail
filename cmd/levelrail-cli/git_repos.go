package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const gitReposPageSize = 500

func gitReposUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s git-providers repos <github|gitlab|bitbucket|gitea> [flags]

Lists every repository the provider connection can see, searched, filtered
and paged on the control plane from a cached list.

Flags:
  --search string         only repositories whose name contains every word
  --connection string     only repositories of one connection id (see "git-providers connections")
  --owner string          only repositories under one owner or organization
  --archived string       include (default), exclude or only archived repositories
  --forks string          include (default), exclude or only forks
  --limit int             page size (default 100, at most 500)
  --offset int            skip this many matches
  --all                   page through every match instead of one page
  --refresh               re-list from the provider now instead of using the cache
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read
  --json                  print the page as JSON to stdout, nothing else
  --output string         output format: json, table, or text
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runGitProvidersRepos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "git-providers repos", "print the page as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, gitReposUsage(prog)) }
	search := fs.String("search", "", "")
	connection := fs.String("connection", "", "")
	owner := fs.String("owner", "", "")
	archived := fs.String("archived", "", "")
	forks := fs.String("forks", "", "")
	limit := fs.Int("limit", 0, "")
	offset := fs.Int("offset", 0, "")
	all := fs.Bool("all", false, "")
	refresh := fs.Bool("refresh", false, "")

	client, provider, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "git-providers repos", "provider"}, lookupEnv)
	if !ok {
		return exitCode
	}

	params := apiclient.GitRepoListParams{
		Provider: provider, Connection: *connection, Search: *search, Owner: *owner, Archived: *archived, Forks: *forks,
		Limit: *limit, Offset: *offset, Refresh: *refresh,
	}
	if *all {
		params.Limit = gitReposPageSize
	}
	res, err := client.ListGitRepos(context.Background(), params)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list %s repositories: %w", provider, err))
	}
	for *all && res.HasMore && res.NextOffset != nil {
		params.Offset, params.Refresh = *res.NextOffset, false
		next, err := client.ListGitRepos(context.Background(), params)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list %s repositories: %w", provider, err))
		}
		res.Items = append(res.Items, next.Items...)
		res.HasMore, res.NextOffset = next.HasMore, next.NextOffset
	}

	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printGitReposHuman(stdout, res) })
}

func printGitReposHuman(out io.Writer, res apiclient.GitRepoListResource) {
	if len(res.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no repositories")
	} else {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "REPOSITORY\tOWNER\tACCOUNT\tPRIVATE\tARCHIVED\tFORK\tDEFAULT_BRANCH")
		for _, r := range res.Items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%v\t%v\t%s\n", r.FullName, r.Owner, accountLabel(r), r.Private, r.Archived, r.Fork, r.DefaultBranch)
		}
		_ = tw.Flush()
	}
	_, _ = fmt.Fprintf(out, "showing %d of %d matches (%d repositories loaded)", len(res.Items), res.Total, res.Loaded)
	if res.HasMore && res.NextOffset != nil {
		_, _ = fmt.Fprintf(out, "; more with --offset %d or --all, or narrow with --search", *res.NextOffset)
	}
	_, _ = fmt.Fprintln(out)
	if res.Truncated {
		_, _ = fmt.Fprintf(out, "warning: the account has more than the %d repositories this control plane lists; search cannot reach the rest\n", res.Cap)
	}
	if res.Stale {
		_, _ = fmt.Fprintf(out, "note: list cached at %s, refreshing\n", res.CachedAt.Format("2006-01-02 15:04:05Z07:00"))
	}
	if res.RateLimit != nil {
		_, _ = fmt.Fprintf(out, "note: the provider is rate limiting; next check at %s\n", res.RateLimit.NextCheckAt.Format("2006-01-02 15:04:05Z07:00"))
	}
	for _, e := range res.Errors {
		_, _ = fmt.Fprintf(out, "warning: could not list repos for %s: %s\n", e.AccountLogin, e.Error)
	}
}

func accountLabel(r apiclient.GitRepoResource) string {
	if r.Account != "" {
		return r.Account
	}
	return r.ConnectionLabel
}
