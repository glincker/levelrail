package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// scrapeTokenName is the fixed name that lets rotation find the previous scrape token.
const scrapeTokenName = "prometheus-scrape"

// runTokensScrape mints a read-only token for Prometheus and prints a ready
// scrape config. Rotation revokes earlier tokens of the same name afterwards.
func runTokensScrape(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens scrape", "print the token and scrape config as JSON to stdout and nothing else", stderr)
	var rotate bool
	var expiresInDays int
	fs.BoolVar(&rotate, "rotate", false, "revoke the existing scrape token(s) once the new one is created")
	fs.IntVar(&expiresInDays, "expires-in-days", 0, "token lifetime in days (default: the server's token lifetime policy)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensScrapeUsage(prog)) }
	if err := fs.Parse(args); err != nil {
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

	ctx := context.Background()
	client, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	existing, err := client.ListTokens(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: %w", err))
	}
	live := liveScrapeTokens(existing)
	if len(live) > 0 && !rotate {
		return reportError(stdout, stderr, jsonOut, newValidationError("a %q token already exists; pass --rotate to replace it", scrapeTokenName))
	}
	created, err := client.CreateToken(ctx, createTokenRequest{Name: scrapeTokenName, Abilities: []string{"read"}, ExpiresInDays: expiresInDays})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create scrape token: %w", err))
	}
	// Revoke after the new token exists so a failed mint never leaves Prometheus without one.
	for _, t := range live {
		if err := client.RevokeToken(ctx, t.ID); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke old scrape token %q: %w", t.ID, err))
		}
	}
	cfg := scrapeConfig(client.baseURL, created.Token)
	out := map[string]string{"id": created.ID, "token": created.Token, "scrape_config": cfg}
	return writeScheduledTaskResult(stdout, stderr, of, out, func() {
		_, _ = fmt.Fprintf(stdout, "scrape token %q created (read only, shown once)\n\n%s", created.ID, cfg)
	})
}

func liveScrapeTokens(all []tokenResource) []tokenResource {
	var live []tokenResource
	for _, t := range all {
		if t.Name == scrapeTokenName && t.RevokedAt == nil {
			live = append(live, t)
		}
	}
	return live
}

// scrapeConfig renders a Prometheus job using bearer credentials.
func scrapeConfig(baseURL, token string) string {
	scheme, host := "https", baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		scheme, host = u.Scheme, u.Host
	}
	return strings.Join([]string{
		"scrape_configs:",
		"  - job_name: levelrail",
		"    scheme: " + scheme,
		"    metrics_path: /metrics",
		"    authorization:",
		"      type: Bearer",
		"      credentials: " + token,
		"    static_configs:",
		"      - targets: ['" + host + "']",
		"",
	}, "\n")
}

func tokensScrapeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens scrape [--rotate] [flags]

Mints a read-only token named %[2]q and prints a Prometheus scrape config
that uses it. The token is shown once. With --rotate any earlier scrape
token is revoked after the new one is created. The endpoint can be turned
off without touching tokens: set observability.metrics_scrape_enabled to false.

Needs a username and password (prompted if omitted), like "tokens create".

Flags:
  --rotate                  replace the existing scrape token
  --expires-in-days int     token lifetime in days (default: server policy)
`, prog, scrapeTokenName)
}
