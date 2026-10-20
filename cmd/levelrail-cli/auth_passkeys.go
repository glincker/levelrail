package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// passkeyResource mirrors internal/api's passkeyResource.
type passkeyResource struct {
	ID           string     `json:"id"`
	Label        string     `json:"label"`
	Transports   []string   `json:"transports,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ProviderKey  string     `json:"provider_key"`
	ProviderName string     `json:"provider_name,omitempty"`
	Synced       *bool      `json:"synced,omitempty"`
	BackedUp     *bool      `json:"backed_up,omitempty"`
	SignCount    uint32     `json:"sign_count"`
}

// ListPasskeys calls GET /api/v1/auth/passkeys (session only, like every auth route).
func (c *authSessionClient) ListPasskeys(ctx context.Context) ([]passkeyResource, error) {
	var out []passkeyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/passkeys", nil, &out)
	return out, err
}

// RenamePasskey calls PATCH /api/v1/auth/passkeys/{id}.
func (c *authSessionClient) RenamePasskey(ctx context.Context, id, label string) (passkeyResource, error) {
	var out passkeyResource
	err := c.do(ctx, http.MethodPatch, "/api/v1/auth/passkeys/"+pathEscape(id), map[string]string{"label": label}, &out)
	return out, err
}

// RemovePasskey calls DELETE /api/v1/auth/passkeys/{id}.
func (c *authSessionClient) RemovePasskey(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/passkeys/"+pathEscape(id), nil, nil)
}

func runAuthPasskeys(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authPasskeysUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authPasskeysUsage(prog))
		return exitOK
	case "list", "rename", "remove":
		return runAuthPasskeysVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv, stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth passkeys subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authPasskeysUsage(prog))
		return exitUsage
	}
}

func runAuthPasskeysVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth passkeys "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var label string
	if verb == "rename" {
		fs.StringVar(&label, "name", "", "the new passkey name (1 to 64 characters)")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authPasskeysUsage(prog)) }
	jsonOut, of, exitCode, ok := parseSessionFlags(fs, args, jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}
	id, vErr := passkeyIDArg(fs, verb, label)
	if vErr != nil {
		return reportError(stdout, stderr, jsonOut, vErr)
	}

	ctx := context.Background()
	client, exitCode, ok := buildSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stdout, stderr, jsonOut)
	if !ok {
		return exitCode
	}
	switch verb {
	case "list":
		rows, err := client.ListPasskeys(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list passkeys: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, rows, func() { printPasskeys(stdout, rows) })
	case "rename":
		row, err := client.RenamePasskey(ctx, id, strings.TrimSpace(label))
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("rename passkey: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, row, func() { _, _ = fmt.Fprintf(stdout, "renamed to %q\n", row.Label) })
	default:
		if err := client.RemovePasskey(ctx, id); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove passkey: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "removed", "id": id}, func() { _, _ = fmt.Fprintln(stdout, "passkey removed") })
	}
}

func passkeyIDArg(fs *flag.FlagSet, verb, label string) (string, error) {
	if verb == "list" {
		return "", nil
	}
	if fs.NArg() != 1 {
		return "", newValidationError("exactly one passkey id is required (see: auth passkeys list)")
	}
	if verb == "rename" && strings.TrimSpace(label) == "" {
		return "", newValidationError("--name is required")
	}
	return fs.Arg(0), nil
}

func printPasskeys(w io.Writer, rows []passkeyResource) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(w, "no passkeys registered")
		return
	}
	for _, p := range rows {
		kind := "unknown"
		if p.Synced != nil {
			kind = "device-bound"
			if *p.Synced {
				kind = "synced"
			}
		}
		provider := p.ProviderName
		if provider == "" {
			provider = "Passkey"
		}
		used := "never used"
		if p.LastUsedAt != nil {
			used = "last used " + p.LastUsedAt.Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(w, "%s  %s  (%s, %s, %d sign-ins, %s)\n", p.ID, p.Label, provider, kind, p.SignCount, used)
	}
}

func authPasskeysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth passkeys list [flags]                         list this account's passkeys
  %[1]s auth passkeys rename --name NAME ID [flags]    rename a passkey
  %[1]s auth passkeys remove ID [flags]                   remove a passkey (refused when it is your only way to sign in)

Every subcommand needs a live session: --username/--password (prompted if
omitted). Adding a passkey needs a browser, so do that from the dashboard.

Flags:
  --name string           the new passkey name (rename only)
  --username string       admin username (prompted if omitted)
  --password string       admin password (prompted without echo if omitted)
  --api-url string        control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the result as JSON to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIURL, defaultAPIURL)
}
