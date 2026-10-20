package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runInvitesList implements "invites list": GET /api/v1/invites, every
// invite not yet accepted or revoked.
func runInvitesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "invites list", "print invites as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, invitesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	invites, err := client.ListInvites(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list invites: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, invites, func() { printInvitesTable(stdout, invites) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printInvitesTable(out io.Writer, invites []inviteResource) {
	if len(invites) == 0 {
		_, _ = fmt.Fprintln(out, "no pending invites")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tEMAIL\tROLE\tCREATED\tEXPIRES\tSTATUS")
	for _, inv := range invites {
		role := inv.Role
		if role == "" {
			role = "custom"
		}
		status := "pending"
		if inv.Expired {
			status = "expired"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", inv.ID, inv.Email, role, inv.CreatedAt.Format("2006-01-02"), inv.ExpiresAt.Format("2006-01-02"), status)
	}
	_ = tw.Flush()
}

func invitesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s invites list [flags]

Lists every pending (not yet accepted or revoked) invite.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print invites as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
