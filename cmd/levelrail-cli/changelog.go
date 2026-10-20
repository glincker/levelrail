package main

import (
	"context"
	"fmt"
	"io"
)

// runChangelog implements "changelog": GET /api/v1/changelog, the same
// endpoint the dashboard's "What's new" panel reads from, printing
// recent release notes from the control plane's own CHANGELOG.md.
func runChangelog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "changelog", "print changelog entries as JSON to stdout and nothing else", stderr)
	var limitFlag int
	fs.IntVar(&limitFlag, "limit", 0, "max entries to return (default: server default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, changelogUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	entries, err := client.GetChangelog(context.Background(), limitFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get changelog: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, entries, func() { printChangelogHuman(stdout, entries) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func changelogUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s changelog [flags]

Prints recent release notes from the running control plane's own
CHANGELOG.md, newest first.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --limit int              max entries to return (default: server default)
  --json                    print changelog entries as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
