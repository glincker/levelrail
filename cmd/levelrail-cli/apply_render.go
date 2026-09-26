package main

import (
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func actionMark(a iac.Action) string {
	switch a {
	case iac.ActionCreate:
		return "+"
	case iac.ActionUpdate:
		return "~"
	case iac.ActionDelete:
		return "-"
	case iac.ActionError:
		return "!"
	}
	return " "
}

func fieldLine(fc iac.FieldChange) string {
	switch fc.Op {
	case iac.OpAdd:
		return fmt.Sprintf("+ %s: %s", fc.Path, fc.New)
	case iac.OpRemove:
		return fmt.Sprintf("- %s: %s", fc.Path, fc.Old)
	case iac.OpKeep:
		return fmt.Sprintf("= %s (kept, not in the files)", fc.Path)
	}
	return fmt.Sprintf("~ %s: %s -> %s", fc.Path, fc.Old, fc.New)
}

func printPlan(w io.Writer, p iac.Plan) {
	s := p.Summary
	_, _ = fmt.Fprintf(w, "Plan: %d to create, %d to update, %d to delete, %d unchanged", s.Create, s.Update, s.Delete, s.Noop)
	if s.Error > 0 {
		_, _ = fmt.Fprintf(w, ", %d with errors", s.Error)
	}
	_, _ = fmt.Fprint(w, "\n\n")
	for _, c := range p.Changes {
		if c.Action == iac.ActionNoop {
			continue
		}
		loc := ""
		if c.File != "" {
			loc = fmt.Sprintf("  (%s:%d)", c.File, c.Line)
		}
		_, _ = fmt.Fprintf(w, "%s %s%s\n", actionMark(c.Action), c.Key(), loc)
		if c.Reason != "" {
			_, _ = fmt.Fprintf(w, "    %s\n", c.Reason)
		}
		for _, fc := range c.Fields {
			_, _ = fmt.Fprintf(w, "    %s\n", fieldLine(fc))
		}
		for _, fc := range c.Kept {
			_, _ = fmt.Fprintf(w, "    %s\n", fieldLine(fc))
		}
		for _, warn := range c.Warnings {
			_, _ = fmt.Fprintf(w, "    warning: %s\n", warn)
		}
	}
}

func printApplyResult(w io.Writer, r iac.ApplyResult) {
	for _, it := range r.Results {
		if it.Status == iac.StatusNoop {
			continue
		}
		line := fmt.Sprintf("%-8s %s", it.Status, it.Key())
		if it.Error != "" {
			line += ": " + it.Error
		}
		_, _ = fmt.Fprintln(w, line)
	}
	_, _ = fmt.Fprintf(w, "\n%d applied, %d failed, %d skipped\n", r.Applied, r.Failed, r.Skipped)
}
