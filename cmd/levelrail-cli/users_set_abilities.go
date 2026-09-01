package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runUsersSetAbilities implements "users set-abilities <id>": PUT
// /api/v1/users/{id}/abilities. Same --role/--abilities exclusivity as
// "users create" (users_create.go's own doc comment); the server also
// refuses id equal to the caller's own user (the self-lockout guard,
// handleUpdateUserAbilities's own doc comment), surfaced here as
// whatever error message the API returns, not duplicated client-side.
func runUsersSetAbilities(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "users set-abilities", "print the updated user as JSON to stdout and nothing else", stderr)
	var abilitiesFlag, role string
	fs.StringVar(&role, "role", "", "curated role preset to apply: admin, operator, or viewer (see \""+prog+" users roles\"); alternative to --abilities")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list, e.g. \"read,deploy\"; alternative to --role")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usersSetAbilitiesUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "users set-abilities", "user id")
	if !ok {
		return exitUsage
	}
	if role == "" && abilitiesFlag == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("one of --role or --abilities is required"))
	}
	if role != "" && abilitiesFlag != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--role and --abilities are mutually exclusive"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updated, err := client.UpdateUserAbilities(context.Background(), id, updateUserAbilitiesRequest{
		Role:      role,
		Abilities: splitAndTrim(abilitiesFlag),
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set abilities for user %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "user %q abilities updated: %v\n", updated.Email, updated.Abilities)
	return exitOK
}

func usersSetAbilitiesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users set-abilities <id> --role ROLE [flags]
  %[1]s users set-abilities <id> --abilities LIST [flags]

Replaces a user's abilities wholesale. Exactly one of --role/--abilities
is required. Refused (400) if <id> is the caller's own user.

Flags:
  --role string               curated role preset: admin, operator, or viewer (see "%[1]s users roles")
  --abilities string         comma-separated ability list (valid: read, read:sensitive, write, write:sensitive, deploy, root)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                       print the updated user as JSON to stdout, nothing else
  -h, --help                 show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
