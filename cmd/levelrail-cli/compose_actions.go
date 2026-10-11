package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// runComposeLogs prints recent logs for every service, prefixed by its key,
// or hands one service to "apps logs" (which also supports --follow).
func runComposeLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var service, since string
	var follow bool
	var tail int
	cmd, code, ok := parseComposeCmd(prog, "compose logs", "compose logs <app> [--service S] [--since 1h] [--tail N] [-f]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&service, "service", "", "only this service (its compose key)")
		fs.StringVar(&since, "since", "1h", "how far back to read")
		fs.IntVar(&tail, "tail", 50, "last N lines per service")
		fs.BoolVar(&follow, "follow", false, "stream live (needs --service)")
		fs.BoolVar(&follow, "f", false, "shorthand for --follow")
	})
	if !ok {
		return code
	}
	app := cmd.args[0]
	if service != "" {
		return runAppsLogs(prog, forwardServiceLogArgs(args, app, service), stdout, stderr, lookupEnv)
	}
	if follow {
		return reportError(stdout, stderr, cmd.json, newValidationError("--follow needs --service; streaming every service at once is not supported"))
	}
	window, err := time.ParseDuration(since)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, newValidationError("--since: %s", err.Error()))
	}
	ctx := context.Background()
	st, err := cmd.client.GetComposeStack(ctx, app)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose logs %q: %w", app, err))
	}
	now := time.Now()
	type line struct {
		at  time.Time
		svc string
		msg string
	}
	var lines []line
	for _, s := range st.Services {
		entries, err := cmd.client.QueryLogs(ctx, s.Name, now.Add(-window), now, "")
		if err != nil {
			return reportError(stdout, stderr, cmd.json, fmt.Errorf("logs for %s: %w", s.Name, err))
		}
		if tail > 0 && len(entries) > tail {
			entries = entries[len(entries)-tail:]
		}
		for _, e := range entries {
			lines = append(lines, line{at: e.Timestamp, svc: s.Key, msg: e.Message})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].at.Before(lines[j].at) })
	return renderCompose(stdout, stderr, cmd, lines, func() {
		for _, l := range lines {
			_, _ = fmt.Fprintf(stdout, "%-12s | %s %s\n", l.svc, l.at.Format(time.RFC3339), strings.TrimRight(l.msg, "\n"))
		}
	})
}

// forwardServiceLogArgs turns "compose logs <app> --service S ..." into
// "apps logs <app>-S ...", keeping every other flag as given.
func forwardServiceLogArgs(args []string, app, service string) []string {
	out := []string{app + "-" + service}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == app:
		case a == "--service" || a == "-service":
			i++
		case strings.HasPrefix(a, "--service=") || strings.HasPrefix(a, "-service="):
		default:
			out = append(out, a)
		}
	}
	return out
}

func runComposeRedeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var service string
	cmd, code, ok := parseComposeCmd(prog, "compose redeploy", "compose redeploy <app> [--service S]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&service, "service", "", "recreate only this service (its compose key)")
	})
	if !ok {
		return code
	}
	restarted, err := cmd.client.RedeployComposeStack(context.Background(), cmd.args[0], service)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose redeploy %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, map[string]any{"app_id": cmd.args[0], "restarted": restarted}, func() {
		_, _ = fmt.Fprintf(stdout, "recreating: %s\n", strings.Join(restarted, ", "))
	})
}

func runComposeRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var revision int
	cmd, code, ok := parseComposeCmd(prog, "compose rollback", "compose rollback <app> [--revision N]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&revision, "revision", 0, "revision to re-apply (default: the one before the latest)")
	})
	if !ok {
		return code
	}
	res, err := cmd.client.RollbackComposeStack(context.Background(), cmd.args[0], revision)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose rollback %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: rolled back to revision %d (now revision %d)\n", res.AppID, res.RolledBackTo, res.Deploy.Revision)
	})
}

func runComposeDown(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var volumes bool
	cmd, code, ok := parseComposeCmd(prog, "compose down", "compose down <app> [--volumes]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&volumes, "volumes", false, "also delete the stack's named volumes (data is lost)")
	})
	if !ok {
		return code
	}
	res, err := cmd.client.ComposeDown(context.Background(), cmd.args[0], volumes)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose down %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, res, func() {
		_, _ = fmt.Fprintf(stdout, "removed: %s\n", strings.Join(res.Removed, ", "))
		if len(res.TeardownPending) > 0 {
			_, _ = fmt.Fprintf(stdout, "containers still stopping (retried automatically): %s\n", strings.Join(res.TeardownPending, ", "))
		}
		if len(res.VolumesKept) > 0 {
			_, _ = fmt.Fprintf(stdout, "volumes kept: %s (rerun with --volumes to delete)\n", strings.Join(res.VolumesKept, ", "))
		}
		if len(res.VolumesRemoved) > 0 {
			_, _ = fmt.Fprintf(stdout, "volumes removed: %s\n", strings.Join(res.VolumesRemoved, ", "))
		}
		for _, e := range res.VolumeErrors {
			_, _ = fmt.Fprintf(stdout, "volume not removed: %s\n", e)
		}
	})
}
