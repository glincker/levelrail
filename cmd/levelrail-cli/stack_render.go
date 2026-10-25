package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
)

func newStackTable(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0) }

func stackDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// stackTime shortens an RFC 3339 time and leaves anything else as it is.
func stackTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return stackDash(s)
	}
	return t.UTC().Format("2006-01-02 15:04")
}

func stackShortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func printStackList(w io.Writer, list []apiclient.StackInfo) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(w, "no stacks")
		return
	}
	tw := newStackTable(w)
	_, _ = fmt.Fprintln(tw, "NAME\tPROJECT\tREVISION\tSTATUS\tPINNED\tLOCKED")
	for _, s := range list {
		pin, lock := "-", "-"
		if s.PinnedRevision > 0 {
			pin = fmt.Sprint(s.PinnedRevision)
		}
		if s.Lock != nil {
			lock = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", s.Name, stackDash(s.Project), s.LatestRevision, stackDash(s.LatestStatus), pin, lock)
	}
	_ = tw.Flush()
}

func printStackSettings(w io.Writer, s apiclient.StackInfo) {
	_, _ = fmt.Fprintf(w, "%s: allow-prune=%t drift=%s require-approval=%t\n", s.Name, s.AllowPrune, stackDash(s.DriftPolicy), s.RequireApproval)
}

func printStack(w io.Writer, s apiclient.StackInfo) {
	_, _ = fmt.Fprintf(w, "Stack     %s\nProject   %s\nCreated   %s by %s\n", s.Name, stackDash(s.Project), stackTime(s.CreatedAt), stackDash(s.CreatedBy))
	status := ""
	if s.LatestStatus != "" {
		status = " (" + s.LatestStatus + ")"
	}
	_, _ = fmt.Fprintf(w, "Revision  %d%s\n", s.LatestRevision, status)
	if s.PinnedRevision > 0 {
		_, _ = fmt.Fprintf(w, "Pinned    revision %d\n", s.PinnedRevision)
	}
	if s.Lock != nil {
		_, _ = fmt.Fprintf(w, "Locked    by %s for revision %d until %s\n", s.Lock.Holder, s.Lock.Revision, stackTime(s.Lock.ExpiresAt))
	}
	_, _ = fmt.Fprintf(w, "Settings  allow-prune=%t drift=%s require-approval=%t\n", s.AllowPrune, stackDash(s.DriftPolicy), s.RequireApproval)
	if len(s.Resources) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "\nResources")
	tw := newStackTable(w)
	_, _ = fmt.Fprintln(tw, "  KIND\tKEY\tSINCE")
	for _, r := range s.Resources {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%d\n", r.Kind, r.Key, r.SinceRevision)
	}
	_ = tw.Flush()
}

func planSummaryLine(s iac.Summary) string {
	line := fmt.Sprintf("%d to create, %d to update, %d to delete, %d unchanged", s.Create, s.Update, s.Delete, s.Noop)
	if s.Error > 0 {
		line += fmt.Sprintf(", %d with errors", s.Error)
	}
	return line
}

func printIssues(w io.Writer, issues []iac.Issue) {
	for _, is := range issues {
		_, _ = fmt.Fprintln(w, "issue: "+is.String())
	}
}

func printStackPlan(w io.Writer, r apiclient.StackPlanResponse) {
	state := "new, created on first apply"
	if r.StackExists {
		state = "exists"
	}
	_, _ = fmt.Fprintf(w, "Stack %s (%s)\n", r.Stack, state)
	if r.PinnedRevision > 0 {
		_, _ = fmt.Fprintf(w, "pinned to revision %d: applies are refused until it is removed\n", r.PinnedRevision)
	}
	_, _ = fmt.Fprintf(w, "Plan: %s\n", planSummaryLine(r.Plan.Summary))
	printIssues(w, r.Issues)
	tw := newStackTable(w)
	rows := 0
	for _, c := range r.Plan.Changes {
		if c.Action == iac.ActionNoop {
			continue
		}
		if rows == 0 {
			_, _ = fmt.Fprintln(tw, "\t\tRESOURCE\tCHANGE")
		}
		rows++
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", actionMark(c.Action), c.Action, c.Key(), changeDetail(c))
	}
	_ = tw.Flush()
	for _, c := range r.Plan.Changes {
		if c.Action == iac.ActionNoop || len(c.Fields) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "\n%s %s\n", actionMark(c.Action), c.Key())
		for _, fc := range c.Fields {
			_, _ = fmt.Fprintf(w, "    %s\n", fieldLine(fc))
		}
	}
	for _, wn := range r.Warnings {
		_, _ = fmt.Fprintln(w, "warning: "+wn)
	}
	if r.Plan.Hash != "" {
		_, _ = fmt.Fprintf(w, "\nPlan hash: %s\n", r.Plan.Hash)
	}
}

// changeDetail is the short last column of a plan row: the reason, or the
// first changed paths.
func changeDetail(c iac.Change) string {
	if c.Reason != "" {
		return c.Reason
	}
	paths := make([]string, 0, 3)
	for i, f := range c.Fields {
		if i == 3 {
			paths = append(paths, fmt.Sprintf("+%d more", len(c.Fields)-3))
			break
		}
		paths = append(paths, f.Path)
	}
	return strings.Join(paths, ", ")
}

func printStackRun(w io.Writer, r apiclient.StackApplyResponse, verb string) {
	printIssues(w, r.Issues)
	if r.Revision == nil {
		_, _ = fmt.Fprintf(w, "stack %s: no revision was created\n", r.Stack)
		return
	}
	if r.Replayed {
		_, _ = fmt.Fprintf(w, "Stack %s: this request was already handled, showing revision %d (%s)\n", r.Stack, r.Revision.Number, r.Revision.Status)
	} else {
		_, _ = fmt.Fprintf(w, "Stack %s: revision %d %s (%s)\n", r.Stack, r.Revision.Number, verb, r.Revision.Status)
	}
	if r.Revision.RollbackOf > 0 {
		_, _ = fmt.Fprintf(w, "restores revision %d\n", r.Revision.RollbackOf)
	}
	if r.PendingApproval {
		_, _ = fmt.Fprintf(w, "awaiting approval until %s; approve with: stack approve %s %d\n", r.ExpiresAt, r.Stack, r.Revision.Number)
	}
	if r.Result != nil {
		_, _ = fmt.Fprintln(w)
		printApplyResult(w, *r.Result)
	}
	for _, wn := range r.Warnings {
		_, _ = fmt.Fprintln(w, "warning: "+wn)
	}
}

func printStackHistory(w io.Writer, h apiclient.StackHistory) {
	if len(h.Revisions) == 0 {
		_, _ = fmt.Fprintf(w, "stack %s has no revisions\n", h.Stack)
		return
	}
	tw := newStackTable(w)
	_, _ = fmt.Fprintln(tw, "REV\tSTATUS\tACTOR\tWHEN\tCOMMIT\tNOTE")
	for _, r := range h.Revisions {
		note := ""
		if r.RollbackOf > 0 {
			note = fmt.Sprintf("rollback of %d", r.RollbackOf)
		}
		if r.Status == string(iac.RevisionPendingApproval) {
			note = strings.TrimSpace(note + " awaiting approval")
		}
		if r.Number == h.PinnedRevision {
			note = strings.TrimSpace(note + " pinned")
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", r.Number, r.Status, stackDash(r.Actor), stackTime(r.CreatedAt), stackDash(stackShortSHA(r.GitSHA)), stackDash(note))
	}
	_ = tw.Flush()
}

func printStackRevision(w io.Writer, stack string, d apiclient.StackRevisionDetail) {
	_, _ = fmt.Fprintf(w, "Stack     %s\nRevision  %d (%s)\nActor     %s\nStarted   %s\nFinished  %s\n", stack, d.Number, d.Status, stackDash(d.Actor), stackTime(d.CreatedAt), stackTime(d.FinishedAt))
	if d.RollbackOf > 0 {
		_, _ = fmt.Fprintf(w, "Restores  revision %d\n", d.RollbackOf)
	}
	if d.GitRepo != "" || d.GitSHA != "" {
		_, _ = fmt.Fprintf(w, "Git       %s %s %s\n", stackDash(d.GitRepo), stackDash(d.GitRef), stackDash(stackShortSHA(d.GitSHA)))
	}
	if d.CIRunURL != "" {
		_, _ = fmt.Fprintf(w, "CI run    %s\n", d.CIRunURL)
	}
	_, _ = fmt.Fprintf(w, "Plan hash %s\n", stackDash(d.PlanHash))
	if len(d.Files) > 0 {
		_, _ = fmt.Fprintln(w, "\nDocuments")
		for _, f := range d.Files {
			_, _ = fmt.Fprintf(w, "  %s\n", f.Name)
		}
	}
	if d.Plan != nil {
		_, _ = fmt.Fprintf(w, "\nPlan: %s\n", planSummaryLine(d.Plan.Summary))
	}
	if d.Result != nil {
		_, _ = fmt.Fprintln(w)
		printApplyResult(w, *d.Result)
	}
}

func printStackDiff(w io.Writer, d apiclient.StackDiff) {
	_, _ = fmt.Fprintf(w, "Stack %s: revision %d against %s\n", d.Stack, d.Revision, stackDash(d.Against))
	if len(d.Changes) == 0 {
		_, _ = fmt.Fprintln(w, "no differences")
		return
	}
	for _, c := range d.Changes {
		key := c.Kind + "/" + c.Name
		if c.Scope != "" {
			key = c.Kind + "/" + c.Scope + "/" + c.Name
		}
		_, _ = fmt.Fprintf(w, "%s %s\n", actionMark(iac.Action(c.Action)), key)
		for _, f := range c.Fields {
			_, _ = fmt.Fprintf(w, "    %s\n", fieldLine(iac.FieldChange{Path: f.Path, Op: f.Op, Old: f.Old, New: f.New}))
		}
	}
}

func printStackDelete(w io.Writer, r apiclient.StackDeleteResult) {
	_, _ = fmt.Fprintf(w, "Stack %s deleted (%s)\n", r.Stack, r.Mode)
	for _, s := range r.Destroyed {
		_, _ = fmt.Fprintln(w, "  destroyed "+s)
	}
	for _, s := range r.Forgotten {
		_, _ = fmt.Fprintln(w, "  forgotten "+s)
	}
	for _, s := range r.Failed {
		_, _ = fmt.Fprintln(w, "  FAILED    "+s)
	}
}
