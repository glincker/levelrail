package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsRecentlyDeleted lists deleted apps and databases still inside the undo window, or restores one.
func runAppsRecentlyDeleted(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		w, code := stderr, exitUsage
		if len(args) > 0 {
			w, code = stdout, exitOK
		}
		_, _ = fmt.Fprint(w, appsRecentlyDeletedUsage(prog))
		return code
	}
	switch args[0] {
	case "list":
		return runAppsRecentlyDeletedList(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runAppsRecentlyDeletedRestore(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps recently-deleted subcommand %q\n\n%s", prog, args[0], appsRecentlyDeletedUsage(prog))
	return exitUsage
}

func appsRecentlyDeletedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps recently-deleted list [flags]                      deleted apps and databases that can still be restored
  %[1]s apps recently-deleted restore <app|database> <name>     bring one back from its saved config

The undo window is the runtime setting retention.recently_deleted_days.
Secret values come back. The deploy source, deploy token, log drain, integrations and build hooks do not; restore prints the command to recreate each.
`, prog)
}

func runAppsRecentlyDeletedList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps recently-deleted list", "print the entries as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rows, err := client.ListRecentlyDeleted(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list recently deleted: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "nothing to restore")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tNAME\tDELETED\tRESTORABLE UNTIL")
		for _, r := range rows {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Kind, r.Name, r.DeletedAt.Format("2006-01-02 15:04"), r.ExpiresAt.Format("2006-01-02 15:04"))
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsRecentlyDeletedRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps recently-deleted restore", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos, ok := requireArgs(fs, stderr, prog, "apps recently-deleted restore", "<app|database> <name>", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.RestoreRecentlyDeletedResult(context.Background(), pos[0], pos[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore %s %q: %w", pos[0], pos[1], err))
	}
	_, _ = fmt.Fprintf(stdout, "restored %s %q\n", pos[0], pos[1])
	printRestoreAttention(stdout, res)
	return exitOK
}

// runAppsRestoreDeleted is the short form: apps restore <name>.
func runAppsRestoreDeleted(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps restore", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos, ok := requireArgs(fs, stderr, prog, "apps restore", "<name>", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.RestoreRecentlyDeletedResult(context.Background(), "app", pos[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore app %q: %w", pos[0], err))
	}
	_, _ = fmt.Fprintf(stdout, "restored %q, stopped; start it when ready\n", pos[0])
	printRestoreAttention(stdout, res)
	return exitOK
}

// printRestoreAttention lists what the restore could not bring back, with the command to recreate each.
func printRestoreAttention(w io.Writer, res apiclient.RestoreResult) {
	for _, a := range res.NeedsAttention {
		_, _ = fmt.Fprintf(w, "not restored: %s, recreate with: %s\n", strings.ReplaceAll(a.Category, "_", " "), a.Fix)
	}
}
