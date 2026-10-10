package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runUpgradeList(ctx context.Context, client *Client, channel string, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	h, err := client.GetReleaseHistory(ctx, channel)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("release history: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, h, func() { printReleaseList(stdout, h) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printReleaseList(out io.Writer, h releaseHistory) {
	schema := "unknown"
	if h.CurrentSchemaVersion != nil {
		schema = fmt.Sprint(*h.CurrentSchemaVersion)
	}
	_, _ = fmt.Fprintf(out, "running %s (database schema %s), showing %s releases\n", h.CurrentVersion, schema, h.View)
	if !h.Reachable {
		_, _ = fmt.Fprintln(out, "GitHub could not be reached: showing cached or retained releases only.")
	}
	if len(h.Releases)+len(h.RetainedOnly) == 0 {
		_, _ = fmt.Fprintln(out, "no releases found for this channel")
		return
	}
	for _, r := range append(append([]releaseHistoryItem{}, h.Releases...), h.RetainedOnly...) {
		var flags []string
		if r.Running {
			flags = append(flags, "running")
		}
		if r.Retained {
			flags = append(flags, "retained")
		}
		if !r.AssetAvailable {
			flags = append(flags, "no binary for this platform")
		}
		_, _ = fmt.Fprintf(out, "%-22s %-6s %-16s schema %-8s %s\n", r.Version, r.Channel, r.Verdict, schemaLabel(r.SchemaVersion), strings.Join(flags, ", "))
	}
}

func schemaLabel(v *int) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprint(*v)
}

func runUpgradeRollbackPlan(ctx context.Context, client *Client, version string, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	p, err := client.GetRollbackPlan(ctx, version)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rollback plan: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, p, func() { printRollbackPlan(stdout, p) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if p.Blocked {
		return exitCheckFailed
	}
	return exitOK
}

func printRollbackPlan(out io.Writer, p rollbackPlan) {
	_, _ = fmt.Fprintf(out, "rollback %s -> %s (%s)\n", p.CurrentVersion, p.Target.Version, p.Verdict)
	_, _ = fmt.Fprintf(out, "database schema %d, target supports %s\n", p.CurrentSchemaVersion, schemaFromInt(p.Target.SchemaVersion))
	_, _ = fmt.Fprintf(out, "steps: %s\nestimated downtime: about %ds\n", strings.Join(p.Steps, ", "), p.DowntimeSeconds)
	for _, c := range p.Checks {
		_, _ = fmt.Fprintf(out, "[%s] %s: %s\n", c.Status, c.Name, c.Message)
	}
	if p.RestoreRequired {
		_, _ = fmt.Fprintln(out, "\nWARNING: the database is newer than this release supports. A binary-only rollback is refused.")
		_, _ = fmt.Fprintln(out, "Restoring a backup works but permanently loses everything written after it:")
		for _, b := range p.Backups {
			_, _ = fmt.Fprintf(out, "  %s  taken %s  schema %d  compatible=%v\n", b.Name, b.CreatedAt, b.SchemaVersion, b.Compatible)
		}
		if p.RestoreCommand != "" {
			_, _ = fmt.Fprintf(out, "\nRestore and roll back (on the host):\n  %s\n", p.RestoreCommand)
		}
	}
	if p.FetchCommand != "" {
		_, _ = fmt.Fprintf(out, "\nFirst stage the verified binary on the host:\n  %s\n", p.FetchCommand)
	}
	if !p.RestoreRequired {
		_, _ = fmt.Fprintf(out, "\nApply on the host:\n  %s\n", p.Command)
	}
}

func schemaFromInt(v int) string {
	if v < 0 {
		return "unknown"
	}
	return fmt.Sprint(v)
}
