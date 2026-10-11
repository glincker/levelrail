package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func logsExplorerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s logs signatures <app> [--since 24h | --from T --to T] [--level error] [--q T] [--limit 25] [--new]
  %[1]s logs saved list [--app NAME]
  %[1]s logs saved create <name> [--app NAME] [--q T] [--level L] [--stream S] [--field k=v]... [--range 1h] [--share] [--pin]
  %[1]s logs saved run <id> [--limit N]
  %[1]s logs saved delete <id>
  %[1]s logs alert-from <app> --name NAME (--count N | --per-minute R) [--q T] [--regex RE] [--level L] [--field k=v]...
        [--window 5m] [--cooldown 15m] [--severity S] [--channel ID | --notify-url URL --notify-kind K]

signatures clusters similar error lines (numbers, ids and timestamps are
normalised away) and prints the top clusters with counts. --new compares with
the window of equal length just before and marks clusters that are new.
alert-from creates a log_pattern rule (--count) or a log_level_rate rule
(--per-minute) from the same filters "logs search" takes.
%[2]s`, prog, commonFlagsHelp)
}

func runLogsExplorer(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	switch sub {
	case "signatures":
		return runAPICmd(prog, logsSignaturesCommand(), args, stdout, stderr, lookupEnv)
	case "alert-from":
		return runAPICmd(prog, logsAlertFromCommand(), args, stdout, stderr, lookupEnv)
	case "saved":
		if len(args) == 0 {
			_, _ = fmt.Fprint(stderr, logsExplorerUsage(prog))
			return exitUsage
		}
		cmds := map[string]apiCmd{
			"list": logsSavedListCommand(), "create": logsSavedCreateCommand(),
			"run": logsSavedRunCommand(), "delete": logsSavedDeleteCommand(),
		}
		cmd, ok := cmds[args[0]]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "%s: unknown logs saved subcommand %q\n\n%s", prog, args[0], logsExplorerUsage(prog))
			return exitUsage
		}
		return runAPICmd(prog, cmd, args[1:], stdout, stderr, lookupEnv)
	}
	return exitUsage
}

func logsSignaturesCommand() apiCmd {
	var tr timeRangeFlags
	var q apiclient.LogSignatureQuery
	return apiCmd{
		label: "logs signatures",
		usage: logsExplorerUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&tr.since, "since", "", "how far back (default 1h)")
			fs.StringVar(&tr.from, "from", "", "RFC3339 window start")
			fs.StringVar(&tr.to, "to", "", "RFC3339 window end (default now)")
			fs.StringVar(&q.Level, "level", "", "minimum level (default error)")
			fs.StringVar(&q.Text, "q", "", "only lines containing this phrase")
			fs.IntVar(&q.Limit, "limit", 25, "top N signatures")
			fs.BoolVar(&q.Baseline, "new", false, "mark signatures absent from the window just before")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			from, to, err := resolveTimeRange(tr, time.Now())
			if err != nil {
				return nil, nil, err
			}
			q.From, q.To = from, to
			res, err := c.GetLogSignatures(ctx, pos[0], q)
			return res, func(w io.Writer) { printLogSignatures(w, res) }, err
		},
	}
}

func printLogSignatures(w io.Writer, r apiclient.LogSignatures) {
	if len(r.Signatures) == 0 {
		_, _ = fmt.Fprintln(w, "no error signatures in this window")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCOUNT\tFIRST SEEN\tLAST SEEN\tNEW\tSIGNATURE")
	for _, s := range r.Signatures {
		isNew := ""
		if s.New {
			isNew = "new"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n", s.ID, s.Count,
			s.FirstSeen.UTC().Format(time.RFC3339), s.LastSeen.UTC().Format(time.RFC3339), isNew, s.Template)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "# %d signatures from %d lines", r.Total, r.Scanned)
	if r.Truncated {
		_, _ = fmt.Fprintf(w, " (newest %d only; narrow the window)", r.MaxRows)
	}
	_, _ = fmt.Fprintln(w)
}

func logsSavedListCommand() apiCmd {
	var app string
	return apiCmd{
		label: "logs saved list",
		usage: logsExplorerUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&app, "app", "", "only queries for this app (plus unscoped ones)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListSavedLogQueries(ctx, app)
			return list, func(w io.Writer) { printSavedLogQueries(w, list) }, err
		},
	}
}

func printSavedLogQueries(w io.Writer, list []apiclient.SavedLogQuery) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(w, "no saved log queries")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tAPP\tSHARED\tPINNED\tQUERY")
	for _, q := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%t\t%s\n", q.ID, q.Name, q.App, q.Visibility == "instance", q.Pinned, describeLogSpec(q.Spec))
	}
	_ = tw.Flush()
}

func describeLogSpec(s apiclient.LogQuerySpec) string {
	var parts []string
	if s.Q != "" {
		parts = append(parts, fmt.Sprintf("q=%q", s.Q))
	}
	if s.Level != "" {
		parts = append(parts, "level>="+s.Level)
	}
	if s.Stream != "" {
		parts = append(parts, "stream="+s.Stream)
	}
	parts = append(parts, s.Fields...)
	if s.Range != "" {
		parts = append(parts, "range="+s.Range)
	}
	return strings.Join(parts, " ")
}

func logsSavedCreateCommand() apiCmd {
	var req apiclient.SavedLogQueryRequest
	var fields stringList
	var share bool
	return apiCmd{
		label: "logs saved create",
		usage: logsExplorerUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&req.App, "app", "", "scope the query to one app")
			fs.StringVar(&req.Spec.Q, "q", "", "full text phrase")
			fs.StringVar(&req.Spec.Level, "level", "", "minimum level")
			fs.StringVar(&req.Spec.Stream, "stream", "", "stdout or stderr")
			fs.StringVar(&req.Spec.Container, "container", "", "container id prefix")
			fs.Var(&fields, "field", "structured field filter, repeatable")
			fs.StringVar(&req.Spec.Range, "range", "1h", "relative window such as 15m, 1h or 7d")
			fs.BoolVar(&share, "share", false, "share with everyone who can see the app (needs write access)")
			fs.BoolVar(&req.Pinned, "pin", false, "pin to the app's log page (needs --app)")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			req.Name, req.Spec.Fields = pos[0], fields
			if share {
				req.Visibility = "instance"
			}
			saved, err := c.CreateSavedLogQuery(ctx, req)
			return saved, func(w io.Writer) { _, _ = fmt.Fprintf(w, "saved log query %s (%s)\n", saved.Name, saved.ID) }, err
		},
	}
}

func logsSavedDeleteCommand() apiCmd {
	return apiCmd{
		label: "logs saved delete",
		usage: logsExplorerUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			err := c.DeleteSavedLogQuery(ctx, pos[0])
			return map[string]string{"deleted": pos[0]}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "deleted %s\n", pos[0]) }, err
		},
	}
}

func logsSavedRunCommand() apiCmd {
	var limit int
	return apiCmd{
		label: "logs saved run",
		usage: logsExplorerUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) { fs.IntVar(&limit, "limit", 200, "newest N matches") },
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			list, err := c.ListSavedLogQueries(ctx, "")
			if err != nil {
				return nil, nil, err
			}
			for _, q := range list {
				if q.ID != pos[0] {
					continue
				}
				if q.App == "" {
					return nil, nil, newValidationError("saved query %s has no app; pass it to \"logs search <app>\" instead", q.ID)
				}
				now := time.Now()
				s := apiclient.LogSearch{Text: q.Spec.Q, Level: q.Spec.Level, Stream: q.Spec.Stream, Container: q.Spec.Container,
					Fields: q.Spec.Fields, From: now.Add(-time.Hour), To: now, Limit: limit}
				if d, ok := parseRelativeRange(q.Spec.Range); ok {
					s.From = now.Add(-d)
				}
				if q.Spec.From != nil {
					s.From = *q.Spec.From
				}
				if q.Spec.To != nil {
					s.To = *q.Spec.To
				}
				res, err := c.SearchAppLogs(ctx, q.App, s)
				return res, func(w io.Writer) { printLogSearch(w, res) }, err
			}
			return nil, nil, newValidationError("no saved query with id %q", pos[0])
		},
	}
}

func logsAlertFromCommand() apiCmd {
	var (
		name, regex, level, window, cooldown, severity, channel, notifyURL, notifyKind, text, forDur string
		count                                                                                        int
		perMinute                                                                                    float64
		fields                                                                                       stringList
	)
	return apiCmd{
		label: "logs alert-from",
		usage: logsExplorerUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "rule name (required)")
			fs.StringVar(&text, "q", "", "full text phrase")
			fs.StringVar(&regex, "regex", "", "RE2 pattern on the message")
			fs.StringVar(&level, "level", "", "minimum level")
			fs.Var(&fields, "field", "structured field filter, repeatable")
			fs.IntVar(&count, "count", 0, "fire when this many lines match inside the window")
			fs.Float64Var(&perMinute, "per-minute", 0, "fire when matching lines per minute reach this rate")
			fs.StringVar(&window, "window", "5m", "look-back window")
			fs.StringVar(&cooldown, "cooldown", "", "suppress re-firing for this long after a firing")
			fs.StringVar(&forDur, "for", "", "how long the condition must hold before firing")
			fs.StringVar(&severity, "severity", "", "info, warning or critical")
			fs.StringVar(&channel, "channel", "", "notification channel id")
			fs.StringVar(&notifyURL, "notify-url", "", "webhook URL when no channel is attached")
			fs.StringVar(&notifyKind, "notify-kind", "", "payload shape for --notify-url")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			if name == "" {
				return nil, nil, newValidationError("--name is required")
			}
			if (count > 0) == (perMinute > 0) {
				return nil, nil, newValidationError("give exactly one of --count and --per-minute")
			}
			w, err := time.ParseDuration(window)
			if err != nil || w <= 0 {
				return nil, nil, newValidationError("--window must be a duration such as 5m")
			}
			cfg := &apiclient.AlertLogRule{Query: text, Regex: regex, Level: level, Fields: fields, WindowSeconds: int(w / time.Second)}
			if cooldown != "" {
				cd, err := time.ParseDuration(cooldown)
				if err != nil || cd < 0 {
					return nil, nil, newValidationError("--cooldown must be a duration such as 15m")
				}
				cfg.CooldownSeconds = int(cd / time.Second)
			}
			kind := "log_pattern"
			if perMinute > 0 {
				kind, cfg.PerMinute = "log_level_rate", perMinute
			} else {
				cfg.Count = count
			}
			rule, err := c.CreateAlertRule(ctx, pos[0], apiclient.CreateAlertRuleRequest{
				Name: name, Kind: kind, LogRule: cfg, ForDuration: forDur, Severity: severity,
				ChannelID: channel, NotifyURL: notifyURL, NotifyKind: notifyKind, Enabled: true,
			})
			return rule, func(out io.Writer) { _, _ = fmt.Fprintf(out, "created %s rule %s (%s)\n", kind, rule.Name, rule.ID) }, err
		},
	}
}

// parseRelativeRange reads the 15m, 1h, 7d form saved queries store.
func parseRelativeRange(r string) (time.Duration, bool) {
	if len(r) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(r[:len(r)-1])
	if err != nil || n <= 0 {
		return 0, false
	}
	switch r[len(r)-1] {
	case 'm':
		return time.Duration(n) * time.Minute, true
	case 'h':
		return time.Duration(n) * time.Hour, true
	case 'd':
		return time.Duration(n) * 24 * time.Hour, true
	}
	return 0, false
}
