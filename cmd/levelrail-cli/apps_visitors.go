package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

func runAppsVisitors(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var domain, window string
	label := "apps visitors"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n\nPrivacy-safe visitor summary from the edge access log: unique visitors (client addresses hashed with a rotating salt, never stored), requests, status classes, top pages and referrers.\n\nFlags:\n  --domain string   only this domain\n  --window string   1h, 24h, 7d or 30d (default 24h); 7d and 30d include stored hourly history\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&domain, "domain", "", "only this domain")
		fs.StringVar(&window, "window", "24h", "1h, 24h, 7d or 30d")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	v, err := c.client.GetVisitors(context.Background(), app, domain, window)
	if err != nil {
		return c.fail(fmt.Errorf("get visitors for %q: %w", app, err))
	}
	return c.render(v, func() {
		_, _ = fmt.Fprintf(stdout, "visitors %d  requests %d  (2xx %d, 3xx %d, 4xx %d, 5xx %d)  window %s\n", v.Visitors, v.Requests, v.Status["2xx"], v.Status["3xx"], v.Status["4xx"], v.Status["5xx"], v.Window)
		if !v.OldestKept.IsZero() {
			_, _ = fmt.Fprintf(stdout, "data since %s\n", v.OldestKept.UTC().Format("2006-01-02 15:04 UTC"))
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		if len(v.TopPaths) > 0 {
			_, _ = fmt.Fprintln(tw, "\nPAGE\tREQUESTS")
			for _, p := range v.TopPaths {
				_, _ = fmt.Fprintf(tw, "%s\t%d\n", p.Label, p.Count)
			}
		}
		if len(v.TopRefs) > 0 {
			_, _ = fmt.Fprintln(tw, "\nREFERRER\tREQUESTS")
			for _, p := range v.TopRefs {
				_, _ = fmt.Fprintf(tw, "%s\t%d\n", p.Label, p.Count)
			}
		}
		_ = tw.Flush()
	})
}
