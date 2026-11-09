package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"text/tabwriter"
	"time"
)

// logsAccessCommand implements "logs access": a bounded search of the edge's
// recent request log across every app the caller can read.
func logsAccessCommand() apiCmd {
	filters := map[string]*string{}
	return apiCmd{
		label: "logs access",
		usage: logsAccessUsage,
		args:  0,
		setup: func(fs *flag.FlagSet) {
			for _, f := range []struct{ name, help string }{
				{"app", "only this app's hosts"},
				{"host", "exact host, e.g. api.example.com"},
				{"method", "HTTP method, e.g. POST"},
				{"path", "path prefix, e.g. /api/"},
				{"q", "case-insensitive text anywhere in the path"},
				{"status", "exact code (404) or class (5xx)"},
				{"status-min", "lowest status to include, e.g. 500"},
				{"status-max", "highest status to include"},
				{"min-ms", "only requests slower than this many milliseconds"},
				{"since", "how far back, e.g. 15m (default 1h, capped by the server)"},
				{"from", "RFC3339 start (overrides --since)"},
				{"to", "RFC3339 end (default now)"},
				{"limit", "rows per page (clamped by the server)"},
				{"cursor", "next_cursor from a previous page"},
			} {
				filters[f.name] = fs.String(f.name, "", f.help)
			}
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			q := url.Values{}
			for name, v := range filters {
				if *v != "" {
					q.Set(accessParamName(name), *v)
				}
			}
			res, err := c.SearchAccessLog(ctx, q)
			return res, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "TIME\tAPP\tSTATUS\tMETHOD\tHOST\tPATH\tMS")
				for _, r := range res.Records {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%.0f\n", r.Time.UTC().Format(time.RFC3339), safeTerminalText(r.App), r.Status, safeTerminalText(r.Method), safeTerminalText(r.Host), safeTerminalText(r.Path), r.DurationMs)
				}
				_ = tw.Flush()
				if res.Truncated {
					_, _ = fmt.Fprintln(w, "# scan limit reached, narrow the filters for a complete result")
				}
				if res.NextCursor != "" {
					_, _ = fmt.Fprintf(w, "# more rows: --cursor %s\n", safeTerminalText(res.NextCursor))
				}
			}, err
		},
	}
}

func accessParamName(flagName string) string {
	switch flagName {
	case "status-min":
		return "status_min"
	case "status-max":
		return "status_max"
	case "min-ms":
		return "min_ms"
	}
	return flagName
}

func logsAccessUsage(prog string) string {
	return fmt.Sprintf(`Usage: %s logs access [--app A] [--host H] [--method M] [--path /prefix] [--q TEXT]
       [--status 404|5xx] [--status-min N] [--status-max N] [--min-ms N]
       [--since 1h | --from T --to T] [--limit N] [--cursor C] [flags]

Searches the edge's recent HTTP request log (kept in memory, newest first)
for the apps you can read. Query strings are never recorded and client IPs
are never returned. The time window, page size, term length and scan size
are capped by runtime settings (telemetry.access_search_*).
`, prog)
}
