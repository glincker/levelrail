package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	timelineBarWidth = 40
	timelineMaxPages = 10
)

// runAppsDeploysTimeline implements "apps deploys timeline", the fleet-wide
// time-axis view of deploys.
func runAppsDeploysTimeline(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys timeline", "print the timeline page as JSON to stdout and nothing else", stderr)
	var since, app, project, node, status string
	var limit int
	fs.StringVar(&since, "since", "24h", "window start: a duration such as 1h, 24h or 7d (at most 30d)")
	fs.StringVar(&app, "app", "", "only this app")
	fs.StringVar(&project, "project", "", "only apps in this project ID")
	fs.StringVar(&node, "node", "", "only apps placed on this node ID")
	fs.StringVar(&status, "status", "", "only these statuses, comma separated (for example failed,rolled_back)")
	fs.IntVar(&limit, "limit", 200, "most deploys to fetch across pages")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysTimelineUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys timeline takes no arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if limit < 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--limit must be a positive integer"))
	}

	q := url.Values{"since": {since}, "limit": {strconv.Itoa(min(limit, 200))}}
	for k, v := range map[string]string{"app": app, "project": project, "node": node, "status": status} {
		if v != "" {
			q.Set(k, v)
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	page, err := fetchTimeline(context.Background(), client, q, limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy timeline: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, page, func() { printTimeline(stdout, page) })
}

func fetchTimeline(ctx context.Context, client *apiclient.Client, q url.Values, limit int) (apiclient.TimelinePage, error) {
	var all apiclient.TimelinePage
	for i := 0; i < timelineMaxPages; i++ {
		page, err := client.DeployTimeline(ctx, q)
		if err != nil {
			return all, err
		}
		all.Since, all.Until = page.Since, page.Until
		all.Items = append(all.Items, page.Items...)
		all.NextCursor = page.NextCursor
		if page.NextCursor == "" || len(all.Items) >= limit {
			break
		}
		q.Set("cursor", page.NextCursor)
	}
	if len(all.Items) > limit {
		all.Items = all.Items[:limit]
	}
	return all, nil
}

func timelineGlyph(status string) byte {
	switch status {
	case "ready":
		return '='
	case "failed":
		return 'x'
	case "building", "queued", "awaiting_approval":
		return '~'
	case "rolled_back":
		return '<'
	}
	return '-'
}

// timelineBar draws the deploy as a run of glyphs placed on a fixed-width
// scale of the window; an open deploy runs to the window end.
func timelineBar(d apiclient.TimelineDeploy, since, until time.Time) string {
	span := until.Sub(since)
	if span <= 0 {
		return strings.Repeat(" ", timelineBarWidth)
	}
	pos := func(t time.Time) int {
		p := int(float64(t.Sub(since)) / float64(span) * float64(timelineBarWidth))
		return max(0, min(timelineBarWidth-1, p))
	}
	end := until
	if d.FinishedAt != nil {
		end = *d.FinishedAt
	}
	from, to := pos(d.StartedAt), pos(end)
	b := []byte(strings.Repeat(" ", timelineBarWidth))
	for i := from; i <= to; i++ {
		b[i] = timelineGlyph(d.Status)
	}
	return string(b)
}

func printTimeline(out io.Writer, page apiclient.TimelinePage) {
	if len(page.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no deploys in this window")
		return
	}
	items := append([]apiclient.TimelineDeploy(nil), page.Items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].StartedAt.Before(items[j].StartedAt) })
	_, _ = fmt.Fprintf(out, "%s -> %s  (= ready, x failed, ~ in flight, < rolled back)\n",
		page.Since.Local().Format("Jan 02 15:04"), page.Until.Local().Format("Jan 02 15:04"))
	for _, d := range items {
		dur := "-"
		if d.DurationMS != nil {
			dur = (time.Duration(*d.DurationMS) * time.Millisecond).Round(time.Second).String()
		}
		sha := d.CommitSHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		_, _ = fmt.Fprintf(out, "%s |%s| %-12s %-11s %6s %-7s %s\n",
			d.StartedAt.Local().Format("15:04"), timelineBar(d, page.Since, page.Until),
			d.App, d.Status, dur, dashIfEmpty(sha), d.ID)
	}
	if page.NextCursor != "" {
		_, _ = fmt.Fprintf(out, "more deploys exist in this window; raise --limit or narrow --since\n")
	}
}

func appsDeploysTimelineUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys timeline [--since 24h] [--app NAME] [--project ID] [--node ID] [--status LIST] [flags]

Shows every deploy across the fleet as a bar on a shared time axis, oldest
first. In-flight deploys run to the window end.

Flags:
  --since string          window start: 1h, 24h or 7d, at most 30d (default 24h)
  --app string            only this app
  --project string        only apps in this project ID
  --node string           only apps placed on this node ID
  --status string         only these statuses, comma separated
  --limit int             most deploys to fetch (default 200)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the timeline page as JSON to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
