package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsErrorsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps errors list <app> [--status open|resolved|ignored] [--since 24h] [--limit N] [flags]
  %[1]s apps errors get <app> <fingerprint> [flags]       the issue and its latest event's stack trace
  %[1]s apps errors resolve <app> <fingerprint> [flags]   a later event reopens it as a regression
  %[1]s apps errors ignore <app> <fingerprint> [flags]
  %[1]s apps errors reopen <app> <fingerprint> [flags]
  %[1]s apps error-tracking enable <app> [flags]          create (or rotate) the app's DSN and print it
  %[1]s apps error-tracking status <app> [flags]
  %[1]s apps error-tracking disable <app> [flags]         remove the key; SDKs get 401s

Errors arrive from any Sentry SDK pointed at the app's DSN and are grouped into
issues tied to the deploy that produced them.
`, prog)
}

func runAppsErrors(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, appsErrorsUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "list":
		return runErrorsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runErrorsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "resolve":
		return runErrorsSetStatus(prog, "resolved", args[1:], stdout, stderr, lookupEnv)
	case "ignore":
		return runErrorsSetStatus(prog, "ignored", args[1:], stdout, stderr, lookupEnv)
	case "reopen":
		return runErrorsSetStatus(prog, "open", args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps errors subcommand %q\n\n%s", prog, args[0], appsErrorsUsage(prog))
	return exitUsage
}

func runErrorsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var o apiclient.ErrorListOptions
	label := "apps errors list"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n\nFlags:\n  --status string  open, resolved or ignored\n  --since string   a duration like 24h or an RFC3339 time\n  --limit int      at most this many (default 50)\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&o.Status, "status", "", "open, resolved or ignored")
		fs.StringVar(&o.Since, "since", "", "duration or RFC3339 time")
		fs.IntVar(&o.Limit, "limit", 0, "maximum issues")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	issues, err := c.client.ListErrors(context.Background(), app, o)
	if err != nil {
		return c.fail(fmt.Errorf("list errors for %q: %w", app, err))
	}
	return c.render(issues, func() {
		if len(issues) == 0 {
			_, _ = fmt.Fprintln(stdout, "no errors in that window")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FINGERPRINT\tSTATUS\tLEVEL\tCOUNT\tLAST SEEN\tTITLE")
		for _, i := range issues {
			status := i.Status
			if i.Regressed {
				status += " (regressed)"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", i.Fingerprint, status, i.Level, i.Count,
				time.Unix(0, i.LastSeenNs).UTC().Format(time.RFC3339), i.Title)
		}
		_ = tw.Flush()
	})
}

func runErrorsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps errors get"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> <fingerprint> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if c.fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs an app name and a fingerprint\n", prog, label)
		return exitUsage
	}
	app, fp := c.fs.Arg(0), c.fs.Arg(1)
	d, err := c.client.GetError(context.Background(), app, fp)
	if err != nil {
		return c.fail(fmt.Errorf("get error %q for %q: %w", fp, app, err))
	}
	return c.render(d, func() { printErrorDetail(stdout, d) })
}

func runErrorsSetStatus(prog, status string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verbs := map[string]string{"resolved": "resolve", "ignored": "ignore", "open": "reopen"}
	label := "apps errors " + verbs[status]
	usage := fmt.Sprintf("Usage:\n  %s %s <app> <fingerprint> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if c.fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs an app name and a fingerprint\n", prog, label)
		return exitUsage
	}
	app, fp := c.fs.Arg(0), c.fs.Arg(1)
	d, err := c.client.SetErrorStatus(context.Background(), app, fp, status)
	if err != nil {
		return c.fail(fmt.Errorf("%s error %q for %q: %w", verbs[status], fp, app, err))
	}
	return c.render(d.Issue, func() {
		_, _ = fmt.Fprintf(stdout, "%s is now %s\n", d.Issue.Fingerprint, d.Issue.Status)
	})
}

type eventView struct {
	Message   json.RawMessage `json:"message"`
	Exception json.RawMessage `json:"exception"`
}

type exceptionView struct {
	Type       string `json:"type"`
	Value      string `json:"value"`
	Stacktrace struct {
		Frames []struct {
			Filename string `json:"filename"`
			Function string `json:"function"`
			Lineno   int    `json:"lineno"`
		} `json:"frames"`
	} `json:"stacktrace"`
}

func printErrorDetail(out io.Writer, d apiclient.ErrorIssueDetail) {
	i := d.Issue
	_, _ = fmt.Fprintf(out, "%s\n%s  level=%s  status=%s  count=%d\n", i.Title, i.Fingerprint, i.Level, i.Status, i.Count)
	_, _ = fmt.Fprintf(out, "first seen %s, last seen %s\n", time.Unix(0, i.FirstSeenNs).UTC().Format(time.RFC3339),
		time.Unix(0, i.LastSeenNs).UTC().Format(time.RFC3339))
	if i.LastRelease != "" || i.LastDeployID != "" {
		_, _ = fmt.Fprintf(out, "release %s, deploy %s\n", i.LastRelease, i.LastDeployID)
	}
	if i.Regressed {
		_, _ = fmt.Fprintln(out, "regression: this issue was resolved and happened again")
	}
	if len(d.Events) == 0 {
		return
	}
	var ev eventView
	_ = json.Unmarshal(d.Events[0].Event, &ev)
	var wrapped struct {
		Values []exceptionView `json:"values"`
	}
	var vals []exceptionView
	if json.Unmarshal(ev.Exception, &wrapped) == nil {
		vals = wrapped.Values
	} else {
		_ = json.Unmarshal(ev.Exception, &vals)
	}
	for _, v := range vals {
		_, _ = fmt.Fprintf(out, "\n%s: %s\n", v.Type, v.Value)
		fr := v.Stacktrace.Frames
		for n := len(fr) - 1; n >= 0; n-- {
			_, _ = fmt.Fprintf(out, "  at %s (%s:%d)\n", fr[n].Function, fr[n].Filename, fr[n].Lineno)
		}
	}
}

func runAppsErrorTracking(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, appsErrorsUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	verb := args[0]
	if verb != "enable" && verb != "status" && verb != "disable" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps error-tracking subcommand %q\n\n%s", prog, verb, appsErrorsUsage(prog))
		return exitUsage
	}
	label := "apps error-tracking " + verb
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args[1:], stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	switch verb {
	case "enable":
		res, err := c.client.EnableErrorTracking(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("enable error tracking for %q: %w", app, err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "error tracking is on for %s. Set this on the app, then redeploy:\n\n  SENTRY_DSN=%s\n\n", app, res.DSN)
			_, _ = fmt.Fprintln(stdout, "The DSN is shown once. Running this again rotates it.")
		})
	case "disable":
		if err := c.client.DisableErrorTracking(ctx, app); err != nil {
			return c.fail(fmt.Errorf("disable error tracking for %q: %w", app, err))
		}
		return c.render(map[string]bool{"disabled": true}, func() {
			_, _ = fmt.Fprintf(stdout, "error tracking is off for %s\n", app)
		})
	default:
		res, err := c.client.GetErrorTracking(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("get error tracking for %q: %w", app, err))
		}
		return c.render(res, func() {
			state := "off"
			if res.Enabled {
				state = "on"
			}
			_, _ = fmt.Fprintf(stdout, "error tracking: %s\nendpoint: %s\n", state, res.Endpoint)
		})
	}
}
