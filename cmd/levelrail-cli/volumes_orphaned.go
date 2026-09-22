package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runVolumesOrphaned implements "volumes-orphaned": GET
// /api/v1/system/volumes/orphaned (internal/api/volumes_orphaned.go),
// every named Docker volume this instance created that no current app,
// database, or storage attachment references any more. Detection only;
// use "volumes-orphaned-cleanup" to actually remove one, the same
// list-then-confirm split "system-prune" doesn't need (that command has
// no selectable candidates, this one does).
func runVolumesOrphaned(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "volumes-orphaned", "print orphaned volumes as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, volumesOrphanedUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	volumes, err := client.ListOrphanedVolumes(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list orphaned volumes: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, volumes, func() { printOrphanedVolumesHuman(stdout, volumes) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printOrphanedVolumesHuman(out io.Writer, volumes []orphanedVolumeResource) {
	if len(volumes) == 0 {
		_, _ = fmt.Fprintln(out, "no orphaned volumes")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSIZE_BYTES\tCREATED_AT")
	for _, v := range volumes {
		size := "unknown"
		if v.SizeBytes != nil {
			size = fmt.Sprintf("%d", *v.SizeBytes)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", v.Name, size, v.CreatedAt)
	}
	_ = tw.Flush()
}

func volumesOrphanedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s volumes-orphaned [flags]

Lists every named Docker volume this control plane created (an app's
storage attachment, a database's data volume) that no current app,
database, or storage attachment references any more: the app or
database was deleted, and Docker never removes a named volume on its
own. Detection only, nothing is deleted; use "%[1]s volumes-orphaned-cleanup"
to remove one after reviewing this list.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print orphaned volumes as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
