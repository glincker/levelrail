package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func maintenanceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s maintenance list
  %[1]s maintenance create --title T --start TIME (--end TIME | --duration 2h) [--scope all|project|app] [--target NAME] [--freeze] [--notify-before 1h] [--message TEXT]
  %[1]s maintenance delete <id>

A window shows as a dashboard banner and on the public status page. --freeze holds
automatic deploys for its duration. TIME is RFC 3339, for example 2026-11-01T02:00:00Z.
For recurring alert silences see "%[1]s alerts maintenance".
`, prog)
}

func runMaintenance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchSub(prog, "maintenance", maintenanceUsage, scheduledMaintenanceCommands(), args, stdout, stderr, lookupEnv)
}

func scheduledMaintenanceCommands() map[string]apiCmd {
	return map[string]apiCmd{
		"list":   scheduledMaintenanceListCommand(),
		"create": scheduledMaintenanceCreateCommand(),
		"delete": scheduledMaintenanceDeleteCommand(),
	}
}

func scheduledMaintenanceListCommand() apiCmd {
	return apiCmd{
		label: "maintenance list",
		usage: maintenanceUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListScheduledMaintenance(ctx)
			return list, func(w io.Writer) { printScheduledMaintenance(w, list) }, err
		},
	}
}

func scheduledMaintenanceCreateCommand() apiCmd {
	var (
		title, message, scope, target, start, end string
		duration, notifyBefore                    time.Duration
		freeze                                    bool
	)
	return apiCmd{
		label: "maintenance create",
		usage: maintenanceUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&title, "title", "", "window title (required)")
			fs.StringVar(&message, "message", "", "details shown on the banner and status page")
			fs.StringVar(&scope, "scope", "all", "all, project or app")
			fs.StringVar(&target, "target", "", "project id or app name, for --scope project or app")
			fs.StringVar(&start, "start", "", "start time, RFC 3339 (required)")
			fs.StringVar(&end, "end", "", "end time, RFC 3339")
			fs.DurationVar(&duration, "duration", 0, "length of the window, instead of --end")
			fs.BoolVar(&freeze, "freeze", false, "hold automatic deploys during the window")
			fs.DurationVar(&notifyBefore, "notify-before", 0, "announce through alert channels this long before the start")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			startAt, err := time.Parse(time.RFC3339, start)
			if err != nil || title == "" {
				return nil, nil, newValidationError("--title and an RFC 3339 --start are required")
			}
			endAt := startAt.Add(duration)
			if end != "" {
				if endAt, err = time.Parse(time.RFC3339, end); err != nil {
					return nil, nil, newValidationError("--end must be RFC 3339")
				}
			} else if duration <= 0 {
				return nil, nil, newValidationError("give --end or --duration")
			}
			w, err := c.CreateScheduledMaintenance(ctx, apiclient.ScheduledMaintenanceRequest{
				Title: title, Message: message, Scope: scope, Target: target, StartsAt: startAt, EndsAt: endAt,
				Freeze: freeze, AnnounceLeadMins: int(notifyBefore / time.Minute),
			})
			return w, func(out io.Writer) { _, _ = fmt.Fprintf(out, "maintenance window %s scheduled (%s)\n", w.ID, w.Title) }, err
		},
	}
}

func scheduledMaintenanceDeleteCommand() apiCmd {
	return apiCmd{
		label: "maintenance delete",
		usage: maintenanceUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			err := c.DeleteScheduledMaintenance(ctx, pos[0])
			return map[string]string{"deleted": pos[0]}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "maintenance window %s removed\n", pos[0]) }, err
		},
	}
}

func printScheduledMaintenance(out io.Writer, list []apiclient.ScheduledMaintenanceResource) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no scheduled maintenance")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTITLE\tSCOPE\tSTART\tEND\tFREEZE\tSTATE")
	for _, w := range list {
		scope := w.Scope
		if w.Target != "" {
			scope += ":" + w.Target
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%t\t%s\n", w.ID, w.Title, scope,
			w.StartsAt.Format(time.RFC3339), w.EndsAt.Format(time.RFC3339), w.Freeze, w.State)
	}
	_ = tw.Flush()
}
