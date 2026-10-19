package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func alertsInboxUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts inbox list [--unread] [--limit N]
  %[1]s alerts inbox count
  %[1]s alerts inbox read (--id ID)... | --all
  %[1]s alerts inbox prefs [--min-severity S] [--mute APP]... [--unmute APP]... [--digest on|off] [--digest-hour H] [--digest-channel ID]

The inbox is the in-app list of alerts that reached you. By default it
shows warning and critical firings and resolutions and hides silenced,
grouped and rate limited noise. "prefs" with no flags prints the current
routing rules; the daily digest is sent at the chosen UTC hour through a
notification channel.
`, prog)
}

func runAlertsInbox(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return runAPICmd(prog, inboxListCommand(), args[1:], stdout, stderr, lookupEnv)
		case "count":
			return runAPICmd(prog, inboxCountCommand(), args[1:], stdout, stderr, lookupEnv)
		case "read":
			return runAPICmd(prog, inboxReadCommand(), args[1:], stdout, stderr, lookupEnv)
		case "prefs":
			return runAPICmd(prog, inboxPrefsCommand(), args[1:], stdout, stderr, lookupEnv)
		}
	}
	return dispatchSub(prog, "alerts inbox", alertsInboxUsage, map[string]apiCmd{}, args, stdout, stderr, lookupEnv)
}

func inboxListCommand() apiCmd {
	var unread bool
	var limit int
	return apiCmd{
		label: "alerts inbox list",
		usage: alertsInboxUsage,
		setup: func(fs *flag.FlagSet) {
			fs.BoolVar(&unread, "unread", false, "only unread items")
			fs.IntVar(&limit, "limit", 50, "maximum rows (newest first)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListInbox(ctx, unread, limit)
			return list, func(w io.Writer) { printInbox(w, list) }, err
		},
	}
}

func printInbox(out io.Writer, list []apiclient.InboxItem) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "inbox is empty")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTIME\tSEVERITY\tAPP\tRULE\tEVENT\t")
	for _, e := range list {
		mark := "*"
		if e.Read {
			mark = ""
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.ID, e.At.Format(time.RFC3339), orDash(e.Severity), orDash(e.App), e.RuleName, e.Event, mark)
	}
	_ = tw.Flush()
}

func inboxCountCommand() apiCmd {
	return apiCmd{
		label: "alerts inbox count",
		usage: alertsInboxUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			n, err := c.InboxUnreadCount(ctx)
			return map[string]int{"unread": n}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "%d unread\n", n) }, err
		},
	}
}

func inboxReadCommand() apiCmd {
	var ids stringList
	var all bool
	return apiCmd{
		label: "alerts inbox read",
		usage: alertsInboxUsage,
		setup: func(fs *flag.FlagSet) {
			fs.Var(&ids, "id", "inbox item ID to mark read (repeatable)")
			fs.BoolVar(&all, "all", false, "mark everything read")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if !all && len(ids) == 0 {
				return nil, nil, newValidationError("give --id ID (repeatable) or --all")
			}
			err := c.MarkInboxRead(ctx, ids, all)
			return map[string]bool{"ok": err == nil}, func(w io.Writer) { _, _ = fmt.Fprintln(w, "marked read") }, err
		},
	}
}

func inboxPrefsCommand() apiCmd {
	var (
		minSev, digest, channel string
		hour                    = -1
		mute, unmute            stringList
	)
	return apiCmd{
		label: "alerts inbox prefs",
		usage: alertsInboxUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&minSev, "min-severity", "", "info, warning or critical")
			fs.Var(&mute, "mute", "hide alerts for this app (repeatable)")
			fs.Var(&unmute, "unmute", "stop hiding this app (repeatable)")
			fs.StringVar(&digest, "digest", "", "on or off")
			fs.IntVar(&hour, "digest-hour", -1, "UTC hour (0 to 23) to send the daily digest")
			fs.StringVar(&channel, "digest-channel", "", "notification channel ID for the digest")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			cur, err := c.GetInboxPrefs(ctx)
			if err != nil {
				return nil, nil, err
			}
			if minSev != "" || len(mute) > 0 || len(unmute) > 0 || digest != "" || hour >= 0 || channel != "" {
				if digest != "" && digest != "on" && digest != "off" {
					return nil, nil, newValidationError("--digest must be on or off")
				}
				cur = applyInboxPrefFlags(cur, minSev, digest, channel, hour, mute, unmute)
				if cur, err = c.SetInboxPrefs(ctx, cur); err != nil {
					return nil, nil, err
				}
			}
			return cur, func(w io.Writer) { printInboxPrefs(w, cur) }, nil
		},
	}
}

func applyInboxPrefFlags(p apiclient.InboxPrefs, minSev, digest, channel string, hour int, mute, unmute []string) apiclient.InboxPrefs {
	if minSev != "" {
		p.MinSeverity = minSev
	}
	if digest != "" {
		p.DigestEnabled = digest == "on"
	}
	if hour >= 0 {
		p.DigestHour = hour
	}
	if channel != "" {
		p.DigestChannelID = channel
	}
	drop := map[string]bool{}
	for _, a := range unmute {
		drop[a] = true
	}
	kept := make([]string, 0, len(p.MutedApps)+len(mute))
	seen := map[string]bool{}
	for _, a := range append(append([]string{}, p.MutedApps...), mute...) {
		if !drop[a] && !seen[a] {
			seen[a] = true
			kept = append(kept, a)
		}
	}
	p.MutedApps = kept
	return p
}

func printInboxPrefs(w io.Writer, p apiclient.InboxPrefs) {
	_, _ = fmt.Fprintf(w, "min severity:   %s\n", p.MinSeverity)
	_, _ = fmt.Fprintf(w, "muted apps:     %s\n", orDash(strings.Join(p.MutedApps, ", ")))
	state := "off"
	if p.DigestEnabled {
		state = fmt.Sprintf("on at %02d:00 UTC via %s", p.DigestHour, orDash(p.DigestChannelID))
	}
	_, _ = fmt.Fprintf(w, "daily digest:   %s\n", state)
}
