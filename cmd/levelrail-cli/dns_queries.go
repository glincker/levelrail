package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runDNSQueries prints recent mesh DNS queries (beta, needs dns.query_log_enabled).
func runDNSQueries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app, qname, rcode, since string
	var limit int
	c, code, ok := parseDBAccess(prog, "dns queries", "dns queries [--app NAME] [--qname PREFIX] [--rcode CODE] [--since 15m] [--limit N]", args, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "only queries from this app")
		fs.StringVar(&qname, "qname", "", "only names starting with this prefix")
		fs.StringVar(&rcode, "rcode", "", "only this response code, such as NXDOMAIN or REFUSED")
		fs.StringVar(&since, "since", "", "only queries newer than a duration (15m) or an RFC 3339 time")
		fs.IntVar(&limit, "limit", 0, "most records to return (server default when 0)")
	})
	if !ok {
		return code
	}
	q := url.Values{}
	for k, v := range map[string]string{"app": app, "qname": qname, "rcode": rcode, "since": since} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	res, err := c.client.DNSQueries(context.Background(), q)
	if err != nil {
		return c.fail(fmt.Errorf("dns queries: %w", err))
	}
	return c.result(res, func() { printDNSQueries(stdout, res) })
}

func printDNSQueries(w io.Writer, r apiclient.DNSQueriesResult) {
	if !r.Enabled {
		_, _ = fmt.Fprintln(w, "dns query log is off (set dns.query_log_enabled to true, beta)")
		return
	}
	if len(r.Queries) == 0 {
		_, _ = fmt.Fprintf(w, "no queries recorded (privacy: %s)\n", r.Privacy)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIME\tAPP\tNAME\tTYPE\tRCODE\tANSWERS\tREASON\tLATENCY")
	for _, q := range r.Queries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%dus\n", q.Time.Format("15:04:05"), dashIfEmpty(q.App), q.QName, q.QType, q.RCode, q.Answers, q.Reason, q.LatencyMicros)
	}
	_ = tw.Flush()
}

// runDNSStats prints per-app mesh DNS counters (beta).
func runDNSStats(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "dns stats", "dns stats", args, 0, "", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := c.client.DNSStats(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("dns stats: %w", err))
	}
	return c.result(res, func() { printDNSStats(stdout, res) })
}

func printDNSStats(w io.Writer, r apiclient.DNSStatsResult) {
	if !r.Enabled {
		_, _ = fmt.Fprintln(w, "dns query log is off (set dns.query_log_enabled to true, beta)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tQUERIES\tNXDOMAIN\tREFUSED")
	for _, a := range append(r.Apps, r.Total) {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n", a.App, a.Queries, a.NXDomain, a.Refused)
	}
	_ = tw.Flush()
}
