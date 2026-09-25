package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func logsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s logs archive set --target ID [--app NAME] [--interval 1h] [--retention-days N] [--disable]
  %[1]s logs archive status                    show every archive policy and its last run
  %[1]s logs archive remove [--app NAME]       stop archiving (all apps when --app is omitted)
  %[1]s logs dump --target ID --from TIME [--to TIME] [--app NAME] [--wait]   archive a time range now
  %[1]s logs ls --target ID [--app NAME]       list archived objects
  %[1]s logs fetch --target ID --key KEY [--out FILE]   download one archived object (gzip NDJSON)

TIME is an RFC3339 timestamp or a duration back from now such as 24h.
Without --app, a policy or dump covers every app.
%[2]s`, prog, commonFlagsHelp)
}

func runLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, logsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, logsUsage(prog))
		return exitOK
	case "archive":
		return runLogsArchive(prog, rest, stdout, stderr, lookupEnv)
	case "dump":
		return runLogsDump(prog, rest, stdout, stderr, lookupEnv)
	case "ls":
		return runLogsLs(prog, rest, stdout, stderr, lookupEnv)
	case "fetch":
		return runLogsFetch(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown logs subcommand %q\n\n%s", prog, args[0], logsUsage(prog))
		return exitUsage
	}
}

func runLogsArchive(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, logsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "set":
		return runLogsArchiveSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runLogsArchiveStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runLogsArchiveRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown logs archive subcommand %q\n\n%s", prog, args[0], logsUsage(prog))
		return exitUsage
	}
}

func runLogsArchiveSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var req apiclient.LogArchivePolicyRequest
	var disable bool
	c, code, ok := parseCLICall(prog, "logs archive set", logsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&req.AppName, "app", "", "app name (all apps when omitted)")
		fs.StringVar(&req.TargetID, "target", "", "storage destination id (required)")
		fs.StringVar(&req.Interval, "interval", "1h", "how often to ship new logs (5m to 24h)")
		fs.IntVar(&req.RetentionDays, "retention-days", 0, "delete archived objects older than this many days (0 keeps forever)")
		fs.BoolVar(&disable, "disable", false, "keep the policy but pause it")
	})
	if !ok {
		return code
	}
	if req.TargetID == "" {
		return c.invalid("--target is required")
	}
	enabled := !disable
	req.Enabled = &enabled
	p, err := c.client.SetLogArchivePolicy(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("set log archive policy: %w", err))
	}
	return c.render(p, func() {
		_, _ = fmt.Fprintf(stdout, "log archive policy for %s set (every %s, target %s)\n", scopeName(p.AppName), p.Interval, p.TargetID)
	})
}

func scopeName(app string) string {
	if app == "" {
		return "all apps"
	}
	return app
}

func runLogsArchiveStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "logs archive status", logsUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	policies, err := c.client.ListLogArchivePolicies(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list log archive policies: %w", err))
	}
	return c.render(policies, func() {
		if len(policies) == 0 {
			_, _ = fmt.Fprintln(stdout, "no log archive policies")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SCOPE\tTARGET\tEVERY\tRETENTION\tENABLED\tLAST SUCCESS\tLAST ERROR")
		for _, p := range policies {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%dd\t%t\t%s\t%s\n", scopeName(p.AppName), p.TargetID, p.Interval, p.RetentionDays, p.Enabled, p.LastSuccessAt, p.LastError)
		}
		_ = tw.Flush()
	})
}

func runLogsArchiveRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app string
	c, code, ok := parseCLICall(prog, "logs archive remove", logsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "app name (all apps when omitted)")
	})
	if !ok {
		return code
	}
	if err := c.client.DeleteLogArchivePolicy(context.Background(), app); err != nil {
		return c.fail(fmt.Errorf("remove log archive policy: %w", err))
	}
	return c.render(map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "log archive policy for %s removed\n", scopeName(app))
	})
}

func parseTimeArg(s string, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not an RFC3339 timestamp or a duration such as 24h", s)
	}
	return now.Add(-d), nil
}

func runLogsDump(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app, target, from, to string
	var wait bool
	c, code, ok := parseCLICall(prog, "logs dump", logsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "app name (all apps when omitted)")
		fs.StringVar(&target, "target", "", "storage destination id (required)")
		fs.StringVar(&from, "from", "", "range start (required)")
		fs.StringVar(&to, "to", "0s", "range end (default now)")
		fs.BoolVar(&wait, "wait", false, "wait for the dump to finish")
	})
	if !ok {
		return code
	}
	if target == "" || from == "" {
		return c.invalid("--target and --from are required")
	}
	now := time.Now().UTC()
	start, err := parseTimeArg(from, now)
	if err != nil {
		return c.invalid("--from: %v", err)
	}
	end, err := parseTimeArg(to, now)
	if err != nil {
		return c.invalid("--to: %v", err)
	}
	run, err := c.client.StartLogArchiveDump(context.Background(), apiclient.LogArchiveDumpRequest{
		AppName: app, TargetID: target, From: start.Format(time.RFC3339), To: end.Format(time.RFC3339),
	})
	if err != nil {
		return c.fail(fmt.Errorf("start log dump: %w", err))
	}
	if wait {
		if run, err = waitForArchiveRun(context.Background(), c.client, run); err != nil {
			return c.fail(err)
		}
	}
	if rc := c.render(run, func() {
		_, _ = fmt.Fprintf(stdout, "dump %s %s: %d lines in %d objects\n", run.ID, run.Status, run.Lines, run.Objects)
	}); rc != exitOK {
		return rc
	}
	if run.Status == "failed" {
		return c.fail(fmt.Errorf("log dump failed: %s", run.Error))
	}
	return exitOK
}

func waitForArchiveRun(ctx context.Context, client *apiclient.Client, run apiclient.LogArchiveRun) (apiclient.LogArchiveRun, error) {
	for run.Status == "running" {
		select {
		case <-ctx.Done():
			return run, fmt.Errorf("wait for log dump: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
		runs, err := client.ListLogArchiveRuns(ctx, run.AppName, false)
		if err != nil {
			return run, fmt.Errorf("poll log dump: %w", err)
		}
		for _, r := range runs {
			if r.ID == run.ID {
				run = r
			}
		}
	}
	return run, nil
}

func runLogsLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app, target, cursor string
	c, code, ok := parseCLICall(prog, "logs ls", logsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "app name (all apps when omitted)")
		fs.StringVar(&target, "target", "", "storage destination id (required)")
		fs.StringVar(&cursor, "cursor", "", "resume from a previous page's cursor")
	})
	if !ok {
		return code
	}
	if target == "" {
		return c.invalid("--target is required")
	}
	page, err := c.client.ListLogArchiveObjects(context.Background(), target, app, cursor)
	if err != nil {
		return c.fail(fmt.Errorf("list archived logs: %w", err))
	}
	return c.render(page, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KEY\tSIZE\tMODIFIED")
		for _, o := range page.Objects {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\n", o.Key, o.Size, o.LastModified)
		}
		_ = tw.Flush()
		if page.Next != "" {
			_, _ = fmt.Fprintf(stdout, "more results: --cursor %s\n", page.Next)
		}
	})
}

func runLogsFetch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var target, key, out string
	c, code, ok := parseCLICall(prog, "logs fetch", logsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&target, "target", "", "storage destination id (required)")
		fs.StringVar(&key, "key", "", "object key from logs ls (required)")
		fs.StringVar(&out, "out", "", "write to this file (default: the object's own name)")
	})
	if !ok {
		return code
	}
	if target == "" || key == "" {
		return c.invalid("--target and --key are required")
	}
	data, err := c.client.DownloadLogArchiveObject(context.Background(), target, key)
	if err != nil {
		return c.fail(fmt.Errorf("download archived log %q: %w", key, err))
	}
	if out == "" {
		out = path.Base(key)
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return c.fail(fmt.Errorf("write %s: %w", out, err))
	}
	return c.render(map[string]any{"file": out, "bytes": len(data)}, func() {
		_, _ = fmt.Fprintf(stdout, "wrote %d bytes to %s\n", len(data), out)
	})
}
