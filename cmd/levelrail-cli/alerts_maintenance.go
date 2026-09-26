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

func maintenanceCommands() map[string]apiCmd {
	return map[string]apiCmd{
		"list":   maintenanceListCommand(),
		"create": maintenanceWriteCommand(false),
		"update": maintenanceWriteCommand(true),
		"delete": maintenanceDeleteCommand(),
	}
}

func maintenanceListCommand() apiCmd {
	return apiCmd{
		label: "alerts maintenance list",
		usage: alertsMaintenanceUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListMaintenanceWindows(ctx)
			return list, func(w io.Writer) { printMaintenanceWindows(w, list) }, err
		},
	}
}

func maintenanceWriteCommand(update bool) apiCmd {
	var (
		name, cronExpr, dur, tz, scope string
		targets                        stringList
		disabled                       bool
	)
	label, args := "alerts maintenance create", 0
	if update {
		label, args = "alerts maintenance update", 1
	}
	return apiCmd{
		label: label,
		usage: alertsMaintenanceUsage,
		args:  args,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "window name (required)")
			fs.StringVar(&cronExpr, "cron", "", "5-field cron expression for each window start (required)")
			fs.StringVar(&dur, "duration", "", "how long each window lasts, e.g. 2h (required)")
			fs.StringVar(&tz, "tz", "UTC", "IANA timezone the cron is evaluated in")
			fs.StringVar(&scope, "scope", "all", "all, app or node")
			fs.Var(&targets, "target", "app or node name the window covers (repeatable, for --scope app or node)")
			fs.BoolVar(&disabled, "disabled", false, "keep the window but do not apply it")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			if name == "" || cronExpr == "" || dur == "" {
				return nil, nil, newValidationError("--name, --cron and --duration are required")
			}
			req := apiclient.MaintenanceWindowResource{Name: name, Cron: cronExpr, Duration: dur, Timezone: tz, Scope: scope, Targets: targets, Enabled: !disabled}
			var (
				w   apiclient.MaintenanceWindowResource
				err error
			)
			if update {
				w, err = c.UpdateMaintenanceWindow(ctx, pos[0], req)
			} else {
				w, err = c.CreateMaintenanceWindow(ctx, req)
			}
			return w, func(out io.Writer) { _, _ = fmt.Fprintf(out, "maintenance window %s saved (%s)\n", w.ID, w.Name) }, err
		},
	}
}

func maintenanceDeleteCommand() apiCmd {
	return apiCmd{
		label: "alerts maintenance delete",
		usage: alertsMaintenanceUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			err := c.DeleteMaintenanceWindow(ctx, pos[0])
			return map[string]string{"deleted": pos[0]}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "maintenance window %s deleted\n", pos[0]) }, err
		},
	}
}

func printMaintenanceWindows(out io.Writer, list []apiclient.MaintenanceWindowResource) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no maintenance windows")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSCHEDULE\tDURATION\tSCOPE\tSTATE\tNEXT")
	for _, w := range list {
		state := "idle"
		switch {
		case !w.Enabled:
			state = "disabled"
		case w.Active:
			state = "active"
		}
		next := "-"
		if w.NextStart != nil {
			next = w.NextStart.Format(time.RFC3339)
		}
		scope := w.Scope
		if len(w.Targets) > 0 {
			scope += ":" + strings.Join(w.Targets, ",")
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s (%s)\t%s\t%s\t%s\t%s\n", w.ID, w.Name, w.Cron, orDash(w.Timezone), w.Duration, scope, state, next)
	}
	_ = tw.Flush()
}

func historyCommand() apiCmd {
	var q apiclient.AlertHistoryQuery
	return apiCmd{
		label: "alerts history",
		usage: alertsHistoryUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&q.App, "app", "", "only this app")
			fs.StringVar(&q.RuleID, "rule", "", "only this rule ID")
			fs.StringVar(&q.Outcome, "outcome", "", "sent, silenced, grouped, inhibited, failed, ratelimited, flapping or skipped")
			fs.StringVar(&q.Event, "event", "", "fired, resolved, flapping or flap_ended")
			fs.StringVar(&q.Since, "since", "", "RFC 3339 timestamp lower bound")
			fs.IntVar(&q.Limit, "limit", 50, "maximum rows (newest first)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListAlertHistory(ctx, q)
			return list, func(w io.Writer) { printAlertHistory(w, list) }, err
		},
	}
}

func alertsHistoryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts history [--app NAME] [--rule ID] [--outcome OUTCOME] [--event EVENT] [--since TIME] [--limit N]

Lists alert firings and resolutions, newest first, with what happened to
each notification (sent, silenced, grouped, inhibited, failed, ratelimited,
flapping, skipped). Retention is set by APP_ALERT_HISTORY_RETENTION.
`, prog)
}

func printAlertHistory(out io.Writer, list []apiclient.AlertHistoryEntry) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no alert history")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIME\tAPP\tRULE\tEVENT\tOUTCOME\tDETAIL")
	for _, e := range list {
		detail := e.Detail
		if e.Error != "" {
			detail = strings.TrimSpace(detail + " " + e.Error)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", e.At.Format(time.RFC3339), orDash(e.App), e.RuleName, e.Event, e.Outcome, orDash(detail))
	}
	_ = tw.Flush()
}
