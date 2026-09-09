package main

import (
	"context"
	"fmt"
	"io"
)

// runInvitesCreate implements "invites create": POST /api/v1/invites,
// AbilityRoot-gated server-side (internal/api/invites.go's own doc
// comment on handleCreateInvite). Exactly one of --role/--abilities must
// be set, the same mutually-exclusive precedence "users create" uses.
// The response always carries the accept link, even when no SMTP is
// configured server-side, so this command is a complete way to invite
// someone with no email capability at all.
func runInvitesCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "invites create", "print the created invite (including its accept link) as JSON to stdout and nothing else", stderr)
	var email, role, abilitiesFlag string
	fs.StringVar(&email, "email", "", "email to invite (required)")
	fs.StringVar(&role, "role", "", "curated role preset to apply: admin, operator, or viewer (see \""+prog+" users roles\"); alternative to --abilities")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list, e.g. \"read,deploy\" (valid: read, read:sensitive, write, write:sensitive, deploy, root); alternative to --role")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, invitesCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if email == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--email is required"))
	}
	if role == "" && abilitiesFlag == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("one of --role or --abilities is required"))
	}
	if role != "" && abilitiesFlag != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--role and --abilities are mutually exclusive"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateInvite(context.Background(), createInviteRequest{
		Email:     email,
		Role:      role,
		Abilities: splitAndTrim(abilitiesFlag),
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create invite for %q: %w", email, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "invite for %q created (id %s)\naccept link: %s\n", created.Email, created.ID, created.Link)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func invitesCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s invites create --email EMAIL --role ROLE [flags]
  %[1]s invites create --email EMAIL --abilities LIST [flags]

Invites a new teammate by email. Exactly one of --role/--abilities is
required. The response always includes the accept link, whether or not
this control plane has SMTP configured, so it can be copy/pasted by hand.

Flags:
  --email string             email to invite (required)
  --role string               curated role preset: admin, operator, or viewer (see "%[1]s users roles")
  --abilities string         comma-separated ability list (valid: read, read:sensitive, write, write:sensitive, deploy, root)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                       print the created invite as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
