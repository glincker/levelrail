package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDatabasesBranchDiff prints the schema differences of a branch against its parent or another database of the tree.
func runDatabasesBranchDiff(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	const cmd = "databases branch diff"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the result as JSON to stdout and nothing else", stderr)
	var against string
	var sqlOnly bool
	fs.StringVar(&against, "against", "", "database to compare with: the parent (default) or another database of the same tree")
	fs.BoolVar(&sqlOnly, "sql", false, "print only the generated migration SQL (review before running)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesBranchUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs 2 argument(s)\n\n", prog, cmd)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	diff, err := client.GetDatabaseBranchSchemaDiff(context.Background(), rest[0], rest[1], against)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", cmd, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, diff, func() { printSchemaDiff(stdout, diff, sqlOnly) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSchemaDiff(w io.Writer, d apiclient.DatabaseBranchSchemaDiff, sqlOnly bool) {
	if sqlOnly {
		// Newlines are kept; every other control character is masked per line.
		for _, line := range strings.Split(d.SQL, "\n") {
			_, _ = fmt.Fprintln(w, safeTerminalText(line))
		}
		return
	}
	_, _ = fmt.Fprintf(w, "%s against %s: %d added, %d removed, %d changed\n", safeTerminalText(d.Branch), safeTerminalText(d.Against), d.Summary.Added, d.Summary.Removed, d.Summary.Changed)
	if d.Truncated {
		_, _ = fmt.Fprintln(w, "note: the object cap cut the read short, raise databases.schema_diff_max_objects")
	}
	if len(d.Changes) == 0 {
		_, _ = fmt.Fprintln(w, "No schema differences")
		return
	}
	marks := map[string]string{"added": "+", "removed": "-", "changed": "~"}
	for _, c := range d.Changes {
		name := safeTerminalText(c.Schema + "." + c.Name)
		if c.Table != "" {
			name = safeTerminalText(c.Schema + "." + c.Table + "." + c.Name)
		}
		_, _ = fmt.Fprintf(w, "%s %s %s\n", marks[c.Kind], c.Object, name)
	}
	_, _ = fmt.Fprintln(w, "\nUse --sql for a migration to review before running.")
}
