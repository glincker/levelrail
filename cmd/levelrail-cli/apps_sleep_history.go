package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const sleepHistoryDefaultSince = "7d"

func runAppsSleepHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps sleep history", "print the result as JSON to stdout and nothing else", stderr)
	since := fs.String("since", sleepHistoryDefaultSince, "how far back to look, such as 24h or 7d")
	limit := fs.Int("limit", 50, "most events to show")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps sleep history <app-name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps sleep history", "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	h, err := client.GetSleepHistory(context.Background(), appName, *since, *limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("sleep history for app %q: %w", appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, h, func() {
		printSleepHistory(stdout, h, time.Now())
	})
}

func printSleepHistory(out io.Writer, h apiclient.SleepHistory, now time.Time) {
	if len(h.Events) == 0 {
		_, _ = fmt.Fprintln(out, "no sleep events in this range")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WHEN\tEVENT\tREASON\tDURATION/LATENCY\tDETAIL")
	for _, e := range h.Events {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", humanAgo(e.At, now), e.Kind, dashIfEmpty(e.Reason), sleepEventMeasure(e), sleepEventDetail(e))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "wake time: %s\n", wakeTimingText(h.WakeTiming))
}

func sleepEventMeasure(e apiclient.SleepEvent) string {
	var parts []string
	if e.DurationSeconds != nil {
		parts = append(parts, "slept "+(time.Duration(*e.DurationSeconds)*time.Second).String())
	}
	if e.WakeLatencyMs != nil {
		parts = append(parts, "ready in "+(time.Duration(*e.WakeLatencyMs)*time.Millisecond).String())
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func sleepEventDetail(e apiclient.SleepEvent) string {
	var parts []string
	if e.TriggerPath != "" {
		parts = append(parts, e.TriggerPath)
	}
	if e.TriggerSource != "" {
		parts = append(parts, "from "+e.TriggerSource)
	}
	if e.IdleMinutes > 0 {
		parts = append(parts, fmt.Sprintf("after %d min idle", e.IdleMinutes))
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	if e.Actor != "" && e.Actor != "system" {
		parts = append(parts, "by "+e.Actor)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "; ")
}

func printSleepStatusStats(out io.Writer, s apiclient.SleepStats) {
	now := time.Now()
	if s.LastSleptAt != nil {
		_, _ = fmt.Fprintf(out, "slept:   %s\n", humanAgo(*s.LastSleptAt, now))
	}
	if s.LastWokeAt != nil {
		_, _ = fmt.Fprintf(out, "woke:    %s (%s)\n", humanAgo(*s.LastWokeAt, now), dashIfEmpty(s.LastWakeReason))
	}
	_, _ = fmt.Fprintf(out, "asleep:  %s in the last 7 days, %d wakes\n", (time.Duration(s.AsleepSeconds7d) * time.Second).String(), s.WakeCount7d)
}

func humanAgo(t, now time.Time) string {
	d := now.Sub(t).Round(time.Minute)
	if d < time.Minute {
		return "just now"
	}
	if d >= 48*time.Hour {
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm ago", h, m)
	}
	return fmt.Sprintf("%dm ago", m)
}
