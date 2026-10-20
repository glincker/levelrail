package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func databasesUpgradesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases upgrades [<name>] [flags]
      With a name: current version, support status, available targets, policy and history.
      Without: every database's security and end-of-life status.
  %[1]s databases upgrade-policy <name>|--platform [flags]
      --auto off|patch|minor   automatic level (majors are never automatic)
      --window "0 3 * * 0"     maintenance window start (cron)
      --duration 2h            window length
      --timezone Europe/Berlin window timezone (default UTC)
      --verify=false           skip the engine health check after the bump
      --revert=false           leave a failed upgrade in place instead of reverting
      --notify chn_a,chn_b     notification channel ids
      --inherit                drop the database's own policy, use the platform default
  %[1]s databases upgrade-now <name> <version> [--confirm <name>] [flags]
      Back up, verify the backup, upgrade, health check, and revert on failure, now.
`, prog)
}

// runDatabasesUpgrades implements "databases upgrades [<name>]".
func runDatabasesUpgrades(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases upgrades", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesUpgradesUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if fs.NArg() == 0 {
		sum, err := client.DatabaseUpgradeSummary(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("database upgrade summary: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, sum, func() { printUpgradeSummary(stdout, sum) })
	}
	name, ok := requireOneArg(fs, stderr, prog, "databases upgrades", "database name")
	if !ok {
		return exitUsage
	}
	res, err := client.GetDatabaseUpgrades(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrades of database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDatabaseUpgrades(stdout, res) })
}

func printUpgradeSummary(out io.Writer, sum apiclient.DBUpgradeSummary) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DATABASE\tENGINE\tVERSION\tSUPPORT\tEOL\tSECURITY\tTARGETS\tLAST RUN")
	for _, it := range sum.Items {
		sec := "-"
		if it.Security {
			sec = "yes"
		}
		last := dash(it.LastState)
		if it.ActiveState != "" {
			last = "running: " + it.ActiveState
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", it.Database, it.Engine, it.Version, it.Support, dash(it.EOL), sec, it.Available, last)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\n%d with security updates, %d past end of life\n", sum.SecurityCount, sum.EOLCount)
}

func printDatabaseUpgrades(out io.Writer, r apiclient.DBUpgradesResource) {
	a := r.Advice
	_, _ = fmt.Fprintf(out, "%s: %s %s (line %s, support %s", r.Database, r.Engine, r.Version, dash(a.Line), a.Support)
	if a.EOL != "" {
		_, _ = fmt.Fprintf(out, ", end of life %s", a.EOL)
	}
	_, _ = fmt.Fprintln(out, ")")
	if len(a.Advisories) > 0 {
		_, _ = fmt.Fprintf(out, "affected by: %s\n", strings.Join(a.Advisories, ", "))
	}
	for _, n := range []string{a.Note, a.Notes, a.ManualReason} {
		if n != "" {
			_, _ = fmt.Fprintf(out, "note: %s\n", n)
		}
	}
	for _, b := range r.Blockers {
		_, _ = fmt.Fprintf(out, "blocked: %s\n", b)
	}

	_, _ = fmt.Fprintln(out, "\nTargets:")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "VERSION\tKIND\tSECURITY\tAUTOMATIC\tEOL")
	for _, t := range r.Advice.Targets {
		sec := "-"
		if t.Security {
			sec = strings.Join(t.Advisories, ",")
		}
		auto := "no"
		if t.Automatic {
			auto = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.Version, t.Kind, sec, auto, dash(t.EOL))
	}
	_ = tw.Flush()

	p := r.Policy
	inherited := ""
	if p.Inherited {
		inherited = " (platform default)"
	}
	_, _ = fmt.Fprintf(out, "\nPolicy%s: auto %s, window %q for %s %s, verify %v, revert %v, notify %s\n", inherited, p.AutoUpgrade,
		p.WindowCron, time.Duration(p.WindowDurationSeconds)*time.Second, dash(p.WindowTimezone), p.VerifyAfter, p.RevertOnFailure, dash(strings.Join(p.Notify, ",")))
	if r.NextWindow != "" {
		next := "nothing to apply"
		if r.NextTarget != nil {
			next = r.NextTarget.Version
		}
		_, _ = fmt.Fprintf(out, "next window: %s (%s)\n", r.NextWindow, next)
	}

	if len(r.History) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\nHistory:")
	tw = tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tFROM\tTO\tKIND\tSOURCE\tSTATE\tREVERT\tBACKUP\tREASON")
	for _, h := range r.History {
		state := h.State
		if h.Phase != "" && h.FinishedAt == "" {
			state += " (" + h.Phase + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.FromVersion, h.ToVersion, h.Kind, h.Source, state, dash(h.RevertPath), dash(h.BackupID), dash(h.Reason))
	}
	_ = tw.Flush()
}

// runDatabasesUpgradePolicy implements "databases upgrade-policy <name>|--platform".
// Flags not given keep their current value.
func runDatabasesUpgradePolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases upgrade-policy", "print the saved policy as JSON to stdout and nothing else", stderr)
	var (
		platform, inherit, verify, revert bool
		auto, window, tz, notify          string
		duration                          time.Duration
	)
	fs.BoolVar(&platform, "platform", false, "edit the platform default instead of one database")
	fs.BoolVar(&inherit, "inherit", false, "drop the database's own policy and use the platform default")
	fs.StringVar(&auto, "auto", "", "off, patch or minor")
	fs.StringVar(&window, "window", "", "maintenance window start as cron, e.g. \"0 3 * * 0\"")
	fs.DurationVar(&duration, "duration", 0, "maintenance window length, e.g. 2h")
	fs.StringVar(&tz, "timezone", "", "maintenance window timezone, e.g. Europe/Berlin")
	fs.BoolVar(&verify, "verify", true, "run engine health checks after the bump")
	fs.BoolVar(&revert, "revert", true, "revert automatically when the upgrade fails")
	fs.StringVar(&notify, "notify", "", "comma separated notification channel ids (\"\" clears)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesUpgradesUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	name := ""
	var current apiclient.DBUpgradePolicy
	if platform {
		if fs.NArg() != 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("--platform takes no database name"))
		}
		p, err := client.GetPlatformUpgradePolicy(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read platform upgrade policy: %w", err))
		}
		current = p
	} else {
		n, ok := requireOneArg(fs, stderr, prog, "databases upgrade-policy", "database name")
		if !ok {
			return exitUsage
		}
		name = n
		if !inherit {
			res, err := client.GetDatabaseUpgrades(ctx, name)
			if err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("read upgrade policy of %q: %w", name, err))
			}
			current = res.Policy
		}
	}
	next := applyPolicyFlags(fs, current, auto, window, tz, notify, duration, verify, revert)
	next.Inherit = inherit

	var saved apiclient.DBUpgradePolicy
	var err error
	if platform {
		saved, err = client.SetPlatformUpgradePolicy(ctx, next)
	} else {
		saved, err = client.SetDatabaseUpgradePolicy(ctx, name, next)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save upgrade policy: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() {
		target := "database " + name
		if platform {
			target = "the platform default"
		}
		_, _ = fmt.Fprintf(stdout, "upgrade policy of %s: auto %s, window %q for %s %s, verify %v, revert %v\n", target, saved.AutoUpgrade,
			saved.WindowCron, time.Duration(saved.WindowDurationSeconds)*time.Second, saved.WindowTimezone, saved.VerifyAfter, saved.RevertOnFailure)
	})
}

// applyPolicyFlags overlays only the flags the operator actually set.
func applyPolicyFlags(fs *flag.FlagSet, p apiclient.DBUpgradePolicy, auto, window, tz, notify string, duration time.Duration, verify, revert bool) apiclient.DBUpgradePolicy {
	p.Inherited = false
	p.BackupBefore = true
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "auto":
			p.AutoUpgrade = auto
		case "window":
			p.WindowCron = window
		case "duration":
			p.WindowDurationSeconds = int64(duration / time.Second)
		case "timezone":
			p.WindowTimezone = tz
		case "verify":
			p.VerifyAfter = verify
		case "revert":
			p.RevertOnFailure = revert
		case "notify":
			p.Notify = splitNonEmpty(notify)
		}
	})
	if p.Notify == nil {
		p.Notify = []string{}
	}
	return p
}

func splitNonEmpty(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// runDatabasesUpgradeNow implements "databases upgrade-now <name> <version>".
func runDatabasesUpgradeNow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases upgrade-now", "print the started run as JSON to stdout and nothing else", stderr)
	var confirm string
	fs.StringVar(&confirm, "confirm", "", "must equal the database name to skip the interactive prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesUpgradesUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases upgrade-now requires a database name and a version\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, version := rest[0], rest[1]
	warn := fmt.Sprintf("Database %q restarts on version %s after a fresh, verified backup. A failed upgrade is reverted automatically when the policy allows it.", name, version)
	if err := confirmDatabaseName(name, confirm, warn, os.Stdin, stderr); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	run, err := client.UpgradeDatabaseNow(context.Background(), name, version, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade database %q to %q: %w", name, version, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, run, func() {
		_, _ = fmt.Fprintf(stdout, "upgrade %s of database %q from %s to %s started; follow it with \"%s databases upgrades %s\"\n", run.ID, name, run.FromVersion, run.ToVersion, prog, name)
	})
}
