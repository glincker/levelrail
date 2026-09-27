package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func logsQueryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s logs query <app> [--level LEVEL] [--since 30m] [--until T] [--deploy ID] [--text PHRASE] [--max-lines N] [--max-bytes N] [flags]

Searches an app's stored logs and prints a capped excerpt of the newest
matching lines, plus how many lines matched in total. It never dumps the
whole log: use --since/--until, --level or --text to narrow, or "%[1]s apps logs"
for the uncapped search.

Flags:
  --level LEVEL      minimum level: trace, debug, info, warn, error or fatal
                     (lines with no detectable level are excluded)
  --since T          window start: a duration like 30m or an RFC3339 time (default 1h)
  --until T          window end: a duration ago or an RFC3339 time (default now)
  --deploy ID        only lines from that deploy attempt's window
  --text PHRASE      only lines containing this phrase
  --max-lines N      most recent lines to return (default %[2]d)
  --max-bytes N      output byte cap (default %[3]d, or $%[4]s)
  --json             print the excerpt as JSON to stdout and nothing else
`, prog, apiclient.DefaultLogMaxLines, apiclient.DefaultLogMaxBytes, apiclient.EnvLogMaxBytes)
}

func logsQueryCommand(lookupEnv func(string) (string, bool)) apiCmd {
	var q apiclient.LogQuery
	var maxBytes int
	return apiCmd{
		label: "logs query",
		usage: logsQueryUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&q.Level, "level", "", "minimum level: trace, debug, info, warn, error or fatal")
			fs.StringVar(&q.Since, "since", "", "window start: a duration like 30m or an RFC3339 time (default 1h)")
			fs.StringVar(&q.Until, "until", "", "window end: a duration ago or an RFC3339 time (default now)")
			fs.StringVar(&q.Deploy, "deploy", "", "only lines from this deploy attempt's window")
			fs.StringVar(&q.Text, "text", "", "only lines containing this phrase")
			fs.IntVar(&q.MaxLines, "max-lines", 0, "most recent lines to return")
			fs.IntVar(&maxBytes, "max-bytes", 0, "output byte cap")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			q.App = pos[0]
			if maxBytes < 0 || q.MaxLines < 0 {
				return nil, nil, newValidationError("--max-lines and --max-bytes must not be negative")
			}
			for flagName, v := range map[string]string{"since": q.Since, "until": q.Until} {
				if _, err := apiclient.ParseLogTime(v, time.Now()); v != "" && err != nil {
					return nil, nil, newValidationError("--%s: %v", flagName, err)
				}
			}
			if maxBytes == 0 {
				maxBytes = apiclient.LogMaxBytesFromEnv(lookupEnv)
			}
			ex, err := c.QueryLogsCompact(ctx, q, maxBytes, time.Now())
			return ex, func(w io.Writer) { printLogExcerpt(w, ex) }, err
		},
	}
}

func printLogExcerpt(w io.Writer, ex apiclient.LogExcerpt) {
	for _, l := range ex.Lines {
		_, _ = fmt.Fprintln(w, l)
	}
	if ex.Notice != "" {
		_, _ = fmt.Fprintf(w, "# %s\n", ex.Notice)
	} else if ex.Matched == 0 {
		_, _ = fmt.Fprintln(w, "# no matching log lines in this window")
	}
}
