package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runUpgradeHistory(ctx context.Context, client *Client, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	h, err := client.GetUpgradeHistory(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade history: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, h, func() { printUpgradeHistory(stdout, h) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runUpgradeAck(ctx context.Context, client *Client, id string, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	item, err := client.AckUpgrade(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("acknowledge upgrade %s: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, item, func() {
		_, _ = fmt.Fprintf(stdout, "acknowledged %s -> %s (%s)\n", versionOrNone(item.FromVersion), item.ToVersion, item.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func versionOrNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func printUpgradeHistory(out io.Writer, h upgradeHistory) {
	_, _ = fmt.Fprintf(out, "running %s, %d unacknowledged\n", h.CurrentVersion, h.Unacknowledged)
	if len(h.Entries) == 0 {
		_, _ = fmt.Fprintln(out, "no upgrades recorded yet: the first one will appear here")
		return
	}
	for _, e := range h.Entries {
		var flags []string
		if !e.Acknowledged {
			flags = append(flags, "UNACKNOWLEDGED")
		}
		if e.SchemaMoved {
			flags = append(flags, "schema changed")
		}
		_, _ = fmt.Fprintf(out, "%s  %-11s %s -> %s  by %s  %s  %s\n", e.OccurredAt, e.Kind, versionOrNone(e.FromVersion), e.ToVersion, e.Initiator, e.ID, strings.Join(flags, ", "))
		if e.ReleaseURL != "" {
			_, _ = fmt.Fprintf(out, "    %s\n", e.ReleaseURL)
		}
	}
	for _, c := range h.AgentChanges {
		_, _ = fmt.Fprintf(out, "%s  agent %-16s %s -> %s\n", c.ObservedAt, c.NodeName, versionOrNone(c.FromVersion), c.ToVersion)
	}
}
