package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"golang.org/x/term"
)

const (
	exportPollEvery   = time.Second
	exportPollTimeout = 90 * time.Second
)

const accountFlagsUsage = `  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`

// runAuthActivity implements "auth activity": the caller's own activity feed.
func runAuthActivity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth activity", "print the feed as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authActivityUsage(prog)) }
	limit := fs.Int("limit", 25, "how many rows to show, 1 to 100")
	kind := fs.String("kind", "", "only one category: sign_in, security, sessions, tokens, token_use, account or failed")
	cursor := fs.String("cursor", "", "continue after the next_cursor of a previous page")
	all := fs.Bool("all", false, "include raw API requests, not just security events")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *all && *kind == "" {
		*kind = "all"
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	feed, err := client.MyActivity(context.Background(), apiclient.MyActivityOptions{Limit: *limit, Kind: *kind, Cursor: *cursor})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("activity: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, feed, func() {
		for _, e := range feed.Entries {
			_, _ = fmt.Fprintf(stdout, "%s  %-10s %-34s %d  %s\n", e.At.Format(time.RFC3339), e.Kind, activityLabel(e), e.StatusCode, e.RemoteAddr)
		}
		if feed.NextCursor != "" {
			_, _ = fmt.Fprintf(stdout, "more: --cursor %s\n", feed.NextCursor)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func activityLabel(e apiclient.MyActivityEntry) string {
	if e.Event != "" {
		return e.Event
	}
	return e.Method + " " + e.Path
}

func authActivityUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth activity [--kind KIND] [--all] [--limit N] [--cursor C] [flags]

Shows what your own account did: sign-ins, token use, session changes and
passkey or two-factor changes. Only your own rows are ever returned.

Flags:
  --kind string           sign_in, security, sessions, tokens, token_use, account or failed
  --limit int             rows to show, 1 to 100 (default 25)
  --cursor string         continue after the next_cursor of a previous page
`+accountFlagsUsage, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAuthPreferences implements "auth preferences get|set".
func runAuthPreferences(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, authPreferencesUsage(prog))
		return exitUsage
	}
	verb := args[0]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth preferences "+verb, "print the preferences as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authPreferencesUsage(prog)) }
	timezone := fs.String("timezone", "", "IANA zone such as Europe/Berlin, empty for the browser's own")
	language := fs.String("language", "", "locale code such as en or pt-BR, empty for the browser's own")
	theme := fs.String("theme", "", "light, dark or system, empty for the default")
	landing := fs.String("landing-page", "", "dashboard path to open after sign-in, such as /apps")
	gravatar := fs.Bool("gravatar", false, "use your Gravatar as avatar (off by default, it shares a hash of your email)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	prefs, err := client.GetPreferences(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preferences: %w", err))
	}
	if verb == "set" {
		changed := false
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "timezone":
				prefs.Timezone, changed = *timezone, true
			case "language":
				prefs.Language, changed = *language, true
			case "theme":
				prefs.Theme, changed = *theme, true
			case "landing-page":
				prefs.LandingPage, changed = *landing, true
			case "gravatar":
				prefs.Gravatar, changed = *gravatar, true
			}
		})
		if !changed {
			return reportError(stdout, stderr, jsonOut, newValidationError("pass at least one of --timezone, --language, --theme, --landing-page, --gravatar"))
		}
		if prefs, err = client.PutPreferences(ctx, prefs); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set preferences: %w", err))
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, prefs, func() {
		_, _ = fmt.Fprintf(stdout, "timezone:      %s\nlanguage:      %s\ntheme:         %s\nlanding_page:  %s\ngravatar:      %t\n", prefs.Timezone, prefs.Language, prefs.Theme, prefs.LandingPage, prefs.Gravatar)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func authPreferencesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth preferences get [flags]
  %[1]s auth preferences set [--timezone Z] [--language L] [--theme T] [--landing-page P] [--gravatar] [flags]

Your own timezone, language, theme, default landing page and avatar choice.
An empty value means "use the default".

Flags:
  --timezone string       IANA zone such as Europe/Berlin
  --language string       locale code such as en or pt-BR
  --theme string          light, dark or system
  --landing-page string   dashboard path to open after sign-in, such as /apps
  --gravatar              use your Gravatar as avatar (off by default)
`+accountFlagsUsage, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAuthExport implements "auth export": request, wait for and save the zip.
func runAuthExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth export", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authExportUsage(prog)) }
	out := fs.String("out", prog+"-account-export.zip", "file to write the export to")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, cancel := context.WithTimeout(context.Background(), exportPollTimeout)
	defer cancel()
	job, err := client.StartAccountExport(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export: %w", err))
	}
	for job.Status == "pending" {
		select {
		case <-ctx.Done():
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("export: timed out waiting for the bundle"))
		case <-time.After(exportPollEvery):
		}
		if job, err = client.AccountExportStatus(ctx, job.ID); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("export status: %w", err))
		}
	}
	if job.Status != "ready" {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export: the bundle could not be built, try again"))
	}
	data, err := client.DownloadAccountExport(ctx, job.ID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export download: %w", err))
	}
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", *out, err))
	}
	result := map[string]any{"file": *out, "size_bytes": len(data)}
	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", *out, len(data))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func authExportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth export [--out FILE] [flags]

Bundles your own profile, sessions, tokens (never their secrets), linked
providers and activity into a zip and saves it. The file is written 0600.

Flags:
  --out string            file to write (default %[1]s-account-export.zip)
`+accountFlagsUsage, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAuthUnlink implements "auth unlink <provider>".
func runAuthUnlink(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth unlink", "print a JSON result object to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authUnlinkUsage(prog)) }
	client, provider, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog: prog, cmdLabel: "auth unlink", argLabel: "provider"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := client.UnlinkProvider(context.Background(), provider); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("unlink %s: %w", provider, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "unlinked", "provider": provider}, func() {
		_, _ = fmt.Fprintf(stdout, "unlinked %s\n", provider)
	})
}

func authUnlinkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth unlink <provider> [flags]

Removes a linked sign-in provider from your account. Refused when it is
your only way to sign in: set a password or add a passkey first.

Flags:
`+accountFlagsUsage, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAuthDeleteAccount implements "auth delete-account": permanent, so it
// needs the typed email and the account password.
func runAuthDeleteAccount(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth delete-account", "print a JSON result object to stdout and nothing else", stderr)
	confirm := fs.String("confirm", "", "your account email, typed to confirm")
	code := fs.String("code", "", "current authenticator code, when two-factor authentication is on")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authDeleteAccountUsage(prog)) }
	jsonOut, of, exitCode, ok := parseSessionFlags(fs, args, jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}
	if strings.TrimSpace(*confirm) == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--confirm EMAIL is required: this deletes your account for good"))
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
	if err := client.DeleteAccount(ctx, *confirm, password, *code); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete account: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "deleted"}, func() {
		_, _ = fmt.Fprintln(stdout, "account deleted")
	})
}

func authDeleteAccountUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth delete-account --confirm EMAIL [flags]

Permanently deletes your own account. Refused while you are the last admin
or the only owner of a team that still has members or projects: hand those
over first (the dashboard lists exactly what blocks and where to fix it).

Flags:
  --confirm string        your account email, typed to confirm
  --code string           current authenticator code, when two-factor is on
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
