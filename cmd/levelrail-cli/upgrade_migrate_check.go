package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runUpgradeMigrateCheck asks the control plane to try its pending migrations
// on a copy of the database. It exits non-zero when they fail.
func runUpgradeMigrateCheck(ctx context.Context, client *Client, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	res, err := client.RunMigrationDryRun(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("migration dry run: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printMigrationDryRun(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !res.OK {
		return exitCheckFailed
	}
	return exitOK
}

func printMigrationDryRun(out io.Writer, r apiclient.MigrationDryRun) {
	verdict := "ok"
	if !r.OK {
		verdict = "FAILED"
	}
	_, _ = fmt.Fprintf(out, "migration dry run on a copy of the database: %s (schema %d -> %d, %d ms)\n", verdict, r.FromSchema, r.ToSchema, r.DurationMS)
	for _, m := range r.Applied {
		_, _ = fmt.Fprintf(out, "  applied %04d %s\n", m.Version, m.Name)
	}
	if len(r.Applied) == 0 && r.OK {
		_, _ = fmt.Fprintln(out, "  nothing pending: the database already has every migration of this binary")
	}
	if r.Error != "" {
		_, _ = fmt.Fprintf(out, "  error: %s\n", r.Error)
	}
	_, _ = fmt.Fprintln(out, "the live database was not touched")
}
