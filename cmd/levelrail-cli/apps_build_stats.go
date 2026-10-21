package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsBuildStatsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps build-stats <name> [--limit N]     build duration trend (p50, p95) and cache hits for the last N builds
  %[1]s apps build-limits show [--app NAME]     show the build timeout, memory, CPU and concurrency limits in effect
  %[1]s apps build-limits set [--app NAME] [--timeout 30m] [--memory-mb N] [--cpus 1.5] [--max-concurrent N]
                                                override the env defaults, for one app with --app (0 means unlimited)
  %[1]s apps build-limits reset [--app NAME]    drop the overrides and use the APP_BUILD_* defaults

Percentiles cover successful builds only. Failed, timed out and canceled builds are counted separately.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsBuildStats(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var limit int
	c, code, ok := parseCLICall(prog, "apps build-stats", appsBuildStatsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&limit, "limit", 20, "how many recent builds to summarise (1 to 500)")
	})
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected exactly one app name")
	}
	app := c.fs.Args()[0]
	s, err := c.client.GetBuildStats(context.Background(), app, limit)
	if err != nil {
		return c.fail(fmt.Errorf("get build stats: %w", err))
	}
	return c.render(s, func() { printBuildStats(stdout, s) })
}

func printBuildStats(w io.Writer, s apiclient.BuildStats) {
	t := s.Trend
	if t.Builds == 0 {
		_, _ = fmt.Fprintf(w, "no builds recorded for %s yet\n", s.App)
		return
	}
	_, _ = fmt.Fprintf(w, "app:      %s (last %d builds)\nbuilds:   %d ok, %d failed, %d timed out, %d canceled\n",
		s.App, t.Builds, t.Succeeded-t.Skewed, t.Failed, t.TimedOut, t.Canceled)
	if t.Succeeded-t.Skewed > 0 {
		_, _ = fmt.Fprintf(w, "duration: p50 %s, p95 %s (min %s, max %s)\n",
			fmtSeconds(t.P50Seconds), fmtSeconds(t.P95Seconds), fmtSeconds(t.MinSeconds), fmtSeconds(t.MaxSeconds))
	}
	if t.CacheKnown {
		_, _ = fmt.Fprintf(w, "cache:    %.0f%% of steps cached\n", t.CacheHitRatio*100)
	}
	if t.Skewed > 0 {
		_, _ = fmt.Fprintf(w, "skipped:  %d build(s) with a negative duration (clock skew)\n", t.Skewed)
	}
	_, _ = fmt.Fprintln(w, "\nstarted               status    duration  cached")
	for _, r := range s.Runs {
		cached := "-"
		if r.TotalSteps > 0 {
			cached = fmt.Sprintf("%d/%d", r.CachedSteps, r.TotalSteps)
		}
		_, _ = fmt.Fprintf(w, "%-21s %-9s %-9s %s\n", r.StartedAt.UTC().Format("2006-01-02T15:04:05Z"), r.Status, fmtSeconds(r.DurationSeconds), cached)
	}
}

func fmtSeconds(s float64) string {
	return (time.Duration(s * float64(time.Second))).Round(100 * time.Millisecond).String()
}

func runAppsBuildLimits(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBuildStatsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBuildStatsUsage(prog))
		return exitOK
	case "show":
		return runBuildLimitsShow(prog, rest, stdout, stderr, lookupEnv)
	case "set":
		return runBuildLimitsSet(prog, rest, stdout, stderr, lookupEnv)
	case "reset":
		return runBuildLimitsReset(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps build-limits subcommand %q\n\n%s", prog, args[0], appsBuildStatsUsage(prog))
		return exitUsage
	}
}

func runBuildLimitsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app string
	c, code, ok := parseCLICall(prog, "apps build-limits show", appsBuildStatsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "show one app's limits instead of the global ones")
	})
	if !ok {
		return code
	}
	if app != "" {
		l, err := c.client.GetAppBuildLimits(context.Background(), app)
		if err != nil {
			return c.fail(fmt.Errorf("get app build limits: %w", err))
		}
		return c.render(l, func() { printBuildLimitValues(stdout, l.Effective, l.Overrides) })
	}
	l, err := c.client.GetBuildLimits(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("get build limits: %w", err))
	}
	return c.render(l, func() { printBuildLimits(stdout, l) })
}

func printBuildLimits(w io.Writer, l apiclient.BuildLimits) {
	printBuildLimitValues(w, l.Effective, l.Overrides)
}

func printBuildLimitValues(w io.Writer, v apiclient.BuildLimitValues, ov apiclient.BuildLimitOverrides) {
	src := func(overridden bool) string {
		if overridden {
			return "override"
		}
		return "default"
	}
	cpus := "unlimited"
	if v.CPUMillis > 0 {
		cpus = fmt.Sprintf("%.2f cores", float64(v.CPUMillis)/1000)
	}
	mem := "unlimited"
	if v.MemoryMB > 0 {
		mem = fmt.Sprintf("%d MB", v.MemoryMB)
	}
	_, _ = fmt.Fprintf(w, "timeout:        %s (%s)\nmemory:         %s (%s)\ncpu:            %s (%s)\nmax concurrent: %s (%s)\n",
		buildLimitLabel(v.TimeoutSeconds, true), src(ov.TimeoutSeconds != nil),
		mem, src(ov.MemoryMB != nil),
		cpus, src(ov.CPUMillis != nil),
		buildLimitLabel(v.MaxConcurrent, false), src(ov.MaxConcurrent != nil))
}

func buildLimitLabel(n int, seconds bool) string {
	switch {
	case n == 0:
		return "unlimited"
	case seconds:
		return (time.Duration(n) * time.Second).String()
	default:
		return fmt.Sprint(n)
	}
}

func runBuildLimitsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var (
		timeout time.Duration
		maxConc int
		memory  int
		cpus    float64
		app     string
	)
	c, code, ok := parseCLICall(prog, "apps build-limits set", appsBuildStatsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.DurationVar(&timeout, "timeout", 0, "per-build time limit, for example 30m (0 means unlimited)")
		fs.IntVar(&maxConc, "max-concurrent", 0, "builds allowed to run at once (0 means unlimited)")
		fs.IntVar(&memory, "memory-mb", 0, "memory one build may use, in MB (0 means unlimited)")
		fs.Float64Var(&cpus, "cpus", 0, "CPU cores one build may use, for example 1.5 (0 means unlimited)")
		fs.StringVar(&app, "app", "", "apply to one app instead of globally")
	})
	if !ok {
		return code
	}
	var req apiclient.BuildLimitsRequest
	c.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "timeout":
			s := int(timeout.Seconds())
			req.TimeoutSeconds = &s
		case "max-concurrent":
			req.MaxConcurrent = &maxConc
		case "memory-mb":
			req.MemoryMB = &memory
		case "cpus":
			m := int(cpus*1000 + 0.5)
			req.CPUMillis = &m
		}
	})
	if req.TimeoutSeconds == nil && req.MaxConcurrent == nil && req.MemoryMB == nil && req.CPUMillis == nil {
		return c.invalid("set at least one of --timeout, --memory-mb, --cpus, --max-concurrent")
	}
	if timeout < 0 || maxConc < 0 || memory < 0 || cpus < 0 {
		return c.invalid("values must not be negative")
	}
	if app != "" {
		if req.MaxConcurrent != nil {
			return c.invalid("--max-concurrent is global; drop --app to set it")
		}
		l, err := c.client.SetAppBuildLimits(context.Background(), app, req)
		if err != nil {
			return c.fail(fmt.Errorf("set app build limits: %w", err))
		}
		return c.render(l, func() { printBuildLimitValues(stdout, l.Effective, l.Overrides) })
	}
	l, err := c.client.SetBuildLimits(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("set build limits: %w", err))
	}
	return c.render(l, func() { printBuildLimits(stdout, l) })
}

func runBuildLimitsReset(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app string
	c, code, ok := parseCLICall(prog, "apps build-limits reset", appsBuildStatsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "reset one app's overrides instead of the global ones")
	})
	if !ok {
		return code
	}
	if app != "" {
		l, err := c.client.ResetAppBuildLimits(context.Background(), app)
		if err != nil {
			return c.fail(fmt.Errorf("reset app build limits: %w", err))
		}
		return c.render(l, func() { printBuildLimitValues(stdout, l.Effective, l.Overrides) })
	}
	l, err := c.client.ResetBuildLimits(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("reset build limits: %w", err))
	}
	return c.render(l, func() { printBuildLimits(stdout, l) })
}
