package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
)

// inboxEntry is one row of the unified inbox: an alert, an approval request or a failure.
type inboxEntry struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	At       time.Time `json:"at,omitzero"`
	Read     bool      `json:"read"`
}

const (
	inboxKindAlert    = "alert"
	inboxKindApproval = "approval"
	inboxKindDeploy   = "deploy"
	inboxKindUpgrade  = "upgrade"
)

func inboxUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s inbox list [--unread] [--kind alert|approval|deploy|upgrade] [--archived] [--limit N]
  %[1]s inbox read (--id ID)... | --all
  %[1]s inbox archive (--id ID)... [--restore]

The inbox gathers alerts, approval requests (sign-in and deploy), failed
deploys and failed database upgrades. Read and archive state is yours alone.
Approve or retry from the dashboard inbox page or with the commands the
title points to.
`, prog)
}

func runInbox(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return runAPICmd(prog, inboxFeedListCommand(), args[1:], stdout, stderr, lookupEnv)
		case "read":
			return runAPICmd(prog, inboxReadCommand(), args[1:], stdout, stderr, lookupEnv)
		case "archive":
			return runAPICmd(prog, inboxArchiveCommand(), args[1:], stdout, stderr, lookupEnv)
		}
	}
	return dispatchSub(prog, "inbox", inboxUsage, map[string]apiCmd{}, args, stdout, stderr, lookupEnv)
}

func inboxFeedListCommand() apiCmd {
	var (
		unread, archived bool
		kind             string
		limit            int
	)
	return apiCmd{
		label: "inbox list",
		usage: inboxUsage,
		setup: func(fs *flag.FlagSet) {
			fs.BoolVar(&unread, "unread", false, "only unread items")
			fs.BoolVar(&archived, "archived", false, "show archived items instead")
			fs.StringVar(&kind, "kind", "", "alert, approval, deploy or upgrade")
			fs.IntVar(&limit, "limit", 50, "maximum rows")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if kind != "" && kind != inboxKindAlert && kind != inboxKindApproval && kind != inboxKindDeploy && kind != inboxKindUpgrade {
				return nil, nil, newValidationError("--kind must be alert, approval, deploy or upgrade")
			}
			state, err := c.GetInboxState(ctx)
			if err != nil {
				return nil, nil, err
			}
			gone := map[string]bool{}
			for _, id := range state.ArchivedIDs {
				gone[id] = true
			}
			seen := map[string]bool{}
			for _, id := range state.ReadIDs {
				seen[id] = true
			}
			var rows []inboxEntry
			if alerts, aerr := c.ListInbox(ctx, false, 200); aerr == nil {
				for _, a := range alerts {
					rows = append(rows, inboxEntry{ID: a.ID, Kind: inboxKindAlert, Severity: a.Severity, Title: a.RuleName + " " + a.Event, At: a.At, Read: a.Read})
				}
			}
			if failed, ferr := c.ListFailedDeploys(ctx, "24h"); ferr == nil {
				for _, d := range failed {
					at := d.StartedAt
					if d.FinishedAt != nil {
						at = *d.FinishedAt
					}
					rows = append(rows, inboxEntry{ID: "deploy:" + d.ID, Kind: inboxKindDeploy, Severity: attention.Critical, Title: "Deploy of " + d.ServiceName + " failed", At: at, Read: seen["deploy:"+d.ID]})
				}
			}
			if pending, perr := c.ListDeployApprovals(ctx, "pending", ""); perr == nil {
				for _, a := range pending {
					rows = append(rows, inboxEntry{ID: "approval:" + a.ID, Kind: inboxKindApproval, Severity: attention.Warning, Title: a.ServiceName + " awaits approval", Read: seen["approval:"+a.ID]})
				}
			}
			if reqs, serr := c.ListSignInRequests(ctx); serr == nil {
				for _, a := range reqs.Approvals {
					rows = append(rows, inboxEntry{ID: "signin:" + a.ID, Kind: inboxKindApproval, Severity: attention.Warning, Title: "New browser sign-in from " + a.RequesterIP, At: a.CreatedAt, Read: seen["signin:"+a.ID]})
				}
			}
			if feed, ferr := c.GetAttentionFeed(ctx); ferr == nil {
				for _, f := range feed {
					k := ""
					switch f.Kind {
					case attention.KindDBUpgradeFailed:
						k = inboxKindUpgrade
					}
					if k != "" {
						rows = append(rows, inboxEntry{ID: f.ID, Kind: k, Severity: f.Severity, Title: f.Title, Read: seen[f.ID]})
					}
				}
			}
			out := rows[:0]
			for _, r := range rows {
				if gone[r.ID] != archived || (unread && r.Read) || (kind != "" && r.Kind != kind) {
					continue
				}
				out = append(out, r)
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return out, func(w io.Writer) { printInboxEntries(w, out) }, nil
		},
	}
}

func printInboxEntries(out io.Writer, rows []inboxEntry) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "inbox is empty")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tSEVERITY\tTIME\tTITLE\t")
	for _, r := range rows {
		mark, when := "*", "-"
		if r.Read {
			mark = ""
		}
		if !r.At.IsZero() {
			when = r.At.Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Kind, orDash(r.Severity), when, r.Title, mark)
	}
	_ = tw.Flush()
}

func inboxArchiveCommand() apiCmd {
	var ids stringList
	var restore bool
	return apiCmd{
		label: "inbox archive",
		usage: inboxUsage,
		setup: func(fs *flag.FlagSet) {
			fs.Var(&ids, "id", "inbox item ID to archive (repeatable)")
			fs.BoolVar(&restore, "restore", false, "move the items back to the inbox")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if len(ids) == 0 {
				return nil, nil, newValidationError("give --id ID (repeatable)")
			}
			err := c.ArchiveInbox(ctx, ids, restore)
			msg := "archived"
			if restore {
				msg = "restored"
			}
			return map[string]bool{"ok": err == nil}, func(w io.Writer) { _, _ = fmt.Fprintln(w, msg) }, err
		},
	}
}
