package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runUsersCreate implements "users create": POST /api/v1/auth/users,
// AbilityRoot-gated server-side (internal/api/users.go's own doc
// comment on handleCreateUser). Exactly one of --role/--abilities must
// be set, mirroring createUserRequest's own Role-takes-precedence
// resolution (internal/api/roles.go's resolveAbilities): --role is the
// convenience path for the curated presets ("%[1]s users roles" lists
// them), --abilities is the raw, hand-picked list "tokens create" itself
// already uses.
func runUsersCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "users create", "print the created user as JSON to stdout and nothing else", stderr)
	var email, password, displayName, abilitiesFlag, role string
	fs.StringVar(&email, "email", "", "email for the new user (required)")
	fs.StringVar(&password, "password", "", "password for the new user, at least 8 characters (required)")
	fs.StringVar(&displayName, "display-name", "", "display name (defaults to the email)")
	fs.StringVar(&role, "role", "", "curated role preset to apply: admin, operator, or viewer (see \""+prog+" users roles\"); alternative to --abilities")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list, e.g. \"read,deploy\" (valid: read, read:sensitive, write, write:sensitive, deploy, root); alternative to --role")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usersCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	if email == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--email is required"))
	}
	if password == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--password is required"))
	}
	if role == "" && abilitiesFlag == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("one of --role or --abilities is required"))
	}
	if role != "" && abilitiesFlag != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--role and --abilities are mutually exclusive"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	created, err := client.CreateUser(context.Background(), createUserRequest{
		Email:       email,
		DisplayName: displayName,
		Password:    password,
		Role:        role,
		Abilities:   splitAndTrim(abilitiesFlag),
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create user %q: %w", email, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "user %q (id %s) created\n", created.Email, created.ID)
	return exitOK
}

func usersCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users create --email EMAIL --password PASSWORD --role ROLE [flags]
  %[1]s users create --email EMAIL --password PASSWORD --abilities LIST [flags]

Creates a local-password user. Exactly one of --role/--abilities is
required.

Flags:
  --email string             email for the new user (required)
  --password string          password, at least 8 characters (required)
  --display-name string      display name (defaults to the email)
  --role string               curated role preset: admin, operator, or viewer (see "%[1]s users roles")
  --abilities string         comma-separated ability list (valid: read, read:sensitive, write, write:sensitive, deploy, root)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --json                       print the created user as JSON to stdout, nothing else
  -h, --help                 show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
