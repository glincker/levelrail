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

func runAppsAccess(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var o apiclient.AccessOptions
	label := "apps access"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n\nRecent edge requests for an app, newest first, kept in memory on the control plane.\n\nFlags:\n  --slow-ms float    only requests at least this slow\n  --min-status int   only responses with at least this status (500 for errors)\n  --path string      only paths starting with this prefix\n  --since string     RFC3339 start\n  --limit int        at most this many (default 100)\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.Float64Var(&o.MinMillis, "slow-ms", 0, "minimum duration in milliseconds")
		fs.IntVar(&o.MinStatus, "min-status", 0, "minimum response status")
		fs.StringVar(&o.Path, "path", "", "path prefix")
		fs.StringVar(&o.Since, "since", "", "RFC3339 start")
		fs.IntVar(&o.Limit, "limit", 0, "maximum records")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	recs, err := c.client.ListAccess(context.Background(), app, o)
	if err != nil {
		return c.fail(fmt.Errorf("list access records for %q: %w", app, err))
	}
	return c.render(recs, func() {
		if len(recs) == 0 {
			_, _ = fmt.Fprintln(stdout, "no matching requests")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TIME\tSTATUS\tMS\tMETHOD\tPATH\tREQUEST ID\tTRACE ID")
		for _, r := range recs {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%.0f\t%s\t%s\t%s\t%s\n", r.Time.UTC().Format(time.RFC3339), r.Status, r.Millis, r.Method, r.Path, r.RequestID, r.TraceID)
		}
		_ = tw.Flush()
	})
}
