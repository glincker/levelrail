package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsDeploysReleaseNotes implements "apps deploys release-notes <name>
// --from ID [--to ID]": the commits between two deploys as markdown.
func runAppsDeploysReleaseNotes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys release-notes", "print the release notes as JSON to stdout and nothing else", stderr)
	var from, to string
	fs.StringVar(&from, "from", "", "deploy attempt ID to start after (required)")
	fs.StringVar(&to, "to", "", "deploy attempt ID to end at (default: newest deploy with a recorded commit)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysReleaseNotesUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys release-notes requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	if from == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--from is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	notes, err := client.ReleaseNotes(context.Background(), name, from, to)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("release notes for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, notes, func() { printReleaseNotesHuman(stdout, notes) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printReleaseNotesHuman(w io.Writer, n releaseNotesResource) {
	_, _ = fmt.Fprint(w, n.Markdown)
	if n.Note != "" {
		_, _ = fmt.Fprintf(w, "\n%s\n", n.Note)
	}
}

// printDeployCompareExtras prints the digest, env key and commit parts of a comparison.
func printDeployCompareExtras(w io.Writer, cmp deployCompareResource) {
	if cmp.From.ImageDigest != "" || cmp.To.ImageDigest != "" {
		_, _ = fmt.Fprintf(w, "\ndigest: %s -> %s\n", deployCompareValueLabel(cmp.From.ImageDigest), deployCompareValueLabel(cmp.To.ImageDigest))
	}
	if len(cmp.EnvChanges) > 0 {
		_, _ = fmt.Fprint(w, "\nenv keys changed:\n")
		for _, e := range cmp.EnvChanges {
			_, _ = fmt.Fprintf(w, "  %-8s %s\n", e.Status, e.Key)
		}
	}
	if len(cmp.Commits) > 0 {
		_, _ = fmt.Fprintf(w, "\ncommits (%d):\n", len(cmp.Commits))
		for _, c := range cmp.Commits {
			_, _ = fmt.Fprintf(w, "  %.7s %s\n", c.SHA, firstLine(c.Message))
		}
	} else if cmp.CommitsNote != "" {
		_, _ = fmt.Fprintf(w, "\ncommits: %s\n", cmp.CommitsNote)
	}
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func appsDeploysReleaseNotesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys release-notes <name> --from ID [--to ID] [flags]

Lists the commits after --from up to --to (default: the newest deploy with
a recorded commit), grouped by conventional-commit type, as markdown.
Needs a connected git source. Promote an image between environments with
"%[1]s apps promote <name> --to ENVIRONMENT_ID".

Flags:
  --from string       deploy attempt ID to start after (required)
  --to string         deploy attempt ID to end at
  --token string      API token
  --api-url string    control plane base URL
  --profile string    named credentials profile
  --json              print JSON to stdout, nothing else
  --output string     json, table, or text
  --query string      JMESPath expression to filter the result
  -h, --help          show this help
`, prog)
}
