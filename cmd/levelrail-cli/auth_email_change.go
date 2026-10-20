package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"
)

type emailChangeRequestBody struct {
	NewEmail string `json:"new_email"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"`
}

// RequestEmailChange calls POST /api/v1/auth/email-change with an inline password proof.
func (c *authSessionClient) RequestEmailChange(ctx context.Context, newEmail, password, code string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/auth/email-change", emailChangeRequestBody{NewEmail: newEmail, Password: password, Code: code}, nil)
}

// ConfirmEmailChange calls POST /api/v1/auth/email-change/confirm and returns the new address.
func (c *authSessionClient) ConfirmEmailChange(ctx context.Context, token string) (string, error) {
	var out struct {
		Email string `json:"email"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/email-change/confirm", map[string]string{"token": token}, &out)
	return out.Email, err
}

// runAuthEmailChange implements "auth email-change request|confirm".
func runAuthEmailChange(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authEmailChangeUsage(prog))
		return exitUsage
	}
	verb := args[0]
	switch args[0] {
	case "request", "confirm":
	default:
		_, _ = fmt.Fprint(stderr, authEmailChangeUsage(prog))
		return exitUsage
	}
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth email-change "+verb, "print a JSON result object to stdout and nothing else", stderr)
	newEmail := fs.String("new-email", "", "the address to switch to (request)")
	code := fs.String("code", "", "current authenticator code, when two-factor authentication is on (request)")
	token := fs.String("token", "", "the token from the confirmation link after #confirm_email= (confirm)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authEmailChangeUsage(prog)) }
	jsonOut, of, exitCode, ok := parseSessionFlags(fs, args[1:], jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}
	if verb == "request" && strings.TrimSpace(*newEmail) == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-email ADDRESS is required"))
	}
	if verb == "confirm" && strings.TrimSpace(*token) == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--token TOKEN is required"))
	}
	username, password, err := resolveLoginCredentials(*usernameP, *passwordP, stdin, stderr, func() (string, error) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(b), err
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	ctx := context.Background()
	client, exitCode, ok := buildSessionClient(ctx, sessionFlags{username, password, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stdout, stderr, jsonOut)
	if !ok {
		return exitCode
	}
	if verb == "request" {
		if err := client.RequestEmailChange(ctx, strings.TrimSpace(*newEmail), password, *code); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("email change request: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "requested"}, func() {
			_, _ = fmt.Fprintln(stdout, "if the address is free, a confirmation link was sent to it")
		})
	}
	email, err := client.ConfirmEmailChange(ctx, strings.TrimSpace(*token))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("email change confirm: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "changed", "email": email}, func() {
		_, _ = fmt.Fprintf(stdout, "email changed to %s\n", email)
	})
}

func authEmailChangeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth email-change request --new-email ADDRESS [flags]
  %[1]s auth email-change confirm --token TOKEN [flags]

request mails a confirmation link to the new address (at most 3 per hour). The
answer is the same whether or not the address is taken. confirm finishes the
change with the token from that link, the part after #confirm_email=.

Flags:
  --new-email string      the address to switch to (request)
  --token string          the token from the confirmation link (confirm)
  --code string           current authenticator code, when two-factor is on (request)
  --username string       account email (prompted if omitted)
  --password string       account password (prompted without echo if omitted)
  --api-url string        control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print a JSON result object to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIURL, defaultAPIURL)
}
