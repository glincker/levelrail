package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Idempotent server-side, so an already-revoked token reports the same success.
func runTokensRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens revoke", "print a JSON result object to stdout and nothing else", stderr)
	tokenFlagP := fs.String("token", "", "API token to revoke with, instead of a password login (overrides "+apiTokenEnvHelp+" and the credentials file)")
	expiredP := fs.Bool("expired", false, "revoke every expired token")
	nameP := fs.String("name", "", "revoke every token with this exact name")
	kindP := fs.String("kind", "", "revoke every token of this kind: emergency, agent, system or user")
	filterP := fs.String("filter", "", "revoke tokens matching key=value pairs, comma separated (q, status, ability, name, kind, expires_before)")
	dryRunP := fs.Bool("dry-run", false, "only show how many tokens would be revoked")
	yesP := fs.Bool("yes", false, "skip the confirmation question")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensRevokeUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	jsonOut := *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	rest := fs.Args()
	filter, ferr := bulkFilterFromFlags(*expiredP, *nameP, *kindP, *filterP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	if !filter.empty() || len(rest) > 1 {
		if !filter.empty() && len(rest) > 0 {
			_, _ = fmt.Fprintf(stderr, "%s: pass token ids or a filter, not both\n", prog)
			return exitValidation
		}
		bo := bulkRevokeOpts{ids: rest, dryRun: *dryRunP, yes: *yesP}
		if !filter.empty() {
			bo.filter = &filter
		}
		return revokeBulk(prog, bo, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, *tokenFlagP, of, lookupEnv, stdin, stdout, stderr)
	}
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: tokens revoke requires a token id, several ids, or a filter such as --expired\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	id := rest[0]

	ctx := context.Background()
	if tok := listTokenAuth(*tokenFlagP, *usernameP, *passwordP, *profileFlagP, prog, lookupEnv); tok != "" {
		baseURL := resolveAPIURL(*apiURLFlagP, lookupEnv, prog, resolveProfile(*profileFlagP, lookupEnv))
		err := revokeTokenWithBearer(ctx, baseURL, tok, id)
		if err == nil {
			return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": id, "status": "revoked"}, func() { _, _ = fmt.Fprintf(stdout, "token %q revoked\n", id) })
		}
		var apiErr *apiError
		if !errors.As(err, &apiErr) || (apiErr.StatusCode != http.StatusUnauthorized && apiErr.StatusCode != http.StatusForbidden) {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke token %q: %w", id, err))
		}
		// The server may only let a signed-in session revoke; say so rather than a CI-mode prompt error.
		if ciMode {
			return reportError(stdout, stderr, jsonOut, newValidationError("the server did not accept the API token for revoking; pass --username and --password to revoke with a session login"))
		}
	}
	sessionClient, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if err := sessionClient.RevokeToken(ctx, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke token %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": id, "status": "revoked"}, func() { _, _ = fmt.Fprintf(stdout, "token %q revoked\n", id) })
}

// bulkFilterFromFlags merges the shortcut flags into --filter, returning nil when no filter was asked for.
func bulkFilterFromFlags(expired bool, name, kind, raw string) (bulkRevokeFilter, error) {
	f, err := parseTokenFilter(raw)
	if err != nil {
		return f, err
	}
	if expired {
		f.Status = "expired"
	}
	if name != "" {
		f.Name = name
	}
	if kind != "" {
		f.Kind = kind
	}
	return f, nil
}

// revokeBulk needs a signed-in session: the server refuses bulk revoke for an API token.
func revokeBulk(prog string, o bulkRevokeOpts, sf sessionFlags, tokenFlag string, of outputFlags, lookupEnv func(string) (string, bool), stdin io.Reader, stdout, stderr io.Writer) int {
	if listTokenAuth(tokenFlag, sf.username, sf.password, sf.profileFlag, prog, lookupEnv) != "" {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, bulkRevokeAPITokenMessage)
		return exitValidation
	}
	ctx := context.Background()
	sc, _, err := loggedInSessionClient(ctx, sf, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, of.Format == "json", err)
	}
	res, err := runBulkRevoke(ctx, sc, o, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, of.Format == "json", err)
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch {
		case res.Count == 0:
			_, _ = fmt.Fprintln(stdout, "no tokens match, nothing to revoke")
		case res.DryRun:
			_, _ = fmt.Fprintf(stdout, "%d tokens would be revoked: %s%s\n", res.Count, strings.Join(res.Sample, ", "), moreLabel(res.Count, len(res.Sample)))
		default:
			_, _ = fmt.Fprintf(stdout, "revoked %d tokens\n", res.Revoked)
		}
	})
}

func tokensRevokeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens revoke <id>... [flags]
  %[1]s tokens revoke --expired | --name NAME | --kind KIND | --filter 'k=v,...' [--dry-run] [--yes]

Several ids or any filter flag revoke in bulk. Bulk revoke needs a signed-in
session (--username/--password), never an API token. It always previews first,
prints the count and a sample of names, asks to confirm unless --yes is given,
and skips revoked tokens and the token making the call.

Bulk flags:
  --expired                  revoke every expired token
  --name string              revoke every token with this exact name
  --kind string              emergency, agent, system or user
  --filter string            key=value pairs, comma separated: q, status, ability, name, kind, expires_before
  --dry-run                  only show the count and sample
  --yes                      skip the confirmation question

Revokes one API token by id (idempotent: revoking an already-revoked id
also reports success). Uses the API token from --token, %[4]s or the saved
profile when one is set; otherwise (or if the server wants a session) signs in
with --username/--password (prompted if omitted, never prompted in CI mode).

Flags:
  --token string             API token to revoke with (default: %[4]s env var, then the credentials file)
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print a JSON result object to stdout, nothing else
  --output string          output format: json, ndjson, yaml, table, wide, text or name (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiURLEnvHelp, defaultAPIURL, apiTokenEnvHelp)
}

// revokeTokenWithBearer calls DELETE /api/v1/auth/tokens/{id} with a bearer token, no session login.
func revokeTokenWithBearer(ctx context.Context, baseURL, token, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(baseURL, "/")+"/api/v1/auth/tokens/"+pathEscape(id), nil) //nolint:gosec // operator-supplied API URL, same as the client
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req) //nolint:gosec // same target as above
	if err != nil {
		return fmt.Errorf("request DELETE %s: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	msg := extractErrorMessage(body)
	if msg == "" {
		msg = resp.Status
	}
	return &apiError{StatusCode: resp.StatusCode, Message: msg}
}
