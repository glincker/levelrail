package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runUpgradeSubcommand handles "upgrade schedule|auto|backup-now". It reports
// handled=false for any other first argument so the flag-style upgrade
// command keeps working unchanged.
func runUpgradeSubcommand(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (code int, handled bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "schedule":
		return runUpgradeSchedule(prog, args[1:], stdout, stderr, lookupEnv), true
	case "auto":
		return runUpgradeAuto(prog, args[1:], stdout, stderr, lookupEnv), true
	case "retry", "rollback":
		return runUpgradeAttemptAction(prog, args[0], args[1:], stdout, stderr, lookupEnv), true
	case "backup-now":
		return runUpgradeBackupNow(prog, args[1:], stdout, stderr, lookupEnv), true
	case "status":
		return runUpgradeStatus(prog, args[1:], stdout, stderr, lookupEnv), true
	}
	return 0, false
}

// runUpgradeSchedule implements "upgrade schedule [show|cancel]".
func runUpgradeSchedule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "upgrade schedule", "print the result as JSON to stdout and nothing else", stderr)
	at := fs.String("at", "", "run at `time`: RFC 3339 (2026-10-12T03:00:00+02:00) or a local time like \"2026-10-12 03:00\"")
	window := fs.Bool("window", false, "run in the next maintenance window instead of at a time")
	version := fs.String("version", "", "release `tag` to install (default: latest on the channel)")
	tz := fs.String("timezone", "", "IANA timezone a local --at time is in (default: the server's)")
	ackBreaking := fs.String("ack-breaking", "", "comma-separated breaking-change `ids` you acknowledge")
	ackMajor := fs.Bool("ack-major", false, "acknowledge a major version jump")
	replace := fs.Bool("replace", false, "replace an existing pending schedule")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s upgrade schedule [--at TIME | --window] [flags]\n  %s upgrade schedule show\n  %s upgrade schedule cancel\n\nSchedules the control plane upgrade. The preflight re-runs at the scheduled time and the upgrade\nis not applied if it fails; it then retries until the grace period or window ends.\n\nFlags:\n", prog, prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if sub == "cancel" {
		sc, err := client.CancelUpgradeSchedule(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("cancel scheduled upgrade: %w", err))
		}
		return renderUpgradeResult(stdout, stderr, of, sc, func() {
			_, _ = fmt.Fprintf(stdout, "cancelled the scheduled upgrade to %s\n", sc.TargetVersion)
		})
	}
	if sub != "" && sub != "show" {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("unknown schedule command %q (use show or cancel)", sub))
	}
	if sub == "show" || (*at == "" && !*window) {
		st, err := client.GetUpgradeSchedule(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade schedule: %w", err))
		}
		return renderUpgradeResult(stdout, stderr, of, st, func() { printUpgradeScheduleStatus(stdout, st) })
	}

	if *at != "" && *window {
		return reportError(stdout, stderr, jsonOut, errors.New("use either --at or --window, not both"))
	}
	req := apiclient.UpgradeScheduleRequest{
		Target: *version, Window: *window, Ack: splitCSV(*ackBreaking), AcknowledgeMajor: *ackMajor, Replace: *replace, Timezone: *tz,
	}
	if *at != "" {
		if t, err := time.Parse(time.RFC3339, *at); err == nil {
			req.At = t.UTC().Format(time.RFC3339)
		} else {
			local, perr := normalizeLocalTime(*at)
			if perr != nil {
				return reportError(stdout, stderr, jsonOut, perr)
			}
			req.AtLocal = local
		}
	}
	sc, err := client.CreateUpgradeSchedule(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("schedule upgrade: %w", err))
	}
	return renderUpgradeResult(stdout, stderr, of, sc, func() {
		_, _ = fmt.Fprintf(stdout, "scheduled the upgrade to %s for %s (retries until %s)\n", sc.TargetVersion, sc.RunAt, sc.RetryUntil)
		_, _ = fmt.Fprintln(stdout, "the preflight re-runs at that time; cancel with: upgrade schedule cancel")
	})
}

func normalizeLocalTime(v string) (string, error) {
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, strings.TrimSpace(v)); err == nil {
			return t.Format("2006-01-02T15:04"), nil
		}
	}
	return "", fmt.Errorf("--at %q is not a time: use RFC 3339 or \"2026-10-12 03:00\"", v)
}

func renderUpgradeResult(stdout, stderr io.Writer, of outputFlags, v any, human func()) int {
	if err := renderResult(stdout, of.Format, of.Query, v, human); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printUpgradeScheduleStatus(out io.Writer, st apiclient.UpgradeScheduleStatus) {
	_, _ = fmt.Fprintf(out, "server time zone: %s\n", st.ServerTimezone)
	if st.Active == nil {
		_, _ = fmt.Fprintln(out, "no upgrade is scheduled")
	} else {
		a := st.Active
		_, _ = fmt.Fprintf(out, "scheduled: %s at %s (%s, %s, retries until %s)\n", a.TargetVersion, a.RunAt, a.Source, a.State, a.RetryUntil)
		if a.LastReason != "" {
			_, _ = fmt.Fprintf(out, "  last check: %s\n", a.LastReason)
		}
	}
	if st.NextWindow != nil {
		_, _ = fmt.Fprintf(out, "next maintenance window: %s to %s\n", st.NextWindow.Start, st.NextWindow.End)
	}
	for _, r := range st.Recent {
		_, _ = fmt.Fprintf(out, "  %s  %-9s %s %s\n", r.RunAt, r.State, r.TargetVersion, r.LastReason)
	}
}

// runUpgradeAuto implements "upgrade auto get|set".
func runUpgradeAuto(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub := "get"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "upgrade auto", "print the result as JSON to stdout and nothing else", stderr)
	mode := fs.String("mode", "", "off, patch (same minor version only) or any (any new release on the channel)")
	cron := fs.String("window-cron", "", "maintenance window start as a cron `expression`, for example \"0 3 * * *\"")
	duration := fs.Duration("window-duration", 0, "how long the window stays open, for example 2h")
	tz := fs.String("timezone", "", "IANA timezone the window cron is evaluated in")
	minAge := fs.Int("min-age-days", 0, "wait this many `days` after a release is published")
	notify := fs.String("notify", "", "comma-separated notification channel `ids` (empty keeps the current ones)")
	pause := fs.Bool("pause", false, "pause automatic upgrades without losing the policy")
	resume := fs.Bool("resume", false, "resume paused automatic upgrades")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s upgrade auto get\n  %s upgrade auto set --mode off|patch|any [flags]\n\nAutomatic upgrades are off by default. Releases with breaking changes that need acknowledgement,\nand major version jumps, are never applied automatically.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := client.GetUpgradePolicy(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade policy: %w", err))
	}
	switch sub {
	case "get":
		return renderUpgradeResult(stdout, stderr, of, cur, func() { printUpgradePolicy(stdout, cur) })
	case "set":
	default:
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("unknown auto command %q (use get or set)", sub))
	}
	if *pause && *resume {
		return reportError(stdout, stderr, jsonOut, errors.New("use either --pause or --resume, not both"))
	}
	req := apiclient.UpgradePolicyRequest{
		Mode: cur.Mode, WindowCron: cur.WindowCron, WindowDurationMinutes: cur.WindowDurationMinutes, Timezone: cur.Timezone,
		MinReleaseAgeDays: cur.MinReleaseAgeDays, NotifyChannelIDs: cur.NotifyChannelIDs, Paused: cur.Paused,
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["mode"] {
		req.Mode = *mode
	}
	if set["window-cron"] {
		req.WindowCron = *cron
	}
	if set["window-duration"] {
		req.WindowDurationMinutes = int(duration.Minutes())
	}
	if set["timezone"] {
		req.Timezone = *tz
	}
	if set["min-age-days"] {
		req.MinReleaseAgeDays = *minAge
	}
	if set["notify"] {
		req.NotifyChannelIDs = splitCSV(*notify)
	}
	if *pause {
		req.Paused = true
	}
	if *resume {
		req.Paused = false
	}
	got, err := client.SetUpgradePolicy(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set upgrade policy: %w", err))
	}
	return renderUpgradeResult(stdout, stderr, of, got, func() { printUpgradePolicy(stdout, got) })
}

func printUpgradePolicy(out io.Writer, p apiclient.UpgradePolicy) {
	state := p.Mode
	if p.Effective == "paused" {
		state += " (paused)"
	}
	_, _ = fmt.Fprintf(out, "automatic upgrades: %s\n", state)
	if p.WindowCron != "" {
		_, _ = fmt.Fprintf(out, "window: %q for %dm in %s\n", p.WindowCron, p.WindowDurationMinutes, p.Timezone)
	}
	_, _ = fmt.Fprintf(out, "minimum release age: %d days\n", p.MinReleaseAgeDays)
	if len(p.NotifyChannelIDs) > 0 {
		_, _ = fmt.Fprintf(out, "notify: %s\n", strings.Join(p.NotifyChannelIDs, ", "))
	}
	if p.NextWindow != nil {
		_, _ = fmt.Fprintf(out, "next window: %s to %s\n", p.NextWindow.Start, p.NextWindow.End)
	}
	if p.Decision != "" {
		_, _ = fmt.Fprintf(out, "latest decision: %s %s\n", p.DecisionTag, p.Decision)
	}
}

// runUpgradeBackupNow implements "upgrade backup-now".
func runUpgradeBackupNow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "upgrade backup-now", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s upgrade backup-now\n\nTakes a control plane backup and re-runs the upgrade preflight, so a stale-backup blocker clears.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.UpgradeBackupNow(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("backup now: %w", err))
	}
	return renderUpgradeResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "backup taken: %s\n", res.Backup.Name)
		printUpgradeHuman(stdout, res.Preflight, "taken: "+res.Backup.Name)
	})
}
