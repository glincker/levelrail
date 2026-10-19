package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

func appsLogBudgetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps log-budget get <app> [flags]
  %[1]s apps log-budget set <app> --max-mb N [flags]
  %[1]s apps log-budget clear <app> [flags]
  %[1]s apps log-budget trim <app> [flags]

trim drops the oldest lines now until the app is back under its budget.
Caps the log bytes kept for one app; the oldest lines are trimmed past it.
--max-mb 0 or clear falls back to the control plane default (APP_LOG_BUDGET_MB).
`, prog)
}

func runLogsUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := fmt.Sprintf("Usage:\n  %s logs usage [flags]\n%s", prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "logs usage", usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := c.client.GetLogUsage(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("get log usage: %w", err))
	}
	return c.render(res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "APP\tSTORED\tLINES\tRATE/MIN\tBUDGET\tUSED\tFORECAST\tTRIMMED")
		for _, a := range res.Apps {
			budget, used, forecast := "unlimited", "-", "-"
			if a.BudgetBytes > 0 {
				budget = humanBytes(a.BudgetBytes)
			}
			if a.Percent != nil {
				used = fmt.Sprintf("%.0f%%", *a.Percent)
			}
			if a.ForecastDays != nil {
				forecast = fmt.Sprintf("%.1fd", *a.ForecastDays)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%d\n", a.App, humanBytes(a.Bytes), a.Lines,
				humanBytes(int64(a.RateBytesMin)), budget, used, forecast, a.TrimmedLines)
		}
		_ = tw.Flush()
	})
}

func runAppsLogBudget(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, appsLogBudgetUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	verb := args[0]
	switch args[0] {
	case "get", "set", "clear", "trim":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps log-budget subcommand %q\n\n%s", prog, verb, appsLogBudgetUsage(prog))
		return exitUsage
	}
	var maxMB int64 = -1
	label := "apps log-budget " + verb
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args[1:], stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		if verb == "set" {
			fs.Int64Var(&maxMB, "max-mb", -1, "log budget in MiB (0 clears)")
		}
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	switch verb {
	case "set":
		if maxMB < 0 {
			_, _ = fmt.Fprintf(stderr, "%s: --max-mb is required\n", prog)
			return exitUsage
		}
		res, err := c.client.SetLogBudget(ctx, app, maxMB)
		if err != nil {
			return c.fail(fmt.Errorf("set log budget for %q: %w", app, err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "log budget for %s set to %d MiB\n", app, res.MaxMB)
		})
	case "trim":
		res, err := c.client.TrimLogBudget(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("trim logs for %q: %w", app, err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "trimmed %d lines (%s) from %s\n", res.TrimmedLines, humanBytes(res.TrimmedBytes), app)
			if res.StillOver {
				_, _ = fmt.Fprintln(stdout, "still over budget, run trim again to keep draining")
			}
		})
	case "clear":
		if err := c.client.DeleteLogBudget(ctx, app); err != nil {
			return c.fail(fmt.Errorf("clear log budget for %q: %w", app, err))
		}
		return c.render(map[string]bool{"cleared": true}, func() {
			_, _ = fmt.Fprintf(stdout, "log budget for %s cleared\n", app)
		})
	default:
		res, err := c.client.GetLogBudget(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("get log budget for %q: %w", app, err))
		}
		return c.render(res, func() {
			switch {
			case res.MaxMB > 0:
				_, _ = fmt.Fprintf(stdout, "%s: %d MiB (override)\n", app, res.MaxMB)
			case res.DefaultMB > 0:
				_, _ = fmt.Fprintf(stdout, "%s: %d MiB (default)\n", app, res.DefaultMB)
			default:
				_, _ = fmt.Fprintf(stdout, "%s: unlimited\n", app)
			}
		})
	}
}
