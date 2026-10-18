package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func buildsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s builds list [--app A] [--status S] [--limit N]
  %[1]s builds show <id>
  %[1]s builds cancel <id>
  %[1]s builds retry <id>
  %[1]s builds cache [--node N]
  %[1]s builds prune [--node N] [--older-than-hours H] [--apply]

The build center: queued, running and finished builds across every app with
duration, cache hit ratio, image size change and the node that ran each one.
retry rebuilds the connected branch of a failed or canceled build. prune
previews by default and only removes idle BuildKit cache with --apply; cache
an in-flight build holds is never removed.
`, prog)
}

// runBuilds implements "builds": the build queue, history and build cache.
func runBuilds(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, buildsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, buildsUsage(prog))
		return exitOK
	case "list":
		return runBuildsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runBuildsShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "cancel":
		return runBuildsAction(prog, "cancel", args[1:], stdout, stderr, lookupEnv)
	case "retry":
		return runBuildsAction(prog, "retry", args[1:], stdout, stderr, lookupEnv)
	case "cache":
		return runBuildsCache(prog, args[1:], stdout, stderr, lookupEnv)
	case "prune":
		return runBuildsPrune(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown builds subcommand %q\n\n%s", prog, args[0], buildsUsage(prog))
	return exitUsage
}

type buildsCall struct {
	client *Client
	jsonO  bool
	of     outputFlags
	rest   []string
}

func parseBuildsCall(prog, label string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), define func(*flag.FlagSet)) (buildsCall, int, bool) {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	if define != nil {
		define(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, buildsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return buildsCall{}, code, false
	}
	return buildsCall{client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), jsonO: jsonOut, of: of, rest: fs.Args()}, exitOK, true
}

func buildDuration(b apiclient.BuildResource) string {
	if b.DurationSeconds == nil {
		return "-"
	}
	return (time.Duration(*b.DurationSeconds * float64(time.Second))).Round(time.Second).String()
}

func buildCache(b apiclient.BuildResource) string {
	if b.CacheHitRatio == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", *b.CacheHitRatio*100)
}

func buildDelta(b apiclient.BuildResource) string {
	if b.ImageBytes == 0 {
		return "-"
	}
	if b.ImageDeltaBytes == nil {
		return humanBytes(b.ImageBytes)
	}
	sign := "+"
	d := *b.ImageDeltaBytes
	if d < 0 {
		sign, d = "-", -d
	}
	return fmt.Sprintf("%s (%s%s)", humanBytes(b.ImageBytes), sign, humanBytes(d))
}

func buildNodeLabel(id string) string {
	if id == "" {
		return "local"
	}
	return id
}

func runBuildsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app, status string
	var limit int
	c, code, ok := parseBuildsCall(prog, "builds list", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "only this app")
		fs.StringVar(&status, "status", "", "only this status: queued, running, succeeded, failed, canceled")
		fs.IntVar(&limit, "limit", 0, "most recent builds to load (default 100, max 500)")
	})
	if !ok {
		return code
	}
	res, err := c.client.ListBuilds(bg(), app, status, limit)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("list builds: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%d queued, %d running, %d finished (%d failed). Up to %d at once per node, target %s.\n",
			res.Counts.Queued, res.Counts.Running, res.Counts.Finished, res.Counts.Failed, res.Limit, res.Platforms.Host)
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tAPP\tSTATUS\tDURATION\tCACHE\tIMAGE\tNODE\tPLATFORM")
		for _, b := range res.Rows {
			st := b.Status
			if b.QueuePosition > 0 {
				st = fmt.Sprintf("queued #%d", b.QueuePosition)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", b.ID, b.App, st, buildDuration(b), buildCache(b), buildDelta(b), buildNodeLabel(b.Node), buildOrDash(b.Platform))
		}
		_ = tw.Flush()
	})
}

func buildOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func runBuildsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseBuildsCall(prog, "builds show", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.rest) != 1 {
		return usageError(stderr, prog, "builds show takes one build ID")
	}
	b, err := c.client.GetBuild(bg(), c.rest[0])
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("show build: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, b, func() {
		_, _ = fmt.Fprintf(stdout, "%s  %s  %s\n", b.ID, b.App, b.Status)
		_, _ = fmt.Fprintf(stdout, "duration %s, cache hit %s, image %s, node %s, platform %s\n", buildDuration(b), buildCache(b), buildDelta(b), buildNodeLabel(b.Node), buildOrDash(b.Platform))
		_, _ = fmt.Fprintf(stdout, "logs: %s apps deploys logs %s %s\n", prog, b.App, b.ID)
		if b.Error != "" {
			_, _ = fmt.Fprintf(stdout, "error: %s\n", b.Error)
		}
		if b.Hint != nil {
			_, _ = fmt.Fprintf(stdout, "hint: %s. %s %s\n", b.Hint.Title, b.Hint.Explanation, b.Hint.Suggestion)
		}
	})
}

func runBuildsAction(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseBuildsCall(prog, "builds "+verb, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.rest) != 1 {
		return usageError(stderr, prog, "builds "+verb+" takes one build ID")
	}
	id := c.rest[0]
	b, err := c.client.GetBuild(bg(), id)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds %s: %w", verb, err))
	}
	msg := ""
	if verb == "cancel" {
		err = c.client.CancelBuild(bg(), b.App, id)
	} else {
		msg, err = c.client.RetryBuild(bg(), b.App, id)
	}
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds %s: %w", verb, err))
	}
	res := map[string]string{"id": id, "app": b.App, "action": verb, "message": msg}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s requested for build %s of %s %s\n", verb, id, b.App, msg)
	})
}

func runBuildsCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var node string
	c, code, ok := parseBuildsCall(prog, "builds cache", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&node, "node", "", "node ID (default: the control plane's own node)")
	})
	if !ok {
		return code
	}
	res, err := c.client.GetBuildCache(bg(), node)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("build cache: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		if !res.Supported {
			_, _ = fmt.Fprintf(stdout, "node %s does not report its build cache: %s\n", buildNodeLabel(node), res.Reason)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%d records, %s total, %s in use, %s reclaimable\n", res.RecordCount, humanBytes(res.TotalBytes), humanBytes(res.InUseBytes), humanBytes(res.ReclaimableBytes))
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tTYPE\tSIZE\tUSES\tSTATE\tDESCRIPTION")
		for _, r := range res.Records {
			state := "idle"
			if r.InUse {
				state = "in use"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", shortImageID(r.ID), r.Type, humanBytes(r.SizeBytes), r.UsageCount, state, buildTruncate(strings.TrimSpace(r.Description), 60))
		}
		_ = tw.Flush()
	})
}

func buildTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "..."
}

func runBuildsPrune(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var node string
	hours := -1
	var apply bool
	c, code, ok := parseBuildsCall(prog, "builds prune", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&node, "node", "", "node ID (default: the control plane's own node)")
		fs.IntVar(&hours, "older-than-hours", -1, "only cache idle for at least this long (default 168)")
		fs.BoolVar(&apply, "apply", false, "remove the cache instead of only previewing")
	})
	if !ok {
		return code
	}
	req := apiclient.BuildCachePruneRequest{Node: node, Apply: apply}
	if hours >= 0 {
		req.OlderThanHours = &hours
	}
	res, err := c.client.PruneBuildCache(bg(), req)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("prune build cache: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		if !res.Applied {
			_, _ = fmt.Fprintf(stdout, "%d records idle for %dh or more, %s would be freed. Run with --apply to remove them.\n", res.CandidateCount, res.OlderThanHours, humanBytes(res.WouldFreeBytes))
			return
		}
		_, _ = fmt.Fprintf(stdout, "removed %d records, freed %s\n", res.RemovedCount, humanBytes(res.FreedBytes))
	})
}
