package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type inboxScopeFlags struct {
	archived, resolved, read bool
	olderThan                time.Duration
}

func (f *inboxScopeFlags) bind(fs *flag.FlagSet, withArchived bool) {
	if withArchived {
		fs.BoolVar(&f.archived, "archived", false, "act on every archived item")
	}
	fs.BoolVar(&f.resolved, "resolved", false, "act on every resolved alert")
	fs.BoolVar(&f.read, "read", false, "act on every read item")
	fs.DurationVar(&f.olderThan, "older-than", 0, "only items older than this, for example 72h")
}

func (f *inboxScopeFlags) scope() (string, error) {
	var scopes []string
	for _, s := range []struct {
		on   bool
		name string
	}{{f.archived, "archived"}, {f.resolved, "resolved"}, {f.read, "read"}} {
		if s.on {
			scopes = append(scopes, s.name)
		}
	}
	if len(scopes) > 1 {
		return "", newValidationError("give only one of --archived, --resolved, --read")
	}
	if f.olderThan < 0 {
		return "", newValidationError("--older-than must not be negative")
	}
	if len(scopes) == 0 {
		return "", nil
	}
	return scopes[0], nil
}

func inboxBulkResult(verb string, n int) (any, func(io.Writer)) {
	return map[string]int{verb: n}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "%s %d item(s)\n", verb, n) }
}

func inboxBulkCommand(label, action string, withArchived bool) apiCmd {
	var (
		f   inboxScopeFlags
		yes bool
	)
	return apiCmd{
		label: label,
		usage: inboxUsage,
		setup: func(fs *flag.FlagSet) {
			f.bind(fs, withArchived)
			fs.BoolVar(&yes, "yes", false, "confirm a destructive action")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			scope, err := f.scope()
			if err != nil {
				return nil, nil, err
			}
			if scope == "" {
				return nil, nil, newValidationError("give a scope: --resolved, --read or --archived (delete and clear only)")
			}
			req := apiclient.InboxClearRequest{Action: action, Scope: scope, OlderThanSeconds: int64(f.olderThan / time.Second), DryRun: true}
			preview, err := c.ClearInbox(ctx, req)
			if err != nil {
				return nil, nil, fmt.Errorf("preview %s: %w", label, err)
			}
			if action == "delete" {
				if !yes {
					return nil, nil, newValidationError("this would delete %d item(s); re-run with --yes to confirm", preview.Count)
				}
				req.ConfirmCount = preview.Count
			}
			req.DryRun = false
			res, err := c.ClearInbox(ctx, req)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", label, err)
			}
			verb := "archived"
			if action == "delete" {
				verb = "deleted"
			}
			out, text := inboxBulkResult(verb, res.Count)
			return out, text, nil
		},
	}
}

func inboxDeleteCommand() apiCmd {
	var (
		ids     stringList
		restore bool
		yes     bool
	)
	return apiCmd{
		label: "inbox delete",
		usage: inboxUsage,
		setup: func(fs *flag.FlagSet) {
			fs.Var(&ids, "id", "stored alert item ID to delete (repeatable)")
			fs.BoolVar(&restore, "restore", false, "undo a delete")
			fs.BoolVar(&yes, "yes", false, "confirm the delete")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if len(ids) == 0 {
				return nil, nil, newValidationError("give --id ID (repeatable); use \"inbox clear\" to delete by scope")
			}
			if !restore && !yes {
				return nil, nil, newValidationError("this would delete %d item(s); re-run with --yes to confirm", len(ids))
			}
			err := c.DeleteInbox(ctx, ids, restore)
			msg := "deleted"
			if restore {
				msg = "restored"
			}
			return map[string]bool{"ok": err == nil}, func(w io.Writer) { _, _ = fmt.Fprintln(w, msg) }, err
		},
	}
}
