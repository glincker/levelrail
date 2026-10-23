package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAuthWhoami implements "auth whoami": GET /api/v1/auth/whoami with the
// resolved token (flag, env, or credentials file).
func runAuthWhoami(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth whoami", "print the identity as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authWhoamiUsage(prog)) }
	accessFlag := fs.Bool("access", false, "also show what this identity can and cannot manage")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if *accessFlag {
		return runWhoamiAccess(client, stdout, stderr, jsonOut, of)
	}

	info, err := client.Whoami(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("whoami: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, info, func() {
		_, _ = fmt.Fprintf(stdout, "kind:        %s\n", info.Kind)
		_, _ = fmt.Fprintf(stdout, "name:        %s\n", info.Name)
		_, _ = fmt.Fprintf(stdout, "source:      %s\n", credentialSource(tokenFlag, prog, resolveProfile(profileFlag, lookupEnv)))
		_, _ = fmt.Fprintf(stdout, "abilities:   %s\n", strings.Join(info.Abilities, ", "))
		_, _ = fmt.Fprintf(stdout, "expires_at:  %s\n", info.ExpiresAt)
		if c, cerr := readCredentialsFile(prog, resolveProfile(profileFlag, lookupEnv)); cerr == nil && c.InstanceID != "" {
			_, _ = fmt.Fprintf(stdout, "server:      %s (%s), fingerprint %s, %s\n", c.ServerName, apiclient.ShortInstanceID(c.InstanceID), c.Fingerprint, c.APIURL)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func authWhoamiUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth whoami [flags]

Shows who the resolved API token (or session) authenticates as: its
kind, name, abilities, and expiry. A real live call, not a local check.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --access                  show tier, teams, attached policies and what you can and cannot manage
  --json                  print the identity as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
