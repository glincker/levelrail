package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func janitorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s janitor status [flags]                    disk state, policy, last pass and quarantined volumes
  %[1]s janitor run [--dry-run] [--if-needed]     one cleanup pass now (--dry-run previews, --if-needed waits for disk pressure)
  %[1]s janitor policy [--enabled=BOOL ...]       show the policy, or change the fields you pass

Policy flags: --enabled, --dry-run, --interval-minutes, --disk-free-budget-percent,
  --build-cache-budget-mb, --build-cache-max-age-hours, --volume-quarantine-days,
  --delete-orphan-volumes, --attempt-retention-days, --attempt-keep-per-app, --disabled-stages a,b
`, prog)
}

func runJanitor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, janitorUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, janitorUsage(prog))
		return exitOK
	case "status":
		return runJanitorStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runJanitorRun(prog, args[1:], stdout, stderr, lookupEnv)
	case "policy":
		return runJanitorPolicy(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown janitor subcommand %q\n\n%s", prog, args[0], janitorUsage(prog))
	return exitUsage
}

func runJanitorStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "janitor status", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, janitorUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	st, err := client.GetJanitor(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("janitor status: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printJanitorStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printJanitorStatus(out io.Writer, st apiclient.JanitorStatus) {
	mode := "removing"
	switch {
	case !st.Policy.Enabled:
		mode = "off"
	case st.Policy.DryRun:
		mode = "preview only"
	}
	_, _ = fmt.Fprintf(out, "cleanup: %s, every %d min\n", mode, st.Policy.IntervalMinutes)
	if st.TotalBytes > 0 {
		pressure := ""
		if st.Pressure {
			pressure = " (below the " + fmt.Sprintf("%.0f", st.Policy.DiskFreeBudgetPercent) + "% budget)"
		}
		_, _ = fmt.Fprintf(out, "disk: %s free of %s, %.1f%%%s\n", humanBytes(st.FreeBytes), humanBytes(st.TotalBytes), st.FreePercent, pressure)
	}
	_, _ = fmt.Fprintf(out, "reclaimed so far: %s\n", humanBytes(st.TotalReclaimed))
	if st.LastRun != nil {
		_, _ = fmt.Fprintf(out, "last pass: %s, %s, %s\n", st.LastRun.StartedAt.Format("2006-01-02 15:04"), st.LastRun.Trigger, humanBytes(st.LastRun.ReclaimedBytes))
	}
	if len(st.Quarantine) > 0 {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "QUARANTINED VOLUME\tSIZE\tDELETABLE AT")
		for _, q := range st.Quarantine {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", q.Name, humanBytes(q.SizeBytes), q.DeletableAt.Format("2006-01-02"))
		}
		_ = tw.Flush()
	}
}

func runJanitorRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "janitor run", "print the report as JSON to stdout and nothing else", stderr)
	dry := fs.Bool("dry-run", false, "preview what would be removed without removing anything")
	ifNeeded := fs.Bool("if-needed", false, "run the size-driven stages only when free space is below the budget")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, janitorUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.RunJanitor(context.Background(), *dry, !*ifNeeded)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("janitor run: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printJanitorReport(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if rep.Error != "" {
		return exitCheckFailed
	}
	return exitOK
}

func printJanitorReport(out io.Writer, rep apiclient.JanitorReport) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STAGE\tACTION\tITEM\tSIZE\tNOTE")
	for _, s := range rep.Stages {
		if s.Status == "skipped" {
			_, _ = fmt.Fprintf(tw, "%s\tskipped\t\t\t%s\n", s.Stage, s.Reason)
			continue
		}
		if len(s.Items) == 0 {
			_, _ = fmt.Fprintf(tw, "%s\tnothing to do\t\t\t\n", s.Stage)
		}
		for _, it := range s.Items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Stage, strings.ReplaceAll(it.Action, "_", " "), it.Name, humanBytes(it.SizeBytes), it.Detail)
		}
		if s.Error != "" {
			_, _ = fmt.Fprintf(tw, "%s\tfailed\t\t\t%s\n", s.Stage, s.Error)
		}
	}
	_ = tw.Flush()
	if rep.DryRun {
		_, _ = fmt.Fprintln(out, "preview only, nothing was removed")
		return
	}
	_, _ = fmt.Fprintf(out, "reclaimed %s\n", humanBytes(rep.ReclaimedBytes))
}

func runJanitorPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "janitor policy", "print the policy as JSON to stdout and nothing else", stderr)
	enabled := fs.Bool("enabled", false, "turn scheduled cleanup on or off")
	dry := fs.Bool("dry-run", false, "scheduled passes preview without removing")
	interval := fs.Int("interval-minutes", 0, "minutes between scheduled passes")
	budget := fs.Float64("disk-free-budget-percent", 0, "free space percentage below which size-driven stages run")
	cacheMB := fs.Int64("build-cache-budget-mb", 0, "build cache size that triggers a deeper trim, 0 disables")
	cacheAge := fs.Int("build-cache-max-age-hours", 0, "idle build cache older than this is removed")
	quarantine := fs.Int("volume-quarantine-days", 0, "days an orphaned volume is held before it may be deleted")
	delVols := fs.Bool("delete-orphan-volumes", false, "let a pass delete volumes whose quarantine ended")
	retention := fs.Int("attempt-retention-days", 0, "age after which failed deploy attempts may be deleted")
	keep := fs.Int("attempt-keep-per-app", 0, "newest deploy attempts per app that are always kept")
	stages := fs.String("disabled-stages", "", "comma separated stages to skip (containers,images,build_cache,volumes,deploy_attempts), empty to enable all")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, janitorUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	var patch apiclient.JanitorPolicyPatch
	changed := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "enabled":
			patch.Enabled = enabled
		case "dry-run":
			patch.DryRun = dry
		case "interval-minutes":
			patch.IntervalMinutes = interval
		case "disk-free-budget-percent":
			patch.DiskFreeBudgetPercent = budget
		case "build-cache-budget-mb":
			patch.BuildCacheBudgetMB = cacheMB
		case "build-cache-max-age-hours":
			patch.BuildCacheMaxAgeHours = cacheAge
		case "volume-quarantine-days":
			patch.VolumeQuarantineDays = quarantine
		case "delete-orphan-volumes":
			patch.DeleteOrphanVolumes = delVols
		case "attempt-retention-days":
			patch.AttemptRetentionDays = retention
		case "attempt-keep-per-app":
			patch.AttemptKeepPerApp = keep
		case "disabled-stages":
			list := []string{}
			for _, s := range strings.Split(*stages, ",") {
				if s = strings.TrimSpace(s); s != "" {
					list = append(list, s)
				}
			}
			patch.DisabledStages = &list
		default:
			return
		}
		changed = true
	})
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var (
		pol apiclient.JanitorPolicy
		err error
	)
	if changed {
		pol, err = client.UpdateJanitorPolicy(context.Background(), patch)
	} else {
		var st apiclient.JanitorStatus
		st, err = client.GetJanitor(context.Background())
		pol = st.Policy
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("janitor policy: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, pol, func() { printJanitorPolicy(stdout, pol) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printJanitorPolicy(out io.Writer, p apiclient.JanitorPolicy) {
	_, _ = fmt.Fprintf(out, "enabled: %t\ndry-run: %t\ninterval-minutes: %d\ndisk-free-budget-percent: %.0f\n", p.Enabled, p.DryRun, p.IntervalMinutes, p.DiskFreeBudgetPercent)
	_, _ = fmt.Fprintf(out, "build-cache-budget-mb: %d\nbuild-cache-max-age-hours: %d\nvolume-quarantine-days: %d\ndelete-orphan-volumes: %t\n", p.BuildCacheBudgetMB, p.BuildCacheMaxAgeHours, p.VolumeQuarantineDays, p.DeleteOrphanVolumes)
	_, _ = fmt.Fprintf(out, "attempt-retention-days: %d\nattempt-keep-per-app: %d\ndisabled-stages: %s\n", p.AttemptRetentionDays, p.AttemptKeepPerApp, strings.Join(p.DisabledStages, ","))
}
