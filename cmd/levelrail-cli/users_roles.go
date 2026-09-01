package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runUsersRoles implements "users roles": GET /api/v1/roles, the curated
// presets --role on "users create"/"users set-abilities" accepts.
func runUsersRoles(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "users roles", "print roles as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usersRolesUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	roles, err := client.ListRoles(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list roles: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, roles); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printRolesTable(stdout, roles)
	return exitOK
}

func printRolesTable(out io.Writer, roles []roleResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tABILITIES\tDESCRIPTION")
	for _, r := range roles {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\n", r.Name, r.Abilities, r.Description)
	}
	_ = tw.Flush()
}

func usersRolesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users roles [flags]

Lists the curated role presets: --role on "%[1]s users create" and
"%[1]s users set-abilities" accepts one of these names, applying its
ability set in one action.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print roles as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
