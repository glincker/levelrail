package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runUsersList implements "users list": GET /api/v1/users.
func runUsersList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "users list", "print users as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usersListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	users, err := client.ListUsers(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list users: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, users); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printUsersTable(stdout, users)
	return exitOK
}

func printUsersTable(out io.Writer, users []userResource) {
	if len(users) == 0 {
		_, _ = fmt.Fprintln(out, "no users")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tEMAIL\tROLE\tABILITIES\tCREATED")
	for _, u := range users {
		role := u.Role
		if role == "" {
			role = "custom"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\n", u.ID, u.Email, role, u.Abilities, u.CreatedAt.Format("2006-01-02"))
	}
	_ = tw.Flush()
}

func usersListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users list [flags]

Lists every user with access to this control plane.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print users as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
