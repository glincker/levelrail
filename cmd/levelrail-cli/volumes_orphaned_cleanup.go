package main

import (
	"context"
	"fmt"
	"io"
)

// runVolumesOrphanedCleanup implements "volumes-orphaned-cleanup": POST
// /api/v1/system/volumes/orphaned/cleanup
// (internal/api/volumes_orphaned.go). --names is required and takes a
// comma-separated list of exact volume names, the CLI's own equivalent
// of the confirm-before-delete flow the dashboard's own dialog gives an
// operator: there is no "--all" shorthand that deletes every currently
// orphaned volume sight unseen, an operator must name what "%[1]s
// volumes-orphaned" showed them was actually safe to remove. The control
// plane still re-confirms each name is genuinely orphaned right now
// before removing it (the request can be run well after the list was
// reviewed), reporting anything that no longer qualifies back as
// skipped rather than failing the whole call.
func runVolumesOrphanedCleanup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "volumes-orphaned-cleanup", "print the cleanup result as JSON to stdout and nothing else", stderr)
	namesFlag := fs.String("names", "", "comma-separated list of exact orphaned volume names to remove (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, volumesOrphanedCleanupUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	names := splitAndTrim(*namesFlag)
	if len(names) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--names is required: pass a comma-separated list of volume names from %q", prog+" volumes-orphaned"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.CleanupOrphanedVolumes(context.Background(), names)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cleanup orphaned volumes: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printCleanupOrphanedVolumesHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printCleanupOrphanedVolumesHuman(out io.Writer, r cleanupOrphanedVolumesResult) {
	_, _ = fmt.Fprintf(out, "removed:   %d (%d bytes reclaimed)\n", len(r.Removed), r.ReclaimedBytes)
	for _, name := range r.Skipped {
		_, _ = fmt.Fprintf(out, "skipped:   %s (no longer orphaned)\n", name)
	}
	for _, e := range r.Errors {
		_, _ = fmt.Fprintf(out, "error: %s\n", e)
	}
}

func volumesOrphanedCleanupUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s volumes-orphaned-cleanup --names name1,name2 [flags]

Removes exactly the named Docker volumes, after the control plane
re-confirms each one is still genuinely orphaned. Run "%[1]s
volumes-orphaned" first to see what's actually safe to remove; there is
no flag that deletes every currently orphaned volume sight unseen.
Requires an admin/root-scoped token, the same as "%[1]s system-prune".

Flags:
  --names string           comma-separated list of exact volume names to remove (required)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the cleanup result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
