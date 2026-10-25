package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const (
	envAdminPassword = "APP_ADMIN_PASSWORD" //nolint:gosec // env var name, not a credential
	envSetupToken    = "APP_SETUP_TOKEN"    //nolint:gosec // env var name, not a credential
)

// runSetupAdmin creates the first admin through POST /auth/register using the
// token the host prints with "setup-token". Secrets come from env or a
// prompt, never from flag values.
func runSetupAdmin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" setup admin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	username := fs.String("username", "admin", "username for the first admin")
	apiURL := fs.String("api-url", "", "control plane base URL (default: "+envAPIURL+" env var, then "+defaultAPIURL+")")
	profile := fs.String("profile", "", "credentials profile (overrides "+envProfile+")")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s setup admin [--username U] [--api-url URL] [--profile NAME]\n\nCreates the first admin on a fresh server. Run \"setup-token\" on the host to get the token, then set %s and %s (or answer the prompts).\n\nFlags:\n", prog, envSetupToken, envAdminPassword)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	token := firstEnv(lookupEnv, envSetupToken)
	password := firstEnv(lookupEnv, envAdminPassword)
	if token == "" || password == "" {
		if !stdinIsTerminal() {
			return reportError(stdout, stderr, false, newValidationError("set %s and %s, or run in a terminal to be prompted", envSetupToken, envAdminPassword))
		}
		var err error
		if token == "" {
			if token, err = promptSecret(stderr, "Setup token (from \"setup-token\" on the server): "); err != nil {
				return reportError(stdout, stderr, false, err)
			}
		}
		if password == "" {
			if password, err = promptSecret(stderr, "New admin password: "); err != nil {
				return reportError(stdout, stderr, false, err)
			}
		}
	}
	base := resolveAPIURL(*apiURL, lookupEnv, prog, resolveProfile(*profile, lookupEnv))
	client, err := newAuthSessionClient(base)
	if err != nil {
		return reportError(stdout, stderr, false, err)
	}
	body := map[string]string{"username": *username, "password": password, "setup_token": token}
	if err := client.do(context.Background(), "POST", "/api/v1/auth/register", body, nil); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("create first admin (it is refused once an admin exists): %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "admin %q created at %s; sign in with: %s login --api-url %s --device\n", *username, base, prog, base)
	return exitOK
}

func firstEnv(lookupEnv func(string) (string, bool), key string) string {
	v, _ := lookupEnv(key)
	return v
}

func promptSecret(stderr io.Writer, label string) (string, error) {
	_, _ = fmt.Fprint(stderr, label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(stderr)
	if err != nil || len(b) == 0 {
		return "", newValidationError("a value is required")
	}
	return string(b), nil
}
