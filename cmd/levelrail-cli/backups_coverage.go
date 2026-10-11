package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func coverageName(kind, app, name string) string {
	if kind == "volume" {
		return app + "/" + name
	}
	return name
}

// runBackupsCoverage implements "backups coverage": GET /api/v1/backups/coverage.
func runBackupsCoverage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups coverage", backupsCoverageUsage(prog), stdout, stderr, lookupEnv)
	var unprotected bool
	c.fs.BoolVar(&unprotected, "unprotected", false, "only list resources that are not protected")
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	cov, err := c.client.BackupCoverage(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("backup coverage: %w", err))
	}
	if unprotected {
		rows := cov.Resources[:0:0]
		for _, r := range cov.Resources {
			if r.State == "unprotected" {
				rows = append(rows, r)
			}
		}
		cov.Resources = rows
	}
	return c.render(cov, func() { printCoverage(stdout, cov) })
}

func printCoverage(out io.Writer, cov apiclient.BackupCoverageResponse) {
	s := cov.Summary
	_, _ = fmt.Fprintf(out, "%d of %d resources protected (%d verified, %d not verified, %d never ran, %d failing, %d not protected)\n",
		s.Protected, s.Total, s.Verified, s.Unverified, s.NeverRan, s.Failing, s.Unprotected)
	_, _ = fmt.Fprintf(out, "last verified restore: %s   next run: %s   storage used: %d bytes\n\n", orDash(s.LastVerifiedRestore), orDash(s.NextRun), s.StorageBytes)
	if len(cov.Resources) == 0 {
		_, _ = fmt.Fprintln(out, "no resources to show")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RESOURCE\tKIND\tSTATE\tLAST BACKUP\tLAST VERIFIED\tNEXT RUN\tNOTE")
	for _, r := range cov.Resources {
		note := r.Reason
		if r.Scratch {
			note = "scratch " + note
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", coverageName(r.Kind, r.AppName, r.Name), r.Kind, r.State,
			orDash(r.LastBackupAt), orDash(r.LastVerifiedAt), orDash(r.NextRun), note)
	}
	_ = tw.Flush()
}

func backupsCoverageUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups coverage [--unprotected] [flags]

Lists every database and app volume with whether it is protected and
verified, protected but never proven to restore, scheduled but never run,
failing, or not protected at all.

Flags:
  --unprotected     only list resources that are not protected
  --token string    API token (default: %[2]s env var, then the credentials file)
  --api-url string  control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string  named credentials profile to read
  --json            print the result as JSON to stdout, nothing else
  --output string   output format: json, table, or text
  --query string    JMESPath expression to filter the result before printing
  -h, --help        show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runBackupsProtectAll implements "backups protect-all": plan by default,
// --apply to schedule.
func runBackupsProtectAll(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups protect-all", backupsProtectAllUsage(prog), stdout, stderr, lookupEnv)
	var (
		target, schedule        string
		retain, retainDays      int
		includeScratch, applied bool
	)
	retain, retainDays = -1, -1
	c.fs.StringVar(&target, "target", "", "backup target ID to store backups in (required)")
	c.fs.StringVar(&schedule, "schedule", "", "cron schedule (default: the instance default)")
	c.fs.IntVar(&retain, "retain", -1, "number of backups to keep (default: the instance default)")
	c.fs.IntVar(&retainDays, "retain-days", -1, "maximum backup age in days (default: the instance default)")
	c.fs.BoolVar(&includeScratch, "include-scratch", false, "also protect preview and canary resources")
	c.fs.BoolVar(&applied, "apply", false, "apply the plan; without it only the plan is printed")
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	if target == "" {
		return c.fail(newValidationError("--target is required"))
	}
	req := apiclient.ProtectAllRequest{TargetID: target, Schedule: schedule, IncludeScratch: includeScratch, Apply: applied}
	if retain >= 0 {
		req.Retain = &retain
	}
	if retainDays >= 0 {
		req.RetainDays = &retainDays
	}
	res, err := c.client.ProtectAll(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("protect all: %w", err))
	}
	code := c.render(res, func() { printProtectAll(stdout, res) })
	if code == exitOK && len(res.Failed) > 0 {
		return exitCheckFailed
	}
	return code
}

func printProtectAll(out io.Writer, res apiclient.ProtectAllResponse) {
	if len(res.Plan) == 0 {
		_, _ = fmt.Fprintln(out, "every resource already has a backup schedule")
		return
	}
	verb := "would schedule"
	if res.Applied {
		verb = "scheduled"
	}
	_, _ = fmt.Fprintf(out, "%s %d resources with %q, keeping %d backups, on target %s\n", verb, len(res.Plan), res.Schedule, res.Retain, res.TargetID)
	for _, p := range res.Plan {
		_, _ = fmt.Fprintf(out, "  %s (%s)\n", coverageName(p.Kind, p.AppName, p.Name), p.Kind)
	}
	for _, f := range res.Failed {
		_, _ = fmt.Fprintf(out, "  FAILED %s: %s\n", coverageName(f.Kind, f.AppName, f.Name), f.Error)
	}
	if !res.Applied {
		_, _ = fmt.Fprintf(out, "\nnothing changed. Re-run with --apply to schedule them.\n")
	}
}

func backupsProtectAllUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups protect-all --target ID [--schedule CRON] [--retain N] [--retain-days N] [--include-scratch] [--apply] [flags]

Schedules a backup for every database and app volume that has none. Without
--apply it only prints the plan and changes nothing. Preview and canary
resources are left out unless --include-scratch is set.

Flags:
  --target string       backup target ID (required)
  --schedule string     cron schedule (default: the instance default, 0 3 * * *)
  --retain int          backups to keep (default: the instance default)
  --retain-days int     maximum backup age in days (default: the instance default)
  --include-scratch     also protect preview and canary resources
  --apply               apply the plan
  --token string        API token (default: %[2]s env var, then the credentials file)
  --api-url string      control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string      named credentials profile to read
  --json                print the result as JSON to stdout, nothing else
  --output string       output format: json, table, or text
  --query string        JMESPath expression to filter the result before printing
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runBackupsOrphans implements "backups orphans list|reclaim".
func runBackupsOrphans(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stderr, backupsOrphansUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "list":
		return runBackupsOrphansList(prog, args[1:], stdout, stderr, lookupEnv)
	case "reclaim":
		return runBackupsOrphansReclaim(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown backups orphans subcommand %q\n\n%s", prog, args[0], backupsOrphansUsage(prog))
	return exitUsage
}

func backupsOrphansUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups orphans list [flags]                          data kept after its database or app was deleted
  %[1]s backups orphans reclaim <volume> --confirm <volume>   delete that data for good (destructive)

Nothing is ever removed automatically. Reclaiming needs the volume name typed
again as --confirm and a token with write:sensitive.
`, prog)
}

func runBackupsOrphansList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups orphans list", backupsOrphansUsage(prog), stdout, stderr, lookupEnv)
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	res, err := c.client.ListStorageOrphans(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list orphaned data: %w", err))
	}
	return c.render(res, func() {
		if len(res.Orphans) == 0 {
			_, _ = fmt.Fprintln(stdout, "no orphaned data")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "VOLUME\tBELONGED TO\tROLE\tSIZE\tAGE (DAYS)\tBACKUPS\tLAST BACKUP")
		for _, o := range res.Orphans {
			size, age, last := "unknown", "-", "none"
			if o.SizeBytes != nil {
				size = fmt.Sprintf("%d", *o.SizeBytes)
			}
			if o.AgeDays != nil {
				age = fmt.Sprintf("%d", *o.AgeDays)
			}
			if o.LastBackup != nil {
				last = o.LastBackup.At
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n", o.ID, coverageName(o.BelongedTo.Kind, o.BelongedTo.AppName, o.BelongedTo.Name),
				o.BelongedTo.Role, size, age, o.BackupCount, last)
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintf(stdout, "\ntotal %d bytes\n", res.TotalBytes)
	})
}

func runBackupsOrphansReclaim(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups orphans reclaim", backupsOrphansUsage(prog), stdout, stderr, lookupEnv)
	var confirm string
	c.fs.StringVar(&confirm, "confirm", "", "the volume name again, to confirm the deletion (required)")
	rest, exit, ok := c.parse(args, 1)
	if !ok {
		return exit
	}
	if confirm != rest[0] {
		return c.fail(newValidationError("--confirm must be exactly %q: this deletes the data and cannot be undone", rest[0]))
	}
	res, err := c.client.ReclaimStorageOrphan(context.Background(), rest[0], confirm)
	if err != nil {
		return c.fail(fmt.Errorf("reclaim %q: %w", rest[0], err))
	}
	return c.render(res, func() {
		_, _ = fmt.Fprintf(stdout, "removed %s, reclaimed %d bytes\n", res.Removed, res.ReclaimedBytes)
	})
}
