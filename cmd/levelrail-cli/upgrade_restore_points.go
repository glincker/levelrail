package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// runUpdates implements "updates": the restore-points subcommand, and every
// other argument goes to "upgrade" so the two spellings stay interchangeable.
func runUpdates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "restore-points" {
		return runRestorePoints(prog, args[1:], stdout, stderr, lookupEnv)
	}
	return runUpgrade(prog, args, stdout, stderr, lookupEnv)
}

// runRestorePoints implements "updates restore-points list|restore <id>".
func runRestorePoints(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub, id := "list", ""
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		sub, args = args[0], args[1:]
	}
	if sub == "restore" && len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		id, args = args[0], args[1:]
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "updates restore-points", "print the result as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "with restore: confirm putting the restore point back")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s updates restore-points list\n  %s updates restore-points restore <id> --yes\n\nRestore points are taken automatically before each upgrade. Restoring puts back the binary and unit file,\nand the database only if it was migrated past the restore point; the result states how many minutes of\nchanges that discards. It refuses while an upgrade is running.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch sub {
	case "list":
		return listRestorePoints(ctx, client, jsonOut, of, stdout, stderr)
	case "restore":
		if id == "" {
			return reportError(stdout, stderr, jsonOut, errors.New("usage: updates restore-points restore <id> --yes"))
		}
		if !*yes {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("restoring %s replaces the running release; re-run with --yes to confirm", id))
		}
		started, err := client.RestoreRestorePoint(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore %s: %w", id, err))
		}
		return renderUpgradeResult(stdout, stderr, of, started, func() {
			_, _ = fmt.Fprintf(stdout, "restoring %s (%s) on the host; follow it with: %s upgrade --attempts\n", started.RestorePoint, started.Version, prog)
		})
	}
	return reportError(stdout, stderr, jsonOut, fmt.Errorf("unknown restore-points command %q (use list or restore)", sub))
}

func listRestorePoints(ctx context.Context, client *Client, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	list, err := client.ListRestorePoints(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore points: %w", err))
	}
	return renderUpgradeResult(stdout, stderr, of, list, func() {
		if len(list.RestorePoints) == 0 {
			_, _ = fmt.Fprintln(stdout, "no restore points yet: one is taken automatically before each upgrade")
		}
		for _, p := range list.RestorePoints {
			note := "binary only (database is compatible)"
			if p.DatabaseRestoreNeeded {
				note = "restores the database too, discarding changes since " + p.CreatedAt
			}
			if !p.CanRestore {
				note = "cannot restore: " + p.CannotRestoreWhy
			}
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s  %d MiB  %s\n", p.ID, p.Version, p.CreatedAt, p.SizeBytes>>20, note)
		}
	})
}
