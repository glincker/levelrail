package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runTagsApps implements "tags apps <id>": GET /api/v1/tags/{id}/apps,
// the "list resources filtered by tag" primitive.
func runTagsApps(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "tags apps", "print apps as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tagsAppsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "tags apps", "tag id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	apps, err := client.ListAppsByTag(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps for tag %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, apps, func() { printTagAppsTable(stdout, apps) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printTagAppsTable(out io.Writer, apps []tagAppResource) {
	if len(apps) == 0 {
		_, _ = fmt.Fprintln(out, "no apps tagged")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME")
	for _, a := range apps {
		_, _ = fmt.Fprintln(tw, a.Name)
	}
	_ = tw.Flush()
}

func tagsAppsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tags apps <id> [flags]

Lists every app attached to a tag.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print apps as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
