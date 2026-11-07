package main

import (
	"context"
	"fmt"
	"io"
)

// runTagsRename implements "tags rename <old> <new>": PATCH /api/v1/tags/{id}.
func runTagsRename(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "tags rename", "print the renamed tag as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tagsRenameUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if fs.NArg() != 2 {
		_, _ = fmt.Fprint(stderr, tagsRenameUsage(prog))
		return exitUsage
	}
	oldName, newName := fs.Arg(0), fs.Arg(1)

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	id, err := resolveTagIDByName(context.Background(), client, oldName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rename tag %q: %w", oldName, err))
	}

	renamed, err := client.RenameTag(context.Background(), id, newName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rename tag %q: %w", oldName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, renamed, func() {
		_, _ = fmt.Fprintf(stdout, "tag %q renamed to %q\n", oldName, renamed.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func tagsRenameUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tags rename <old> <new> [flags]

Renames a tag by name. Every attachment is kept. Fails if <new> is taken.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the renamed tag as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
