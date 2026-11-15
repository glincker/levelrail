package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// rotateResult is what both rotate commands print: the new secret appears here once and nowhere else.
type rotateResult struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Token              string     `json:"token"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	Replaces           string     `json:"replaces"`
	OldTokenValidUntil *time.Time `json:"old_token_valid_until"`
	CredentialsUpdated bool       `json:"credentials_updated,omitempty"`
}

func toRotateResult(r apiclient.RotatedToken) rotateResult {
	return rotateResult{ID: r.ID, Name: r.Name, Token: r.Token, ExpiresAt: r.ExpiresAt, Replaces: r.Replaces, OldTokenValidUntil: r.OldTokenValidUntil}
}

func printRotateResult(out io.Writer, r rotateResult, note string) {
	_, _ = fmt.Fprintf(out, "rotated token %q (%s), replaces %s\n", r.Name, r.ID, r.Replaces)
	if r.ExpiresAt != nil {
		_, _ = fmt.Fprintf(out, "expires:   %s\n", r.ExpiresAt.Local().Format("2006-01-02"))
	}
	if r.OldTokenValidUntil != nil {
		_, _ = fmt.Fprintf(out, "old token keeps working until %s\n", r.OldTokenValidUntil.Local().Format(time.RFC3339))
	}
	if note != "" {
		_, _ = fmt.Fprintln(out, note)
		return
	}
	_, _ = fmt.Fprintf(out, "new token (shown once): %s\n", r.Token)
}

// pickRotationTarget resolves an id, or a name that matches exactly one live token.
func pickRotationTarget(tokens []tokenResource, key string) (string, error) {
	var byName []string
	for _, t := range tokens {
		if t.RevokedAt != nil {
			continue
		}
		if t.ID == key {
			return t.ID, nil
		}
		if t.Name == key {
			byName = append(byName, t.ID)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return "", newValidationError("no active token with id or name %q; run tokens list", key)
	}
	return "", newValidationError("%d tokens are named %q; rotate by id: %s", len(byName), key, strings.Join(byName, ", "))
}

// runTokensRotate implements "tokens rotate <id|name>".
func runTokensRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens rotate", "print the new token as JSON to stdout and nothing else", stderr)
	tokenFlagP := fs.String("token", "", "API token to rotate with, instead of a password login (overrides "+apiTokenEnvHelp+" and the credentials file)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensRotateUsage(prog)) }
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
	if len(fs.Args()) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: tokens rotate requires exactly one token id or name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	key := fs.Args()[0]
	ctx := context.Background()

	if tok := listTokenAuth(*tokenFlagP, *usernameP, *passwordP, *profileFlagP, prog, lookupEnv); tok != "" {
		client := NewClient(resolveAPIURL(*apiURLFlagP, lookupEnv, prog, resolveProfile(*profileFlagP, lookupEnv)), tok)
		res, err := rotateWithBearer(ctx, client, key)
		var apiErr *apiError
		if err == nil || !errors.As(err, &apiErr) || (apiErr.StatusCode != http.StatusUnauthorized && apiErr.StatusCode != http.StatusForbidden) || ciMode {
			if err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate token %q: %w", key, err))
			}
			r := toRotateResult(res)
			return writeScheduledTaskResult(stdout, stderr, of, r, func() { printRotateResult(stdout, r, "") })
		}
	}
	sc, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	tokens, err := sc.ListTokens(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: %w", err))
	}
	id, err := pickRotationTarget(tokens, key)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res, err := sc.RotateToken(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate token %q: %w", key, err))
	}
	r := toRotateResult(res)
	return writeScheduledTaskResult(stdout, stderr, of, r, func() { printRotateResult(stdout, r, "") })
}

func rotateWithBearer(ctx context.Context, client *Client, key string) (apiclient.RotatedToken, error) {
	raw, err := client.ListTokensRaw(ctx)
	if err != nil {
		return apiclient.RotatedToken{}, err
	}
	tokens, err := decodeTokenList(raw)
	if err != nil {
		return apiclient.RotatedToken{}, err
	}
	id, err := pickRotationTarget(tokens, key)
	if err != nil {
		return apiclient.RotatedToken{}, err
	}
	return client.RotateToken(ctx, id)
}

// runAuthRotate implements "auth rotate": replace the current profile's token and rewrite its credentials entry.
func runAuthRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth rotate", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authRotateUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	profile := resolveProfile(profileFlag, lookupEnv)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	if token == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("no token for profile %q: nothing to rotate", profile))
	}
	baseURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := NewClient(baseURL, token)
	me, err := client.Whoami(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check current token: %w", err))
	}
	if me.Kind != "token" || me.TokenID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("the current credential is not an API token; sign in again with %s", loginFix(prog, profile, baseURL)))
	}
	res, err := client.RotateToken(ctx, me.TokenID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate current token: %w", err))
	}
	r, note := toRotateResult(res), ""
	if strings.HasPrefix(credentialSource(tokenFlag, prog, profile), "profile") {
		creds, rerr := readCredentialsFile(prog, profile)
		if rerr == nil && creds.Token == token {
			creds.Token, creds.TokenExpiresAt = res.Token, expiryString(res.ExpiresAt)
			if werr := writeCredentialsFile(prog, profile, creds); werr != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("the token was rotated but the credentials file could not be updated (the new token is printed below, save it): %w\n%s", werr, res.Token))
			}
			r.Token, r.CredentialsUpdated = "", true
			note = fmt.Sprintf("profile %q updated in the credentials file", profile)
		}
	}
	if note == "" {
		note = "the credential came from the flag or environment: update it with the new token below\nnew token (shown once): " + res.Token
	}
	return writeScheduledTaskResult(stdout, stderr, of, r, func() { printRotateResult(stdout, r, note) })
}

func tokensRotateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens rotate <id|name> [flags]

Mints a replacement with the same name, abilities and scope and a fresh
expiry, and prints its secret once. The old token keeps working for a grace
period (tokens.rotation_grace) and is then revoked. Platform tokens cannot be
rotated. Uses the token from --token, %[4]s or the saved profile when it may
rotate; otherwise signs in with --username/--password.

Flags:
  --token string             API token to rotate with
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiURLEnvHelp, defaultAPIURL, apiTokenEnvHelp)
}

func authRotateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth rotate [flags]

Rotates the token the current profile uses, then rewrites that profile in the
credentials file (atomically, mode 0600, with the new expiry). The old token
keeps working for a short grace period. The token needs the write ability to
rotate itself; a read-only token is rotated by its owner with "%[1]s tokens rotate".

Flags:
  --token string             API token (overrides %[4]s and the credentials file)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiURLEnvHelp, defaultAPIURL, apiTokenEnvHelp)
}

func decodeTokenList(raw json.RawMessage) ([]tokenResource, error) {
	var tokens []tokenResource
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return nil, fmt.Errorf("decode tokens: %w", err)
	}
	return tokens, nil
}

// expiryLabel is the EXPIRES column: the date, the days left, and a state word.
func expiryLabel(t tokenResource, now time.Time, soon time.Duration) string {
	if t.ExpiresAt == nil {
		return "never"
	}
	left := t.ExpiresAt.Sub(now)
	date := t.ExpiresAt.Local().Format("2006-01-02")
	switch {
	case left <= 0:
		return date + " expired"
	case left <= soon:
		return fmt.Sprintf("%s %dd left, expires soon", date, int(left.Hours()/24))
	}
	return fmt.Sprintf("%s %dd left", date, int(left.Hours()/24))
}
