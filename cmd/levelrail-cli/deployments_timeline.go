package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	eventKindLog      = "log"
	eventStatusFailed = "failed"
	msgSnapshot       = "snapshot"
	msgEvent          = "event"
	msgEnd            = "end"
)

var errTimelineEnd = errors.New("deploy timeline ended")

func timelineBad(verdict string) bool {
	return verdict == "failed" || verdict == "rolled_back"
}

func formatTimelineEvent(e apiclient.DeployEvent) string {
	line := fmt.Sprintf("%s  %-8s %-26s %s", e.At.Local().Format("15:04:05"), e.Status, e.Code, e.Title)
	if e.DurationMS > 0 {
		line += " (" + (time.Duration(e.DurationMS) * time.Millisecond).Round(time.Millisecond).String() + ")"
	}
	if e.Reason != "" {
		line += " [" + e.Reason + "]"
	}
	if e.Detail != "" && e.Kind != eventKindLog {
		line += ": " + e.Detail
	}
	if e.Impact != nil {
		line += ": " + formatImpact(*e.Impact)
	}
	if e.Kind == eventKindLog {
		return fmt.Sprintf("%s  %-8s   | %s", e.At.Local().Format("15:04:05"), e.Stream, e.Detail)
	}
	return line
}

func formatImpact(im apiclient.DeployEventImpact) string {
	side := func(w apiclient.DeployEventWindow) string {
		if !w.HasTraffic {
			return "no traffic"
		}
		return fmt.Sprintf("%.2f req/s, %.1f%% 5xx, p95 %.0fms", w.RatePerSec, w.ErrorRate5xx*100, w.P95Ms)
	}
	suffix := ""
	if !im.Complete {
		suffix = " (window still open)"
	}
	return "before " + side(im.Before) + " | after " + side(im.After) + suffix
}

// timelineWriter prints events and folds build log lines into a count unless
// the operator asked for them.
type timelineWriter struct {
	out      io.Writer
	jsonOut  bool
	showLogs bool
	hidden   int
}

func (w *timelineWriter) event(e apiclient.DeployEvent) error {
	if w.jsonOut {
		return writeJSONLine(w.out, e)
	}
	if e.Kind == eventKindLog && !w.showLogs {
		w.hidden++
		return nil
	}
	_, err := fmt.Fprintln(w.out, formatTimelineEvent(e))
	return err
}

func (w *timelineWriter) finish(verdict string) {
	if w.jsonOut {
		return
	}
	if w.hidden > 0 {
		_, _ = fmt.Fprintf(w.out, "%d build log lines hidden, rerun with --logs to show them\n", w.hidden)
	}
	if verdict != "" {
		_, _ = fmt.Fprintf(w.out, "verdict: %s\n", verdict)
	}
}

func runDeploymentsTimeline(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "deployments timeline", "print JSON instead of text: one object per event when following", stderr)
	var follow, logs bool
	var compare string
	fs.BoolVar(&follow, "follow", false, "keep streaming until the deploy settles")
	fs.BoolVar(&logs, "logs", false, "include build log lines")
	fs.StringVar(&compare, "compare", "", "another deploy id to compare phase timings and signals against")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deploymentsTimelineUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "deployments timeline", "a deploy id", 1)
	if !ok {
		return exitUsage
	}
	id := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if compare != "" {
		cmp, err := client.CompareDeployEventTimelines(ctx, id, compare)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("compare deploy timelines: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, cmp, func() { printTimelineComparison(stdout, cmp) })
	}
	if follow {
		return followDeployTimeline(ctx, client, id, &timelineWriter{out: stdout, jsonOut: jsonOut, showLogs: logs}, stdout, stderr)
	}
	tl, err := client.DeployEventTimeline(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy timeline: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, tl, func() {
		w := &timelineWriter{out: stdout, showLogs: logs}
		for _, e := range tl.Events {
			_ = w.event(e)
		}
		w.finish(tl.Verdict)
	})
}

func followDeployTimeline(ctx context.Context, client *apiclient.Client, id string, w *timelineWriter, stdout, stderr io.Writer) int {
	verdict := ""
	err := client.StreamDeployEventTimeline(ctx, id, func(m apiclient.DeployEventMessage) error {
		if m.Verdict != "" {
			verdict = m.Verdict
		}
		switch m.Type {
		case msgSnapshot:
			if m.Timeline != nil {
				for _, e := range m.Timeline.Events {
					if err := w.event(e); err != nil {
						return err
					}
				}
			}
		case msgEvent:
			if m.Event != nil {
				return w.event(*m.Event)
			}
		case msgEnd:
			return errTimelineEnd
		}
		return nil
	})
	if ctx.Err() != nil {
		return exitOK
	}
	if err != nil && !errors.Is(err, errTimelineEnd) {
		return reportError(stdout, stderr, w.jsonOut, fmt.Errorf("follow deploy timeline for %s: %w", id, err))
	}
	w.finish(verdict)
	if timelineBad(verdict) {
		return exitAPIError
	}
	return exitOK
}

func printTimelineComparison(out io.Writer, c apiclient.DeployEventComparison) {
	_, _ = fmt.Fprintf(out, "A %s (%s)   B %s (%s)\n", c.A.DeployID, c.A.Verdict, c.B.DeployID, c.B.Verdict)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PHASE\tA\tB\tDELTA")
	ms := func(v *int64) string {
		if v == nil {
			return "-"
		}
		return (time.Duration(*v) * time.Millisecond).Round(time.Millisecond).String()
	}
	for _, p := range c.Phases {
		delta := "-"
		if p.DeltaMS != nil {
			delta = fmt.Sprintf("%+dms", *p.DeltaMS)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Phase, ms(p.AMS), ms(p.BMS), delta)
	}
	_ = tw.Flush()
	if len(c.OnlyA) > 0 {
		_, _ = fmt.Fprintf(out, "only in A: %s\n", strings.Join(c.OnlyA, ", "))
	}
	if len(c.OnlyB) > 0 {
		_, _ = fmt.Fprintf(out, "only in B: %s\n", strings.Join(c.OnlyB, ", "))
	}
}

func deploymentsTimelineUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deployments timeline <deploy-id> [--follow] [--logs] [--compare OTHER-ID] [flags]

Shows one deploy as an ordered story: the push or webhook that started it,
build and push steps, hooks, readiness probes, cutover, post-deploy watch
signals, rollback or success, and request health before and after cutover.
Find a deploy id with "%[1]s deployments list".

With --follow it streams until the deploy settles and exits non-zero when the
deploy failed or was rolled back. With --compare it prints per-phase timings
and the signals only one of the two deploys had.

Flags:
  --follow                keep streaming until the deploy settles
  --logs                  include build log lines (hidden by default)
  --compare string        another deploy id to compare against
  --token, --api-url, --profile, --json, --output, --query, -h
`, prog)
}
