package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runBackupsCloneRestores implements "backups clone-restores <database>".
func runBackupsCloneRestores(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runCloneRestoresList(prog, "backups clone-restores", "backups clone-restores <database>", "a database's restore-as-new attempt history", 1, args, stdout, stderr, lookupEnv,
		func(ctx context.Context, c *Client, rest []string) (any, func(io.Writer), error) {
			history, err := c.ListCloneRestores(ctx, rest[0])
			return history, func(out io.Writer) { printCloneRestoresTable(out, history) }, err
		})
}

// runAppVolumeBackupsCloneRestores implements "app-volume-backups clone-restores <app> <volume>".
func runAppVolumeBackupsCloneRestores(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runCloneRestoresList(prog, "app-volume-backups clone-restores", "app-volume-backups clone-restores <app> <volume>", "an app named volume's restore-as-new attempt history", 2, args, stdout, stderr, lookupEnv,
		func(ctx context.Context, c *Client, rest []string) (any, func(io.Writer), error) {
			history, err := c.ListVolumeCloneRestores(ctx, rest[0], rest[1])
			return history, func(out io.Writer) { printVolumeCloneRestoresTable(out, history) }, err
		})
}

func runCloneRestoresList(prog, label, synopsis, what string, nargs int, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool),
	fetch func(context.Context, *Client, []string) (any, func(io.Writer), error)) int {
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
	history, printTable, err := fetch(context.Background(), client, rest)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list clone restores: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, history, func() { printTable(stdout) })
}

func printCloneRestoresTable(out io.Writer, history []cloneRestoreResource) {
	if len(history) == 0 {
		_, _ = fmt.Fprintln(out, "no restore-as-new attempts")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNEW DATABASE\tBACKUP\tSTATUS\tSTARTED\tFINISHED\tERROR")
	for _, h := range history {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.NewDatabaseName, h.BackupHistoryID, h.Status, h.StartedAt, dashIfEmpty(h.FinishedAt), dashIfEmpty(h.Error))
	}
	_ = tw.Flush()
}

func printVolumeCloneRestoresTable(out io.Writer, history []volumeCloneRestoreResource) {
	if len(history) == 0 {
		_, _ = fmt.Fprintln(out, "no restore-as-new attempts")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNEW VOLUME\tBACKUP\tSTATUS\tSTARTED\tFINISHED\tERROR")
	for _, h := range history {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.NewVolumeName, h.BackupHistoryID, h.Status, h.StartedAt, dashIfEmpty(h.FinishedAt), dashIfEmpty(h.Error))
	}
	_ = tw.Flush()
}
