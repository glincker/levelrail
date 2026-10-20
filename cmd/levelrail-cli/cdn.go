package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func cdnUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cdn overview [--window 5m|1h|24h|7d]

Fleet traffic from ingress metrics, edge cache hit rate and the share of requests per node.
--json prints one object with window, requests, rate_per_sec, error_rate_5xx, bytes_out, cache and nodes.
`, prog)
}

func runCDN(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var window string
	cmd := apiCmd{
		label: "cdn overview",
		usage: cdnUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&window, "window", "1h", "time window: 5m, 1h, 24h or 7d")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			o, err := c.GetCDNOverview(ctx, window)
			return o, func(w io.Writer) { printCDNOverview(w, o) }, err
		},
	}
	return dispatchSub(prog, "cdn", cdnUsage, map[string]apiCmd{"overview": cmd}, args, stdout, stderr, lookupEnv)
}

func printCDNOverview(out io.Writer, o apiclient.CDNOverview) {
	if !o.HasTraffic {
		_, _ = fmt.Fprintf(out, "no traffic in the last %s\n", o.Window)
		return
	}
	_, _ = fmt.Fprintf(out, "window %s: %.0f requests (%.2f/s), 5xx %.2f%%, cache hit rate %.1f%%\n", o.Window, o.Requests, o.RatePerSec, o.ErrorRate5xx*100, o.Cache.HitRate*100)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tREGION\tREQUESTS\tSHARE\tAPPS")
	for _, n := range o.Nodes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%.0f\t%.1f%%\t%d\n", n.Name, n.Region, n.Requests, n.Share*100, n.Apps)
	}
	_ = tw.Flush()
}
