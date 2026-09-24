package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// restoresListUsage is the shared usage text for the read-only restore history
// subcommands, which all take the same flags.
func restoresListUsage(prog, synopsis, what string) string {
	return fmt.Sprintf(`Usage:
  %[1]s %[5]s [flags]

Lists %[6]s.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the history as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, synopsis, what)
}

// runBackupsRestores implements "backups restores <database>".
func runBackupsRestores(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runRestoresList(prog, "backups restores", "backups restores <database>", "a database's restore attempt history", 1, args, stdout, stderr, lookupEnv,
		func(ctx context.Context, c *Client, rest []string) ([]restoreHistoryResource, error) {
			return c.ListRestores(ctx, rest[0])
		})
}

// runAppVolumeBackupsRestores implements "app-volume-backups restores <app> <volume>".
func runAppVolumeBackupsRestores(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runRestoresList(prog, "app-volume-backups restores", "app-volume-backups restores <app> <volume>", "an app named volume's restore attempt history", 2, args, stdout, stderr, lookupEnv,
		func(ctx context.Context, c *Client, rest []string) ([]restoreHistoryResource, error) {
			return c.ListVolumeRestores(ctx, rest[0], rest[1])
		})
}

func runRestoresList(prog, label, synopsis, what string, nargs int, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool),
	fetch func(context.Context, *Client, []string) ([]restoreHistoryResource, error)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the history as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, restoresListUsage(prog, synopsis, what)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, label, fmt.Sprintf("%d argument(s), see usage", nargs), nargs)
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	history, err := fetch(context.Background(), client, rest)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list restores: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, history, func() { printRestoreHistoryTable(stdout, history) })
}

func printRestoreHistoryTable(out io.Writer, history []restoreHistoryResource) {
	if len(history) == 0 {
		_, _ = fmt.Fprintln(out, "no restores")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tBACKUP\tSTATUS\tSTARTED\tFINISHED\tERROR")
	for _, h := range history {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.BackupHistoryID, h.Status, h.StartedAt, dashIfEmpty(h.FinishedAt), dashIfEmpty(h.Error))
	}
	_ = tw.Flush()
}

// runPITRRestores implements "pitr restores <database>".
func runPITRRestores(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr restores", "print the history as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, restoresListUsage(prog, "pitr restores <database>", "a database's point-in-time restore attempt history"))
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "pitr restores", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	history, err := client.ListPITRRestores(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list pitr restores for database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, history, func() {
		if len(history) == 0 {
			_, _ = fmt.Fprintln(stdout, "no point-in-time restores")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tBASE BACKUP\tTARGET TIME\tSTATUS\tSTARTED\tERROR")
		for _, h := range history {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.BaseBackupHistoryID, h.TargetTimestamp, h.Status, h.StartedAt, dashIfEmpty(h.Error))
		}
		_ = tw.Flush()
	})
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
