package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func cdnSetupCmd() apiCmd {
	var provider string
	var dryRun bool
	return apiCmd{
		label: "cdn setup",
		usage: cdnUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&provider, "provider", "cloudflare", "CDN provider")
			fs.BoolVar(&dryRun, "dry-run", false, "run the checks and preview DNS changes without applying")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			res, err := c.SetupAppCDN(ctx, pos[0], provider, dryRun)
			return res, func(w io.Writer) { printCDNSetup(w, res) }, err
		},
	}
}

func cdnEdgeCmd() apiCmd {
	return apiCmd{
		label: "cdn edge",
		usage: cdnUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			res, err := c.GetAppEdge(ctx, pos[0])
			return res, func(w io.Writer) { printAppEdge(w, res) }, err
		},
	}
}

func printCDNSetup(out io.Writer, r apiclient.CDNSetup) {
	for _, s := range r.Steps {
		_, _ = fmt.Fprintf(out, "[%s] %s: %s\n", s.Status, s.ID, s.Message)
		if s.Fix != "" {
			_, _ = fmt.Fprintf(out, "       fix: %s\n", s.Fix)
		}
	}
	_, _ = fmt.Fprintf(out, "result: %s\n", r.Status)
}

func printAppEdge(out io.Writer, e apiclient.AppEdge) {
	mode := "direct"
	if e.Proxied {
		mode = "proxied"
	}
	_, _ = fmt.Fprintf(out, "%s: %s (%s), provider %s, zone %s, SSL %s\n", e.App, mode, e.Health, orDash(e.Provider), orDash(e.Zone), orDash(e.SSLMode))
	_, _ = fmt.Fprintf(out, "cache hit rate %.1f%% (%d hits, %d misses), %d DNS record(s) managed\n", e.Cache.HitRate*100, e.Cache.Hits, e.Cache.Misses, e.DNS.Managed)
	if p := e.LastPurge; p != nil {
		result := "ok"
		if !p.OK {
			result = "failed: " + p.Error
		}
		_, _ = fmt.Fprintf(out, "last purge %s on %s, %d host(s): %s\n", p.At.Format("2006-01-02 15:04:05"), p.Trigger, p.Hosts, result)
	}
	for _, issue := range e.Issues {
		_, _ = fmt.Fprintf(out, "attention: %s\n", issue)
	}
}
