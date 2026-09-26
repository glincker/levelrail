package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// printDiagnosisCauses prints each typed cause with its numbered fixes, the
// numbers "apps diagnose --apply-fix" takes.
func printDiagnosisCauses(out io.Writer, d diagnosisResource) {
	if len(d.Causes) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\ncauses:")
	for _, c := range d.Causes {
		_, _ = fmt.Fprintf(out, "  %s (%s): %s\n", c.Code, c.Confidence, c.Title)
		for _, f := range c.Fixes {
			_, _ = fmt.Fprintf(out, "    fix %d [%s]: %s\n", f.N, f.Kind, f.Label)
			for _, ch := range f.Changes {
				if ch.NeedsInput {
					_, _ = fmt.Fprintf(out, "      %s: needs a value (--input %s=VALUE)\n", ch.Field, ch.Field)
					continue
				}
				_, _ = fmt.Fprintf(out, "      %s: %s -> %s\n", ch.Field, ch.From, ch.To)
			}
			if f.Hint != "" {
				_, _ = fmt.Fprintf(out, "      %s\n", f.Hint)
			}
		}
	}
}

// printRecentChanges prints the "what changed" section: newest first, the
// likely cause tagged.
func printRecentChanges(out io.Writer, r *apiclient.RecentChangesResource, indent string) {
	if r == nil {
		return
	}
	window := (time.Duration(r.WindowSeconds) * time.Second).String()
	if len(r.Changes) == 0 {
		_, _ = fmt.Fprintf(out, "%sno changes in the last %s\n", indent, window)
		return
	}
	_, _ = fmt.Fprintf(out, "%schanged in the last %s:\n", indent, window)
	for _, c := range r.Changes {
		line := c.At.Local().Format("15:04:05") + " " + c.Title
		if len(c.Keys) > 0 {
			line += " [" + strings.Join(c.Keys, ", ") + "]"
		}
		if c.Detail != "" {
			line += " (" + c.Detail + ")"
		}
		if c.Actor != "" {
			line += " by " + c.Actor
		}
		if c.LikelyCause {
			line += "  <- likely cause"
		}
		_, _ = fmt.Fprintf(out, "%s  %s\n", indent, line)
	}
	if more := r.Total - len(r.Changes); more > 0 {
		_, _ = fmt.Fprintf(out, "%s  and %d more\n", indent, more)
	}
}
