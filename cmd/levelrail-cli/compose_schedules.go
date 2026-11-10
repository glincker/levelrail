package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const composeSchedulesUsage = `Usage:
  %[1]s compose schedules list <app>                                      every schedule of every service
  %[1]s compose schedules add <app> <service> --schedule CRON [--name N] -- CMD...
  %[1]s compose schedules run <app> <service> <id|name>                    run now, inside that service
  %[1]s compose schedules runs <app> <service> <id|name> [--limit N]       run history with exit codes
  %[1]s compose schedules pause|resume <app> <service> <id|name>
  %[1]s compose schedules rm <app> <service> <id|name>
`

// runComposeSchedules manages commands run on a schedule inside one member service,
// through that service's exec path (console exec must be enabled for it).
func runComposeSchedules(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprintf(stdout, composeSchedulesUsage, prog)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	verb, rest := args[0], args[1:]
	ctx := context.Background()
	switch verb {
	case "list":
		cmd, code, ok := parseComposeCmd(prog, "compose schedules list", "compose schedules list <app>", rest, 1, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		list, err := cmd.client.ListComposeSchedules(ctx, cmd.args[0])
		if err != nil {
			return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose schedules %q: %w", cmd.args[0], err))
		}
		return renderCompose(stdout, stderr, cmd, list, func() { printComposeSchedules(stdout, list) })
	case "add":
		return runComposeScheduleAdd(prog, rest, stdout, stderr, lookupEnv)
	case "runs":
		var limit int
		cmd, code, ok := parseComposeCmd(prog, "compose schedules runs", "compose schedules runs <app> <service> <id|name> [--limit N]", rest, 3, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.IntVar(&limit, "limit", 20, "newest N runs")
		})
		if !ok {
			return code
		}
		runs, err := cmd.client.ListComposeScheduleRuns(ctx, cmd.args[0], cmd.args[1], cmd.args[2], limit)
		if err != nil {
			return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose schedule runs: %w", err))
		}
		return renderCompose(stdout, stderr, cmd, runs, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "RUN\tTRIGGER\tSTATUS\tEXIT\tSTARTED\tDURATION\tMESSAGE")
			for _, r := range runs {
				exit := "-"
				if r.ExitCode != nil {
					exit = fmt.Sprint(*r.ExitCode)
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%dms\t%s\n", r.ID, r.Trigger, r.Status, exit, r.StartedAt.Format("2006-01-02 15:04:05"), r.DurationMS, dashIfEmpty(r.Message))
			}
			_ = tw.Flush()
		})
	case "run", "pause", "resume", "rm":
		cmd, code, ok := parseComposeCmd(prog, "compose schedules "+verb, "compose schedules "+verb+" <app> <service> <id|name>", rest, 3, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		app, svc, id := cmd.args[0], cmd.args[1], cmd.args[2]
		var err error
		msg := ""
		switch verb {
		case "run":
			var started apiclient.ComposeScheduleStarted
			if started, err = cmd.client.RunComposeSchedule(ctx, app, svc, id); err == nil {
				msg = fmt.Sprintf("started run %s; follow it with: %s compose schedules runs %s %s %s", dashIfEmpty(started.RunID), prog, app, svc, id)
			}
		case "pause", "resume":
			err, msg = cmd.client.SetComposeScheduleEnabled(ctx, app, svc, id, verb == "resume"), verb+"d "+id
		default:
			err, msg = cmd.client.DeleteComposeSchedule(ctx, app, svc, id), "removed "+id
		}
		if err != nil {
			return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose schedules %s: %w", verb, err))
		}
		return renderCompose(stdout, stderr, cmd, map[string]string{"result": msg}, func() { _, _ = fmt.Fprintln(stdout, msg) })
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown compose schedules command %q\n\n", prog, verb)
		_, _ = fmt.Fprintf(stderr, composeSchedulesUsage, prog)
		return exitUsage
	}
}

func runComposeScheduleAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	command := []string{}
	for i, a := range args {
		if a == "--" {
			command, args = args[i+1:], args[:i]
			break
		}
	}
	var schedule, name, timezone string
	var paused bool
	cmd, code, ok := parseComposeCmd(prog, "compose schedules add", "compose schedules add <app> <service> --schedule CRON [--name N] -- CMD...", args, 2, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&schedule, "schedule", "", "cron expression or @hourly style macro")
		fs.StringVar(&name, "name", "", "schedule name, unique within the service")
		fs.StringVar(&timezone, "timezone", "", "IANA zone the schedule is evaluated in (default UTC)")
		fs.BoolVar(&paused, "paused", false, "create it paused")
	})
	if !ok {
		return code
	}
	if schedule == "" || len(command) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: compose schedules add needs --schedule and a command after --\n", prog)
		return exitUsage
	}
	req := apiclient.ScheduledTaskRequest{Command: command, Schedule: schedule, Enabled: !paused, Name: name, Timezone: timezone}
	task, err := cmd.client.CreateComposeSchedule(context.Background(), cmd.args[0], cmd.args[1], req)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose schedules add: %w", err))
	}
	return renderCompose(stdout, stderr, cmd, task, func() {
		_, _ = fmt.Fprintf(stdout, "created %s on %s: %s runs %s\n", task.ID, cmd.args[1], task.Schedule, strings.Join(task.Command, " "))
	})
}

func printComposeSchedules(out io.Writer, list []apiclient.ComposeSchedule) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SERVICE\tID\tNAME\tSCHEDULE\tENABLED\tLAST STATUS\tCOMMAND")
	for _, s := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\n", s.Service, s.ID, dashIfEmpty(s.Name), s.Schedule, s.Enabled, dashIfEmpty(s.LastRunStatus), strings.Join(s.Command, " "))
	}
	_ = tw.Flush()
}
