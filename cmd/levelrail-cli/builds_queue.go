package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runBuildsQueue implements "builds queue": the queue state, running and waiting builds.
func runBuildsQueue(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseBuildsCall(prog, "builds queue", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	q, err := c.client.GetBuildQueue(bg())
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds queue: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, q, func() { printBuildQueue(stdout, q) })
}

func printBuildQueue(stdout io.Writer, q apiclient.BuildQueue) {
	line := "Build queue: " + q.State
	if q.State != "running" {
		if q.Actor != "" {
			line += " by " + q.Actor
		}
		if q.Since != nil {
			line += " since " + q.Since.Local().Format(time.RFC3339)
		}
		if q.Reason != "" {
			line += " (" + q.Reason + ")"
		}
	}
	_, _ = fmt.Fprintln(stdout, line)
	_, _ = fmt.Fprintf(stdout, "Slots: %d of %d in use\n", q.Slots.Used, q.Slots.Total)
	for _, r := range q.Running {
		_, _ = fmt.Fprintf(stdout, "  running  %-24s %-10s %s\n", r.App, r.Phase, (time.Duration(r.ElapsedSeconds) * time.Second).String())
	}
	for _, w := range q.Queued {
		_, _ = fmt.Fprintf(stdout, "  #%-7d %-24s %s\n", w.Position, w.App, w.WaitReason)
	}
}

// runBuildsQueueChange implements "builds pause|resume|drain".
func runBuildsQueueChange(prog, action string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var reason string
	c, code, ok := parseBuildsCall(prog, "builds "+action, args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		if action != "resume" {
			fs.StringVar(&reason, "reason", "", "why the queue is held, shown to everyone")
		}
	})
	if !ok {
		return code
	}
	q, err := c.client.SetBuildQueue(bg(), action, strings.TrimSpace(reason))
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds %s: %w", action, err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, q, func() { printBuildQueue(stdout, q) })
}

// runBuildsKillAll implements "builds kill-all": cancels every queued and running build.
func runBuildsKillAll(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var reason string
	var yes, pause bool
	c, code, ok := parseBuildsCall(prog, "builds kill-all", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&reason, "reason", "", "why every build is canceled")
		fs.BoolVar(&yes, "yes", false, "confirm canceling every queued and running build")
		fs.BoolVar(&pause, "pause", false, "also pause the queue so no new build starts")
	})
	if !ok {
		return code
	}
	if !yes {
		return usageError(stderr, prog, "builds kill-all cancels every queued and running build; pass --yes to confirm")
	}
	res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.KillAllBuildsResult, error) {
		return c.client.KillAllBuilds(bg(), strings.TrimSpace(reason), pause, p)
	})
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds kill-all: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "Canceled %d builds\n", len(res.Canceled))
		for _, it := range res.Canceled {
			_, _ = fmt.Fprintf(stdout, "  %s  %s (%s)\n", it.DeployID, it.App, it.Was)
		}
		for _, it := range res.Failed {
			_, _ = fmt.Fprintf(stdout, "  not canceled %s  %s: %s\n", it.DeployID, it.App, it.Error)
		}
		if res.Paused {
			_, _ = fmt.Fprintln(stdout, "The build queue is paused; run builds resume to start builds again.")
		}
	})
}
