package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsDeploysStages implements "apps deploys stages <name> <deploy-id>",
// also reached as "apps deploys steps ... --stages".
func runAppsDeploysStages(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys stages", "print the stages as JSON to stdout and nothing else", stderr)
	tail := fs.String("tail", "", "stream only this stage's log lines (source, detect, install, build, export, push)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps deploys stages <name> <deploy-id> [--tail STAGE] [flags]\n\nShows the build stages of one deploy (source, detect, install, build, export,\npush) with start, duration and cache state. --tail follows one stage's log.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys stages", "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if *tail != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err := client.StreamDeployStageLog(ctx, rest[0], rest[1], *tail, func(e apiclient.LogStreamEntry) error {
			if jsonOut {
				return writeJSONLine(stdout, e)
			}
			_, werr := fmt.Fprintln(stdout, e.Line)
			return werr
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("tail stage %s of %s/%s: %w", *tail, rest[0], rest[1], err))
		}
		return exitOK
	}

	res, err := client.GetDeployStages(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get build stages for app %q deploy %q: %w", rest[0], rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDeployStagesHuman(stdout, res) })
}

// withoutStagesFlag removes a bare --stages from args and reports whether it was there.
func withoutStagesFlag(args []string) ([]string, bool) {
	var out []string
	found := false
	for _, a := range args {
		if a == "--stages" || a == "-stages" {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

func printDeployStagesHuman(out io.Writer, res apiclient.DeployStages) {
	if len(res.Stages) == 0 {
		_, _ = fmt.Fprintln(out, "no build stages recorded")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STAGE\tSTATUS\tSTARTED\tDURATION\tCACHE\tLOG")
	for _, s := range res.Stages {
		cache := "-"
		if s.Cache != "" {
			cache = fmt.Sprintf("%s (%d/%d)", s.Cache, s.CachedVertices, s.Vertices)
		}
		dur := (time.Duration(s.DurationMS) * time.Millisecond).Round(time.Millisecond).String()
		if s.EndedAt == nil {
			dur = "running"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d B\n", s.Stage, s.Status, formatTimelineTime(s.StartedAt.UTC().Format(time.RFC3339)), dur, cache, s.LogBytes)
	}
	_ = tw.Flush()
	wall := (time.Duration(res.WallMS) * time.Millisecond).Round(time.Millisecond)
	sum := (time.Duration(res.StagesMS) * time.Millisecond).Round(time.Millisecond)
	_, _ = fmt.Fprintf(out, "stages total %s, deploy elapsed %s\n", sum, wall)
}
