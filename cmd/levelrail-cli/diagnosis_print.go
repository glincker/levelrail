package main

import (
	"fmt"
	"io"
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
