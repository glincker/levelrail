package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runUsersDelete implements "users delete <id>": DELETE
// /api/v1/users/{id}. Refused (400) for the caller's own user and for
// the last remaining user, both enforced server-side
// (handleDeleteUser's own doc comment), surfaced here as-is.
func runUsersDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "users delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usersDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "users delete", "user id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteUser(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete user %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "user %q deleted\n", id)
	return exitOK
}

func usersDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users delete <id> [flags]

Removes a user. Refused (400) for the caller's own user and for the last
remaining user.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
