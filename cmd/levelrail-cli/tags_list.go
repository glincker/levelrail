package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runTagsList implements "tags list": GET /api/v1/tags.
func runTagsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "tags list", "print tags as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tagsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	tags, err := client.ListTags(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tags: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, tags, func() { printTagsTable(stdout, tags) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printTagsTable(out io.Writer, tags []tagResource) {
	if len(tags) == 0 {
		_, _ = fmt.Fprintln(out, "no tags")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCREATED")
	for _, t := range tags {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", t.ID, t.Name, t.CreatedAt)
	}
	_ = tw.Flush()
}

func tagsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tags list [flags]

Lists every tag.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print tags as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
