package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// protectionCmd bundles one backups subcommand's flag set and the common
// API flags so each handler below stays a few lines.
type protectionCmd struct {
	prog, label string
	fs          *flag.FlagSet
	ptrs        apiFlagPtrs
	stdout      io.Writer
	stderr      io.Writer
	lookupEnv   func(string) (string, bool)
	usage       string

	client  *Client
	jsonOut bool
	of      outputFlags
}

func newProtectionCmd(prog, label, usage string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) *protectionCmd {
	fs, t, a, p, j, o, q := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	c := &protectionCmd{prog: prog, label: label, fs: fs, ptrs: apiFlagPtrs{t, a, p, j, o, q}, stdout: stdout, stderr: stderr, lookupEnv: lookupEnv, usage: usage}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	return c
}

// parse parses flags and builds the client. ok is false once a message was
// written and the caller should return exit.
func (c *protectionCmd) parse(args []string, wantArgs int) (rest []string, exit int, ok bool) {
	token, apiURL, profile, jsonOut, of, exit, ok := parseAPIFlags(c.fs, args, c.ptrs, c.prog, c.stderr)
	if !ok {
		return nil, exit, false
	}
	rest = c.fs.Args()
	if len(rest) != wantArgs {
		_, _ = fmt.Fprintf(c.stderr, "%s: %s takes %d positional argument(s)\n\n", c.prog, c.label, wantArgs)
		c.fs.Usage()
		return nil, exitUsage, false
	}
	c.client = apiClientFromFlags(c.prog, apiURL, token, profile, c.lookupEnv)
	c.jsonOut, c.of = jsonOut, of
	return rest, exitOK, true
}

func (c *protectionCmd) fail(err error) int { return reportError(c.stdout, c.stderr, c.jsonOut, err) }

func (c *protectionCmd) render(v any, table func()) int {
	return writeScheduledTaskResult(c.stdout, c.stderr, c.of, v, table)
}

// runBackupsHealth implements "backups health": GET /api/v1/backups/health.
func runBackupsHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups health", backupsHealthUsage(prog), stdout, stderr, lookupEnv)
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	h, err := c.client.BackupHealth(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("backup health: %w", err))
	}
	return c.render(h, func() { printBackupHealth(stdout, h) })
}

func printBackupHealth(out io.Writer, h apiclient.BackupHealthResponse) {
	if len(h.Resources) == 0 {
		_, _ = fmt.Fprintln(out, "no backed up resources")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RESOURCE\tSTATE\tLAST BACKUP\tLAST VERIFIED RESTORE\tSIZE\tENCRYPTED\tNEXT RUN")
	for _, r := range h.Resources {
		name := r.ResourceName
		if r.Kind == "volume" {
			name = r.AppName + "/" + r.ResourceName
		}
		lastBackup, lastRestore := "-", "never"
		if r.LastBackup != nil {
			lastBackup = r.LastBackup.At
		}
		if r.LastVerifiedRestore != nil {
			lastRestore = r.LastVerifiedRestore.At
		}
		state := r.State
		if r.StateReason != "" {
			state += " (" + r.StateReason + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%t\t%s\n", name, state, lastBackup, lastRestore, r.TotalBytes, r.Encrypted, orDash(r.NextRun))
	}
	_ = tw.Flush()
	for _, t := range h.Targets {
		if t.Warning != "" {
			_, _ = fmt.Fprintf(out, "\ntarget %s (%s): %s\n", t.TargetID, t.Level, t.Warning)
		}
	}
	if !h.Encryption.Enabled {
		_, _ = fmt.Fprintln(out, "\nnew volume backups are compressed but not encrypted (APP_BACKUP_ENCRYPTION=off)")
	}
}

func backupsHealthUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups health [flags]

Shows every backed up database and app volume with its last backup, last
verified restore (a drill that proved a restore works), size, encryption
and next scheduled run, plus bucket protection warnings.

Flags:
  --token string     API token (default: %[2]s env var, then the credentials file)
  --api-url string  control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string  named credentials profile to read
  --json              print the result as JSON to stdout, nothing else
  --output string   output format: json, table, or text
  --query string    JMESPath expression to filter the result before printing
  -h, --help        show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runBackupsProtection implements "backups protection [--refresh]".
func runBackupsProtection(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := fmt.Sprintf("Usage:\n  %s backups protection [--refresh] [flags]\n\nShows whether each backup bucket has object lock or versioning. --refresh probes the buckets now.\n", prog)
	c := newProtectionCmd(prog, "backups protection", usage, stdout, stderr, lookupEnv)
	var refresh bool
	c.fs.BoolVar(&refresh, "refresh", false, "probe every bucket now instead of reading the last result")
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	var targets []apiclient.TargetProtectionResource
	var err error
	if refresh {
		targets, err = c.client.RefreshBackupProtection(context.Background(), nil)
	} else {
		var h apiclient.BackupHealthResponse
		h, err = c.client.BackupHealth(context.Background())
		targets = h.Targets
	}
	if err != nil {
		return c.fail(fmt.Errorf("backup bucket protection: %w", err))
	}
	return c.render(targets, func() {
		if len(targets) == 0 {
			_, _ = fmt.Fprintln(stdout, "no backup targets probed yet (use --refresh)")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TARGET\tLEVEL\tOBJECT LOCK\tVERSIONING\tKEY CAN DELETE\tCHECKED")
		for _, t := range targets {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%s\t%t\t%s\n", t.TargetID, t.Level, t.ObjectLock, orDash(t.Versioning), t.CanDelete, t.CheckedAt)
			if t.Warning != "" {
				_, _ = fmt.Fprintf(tw, "\t%s\n", t.Warning)
			}
			if t.ProbeError != "" {
				_, _ = fmt.Fprintf(tw, "\tprobe failed: %s\n", t.ProbeError)
			}
		}
		_ = tw.Flush()
	})
}

// runBackupsDrill implements "backups drill run|list|show".
func runBackupsDrill(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stderr, backupsDrillUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "run":
		return runBackupsDrillRun(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runBackupsDrillList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runBackupsDrillShow(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown backups drill subcommand %q\n\n%s", prog, args[0], backupsDrillUsage(prog))
	return exitUsage
}

func backupsDrillUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups drill run --backup ID [--wait] [flags]   restore a backup into a scratch resource, validate it, destroy it
  %[1]s backups drill list [--app A --volume V | --database D] [--limit N]   drill history
  %[1]s backups drill show <drill-id>                    one drill's checks and error

A drill proves a restore works. A failed drill exits non-zero.
`, prog)
}

func runBackupsDrillRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups drill run", backupsDrillUsage(prog), stdout, stderr, lookupEnv)
	var backupID string
	var wait bool
	var timeout time.Duration
	c.fs.StringVar(&backupID, "backup", "", "id of a succeeded backup to drill (required)")
	c.fs.BoolVar(&wait, "wait", true, "wait for the drill to finish and exit non-zero when it fails")
	c.fs.DurationVar(&timeout, "timeout", 30*time.Minute, "how long --wait polls")
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	if backupID == "" {
		return c.fail(newValidationError("--backup is required"))
	}
	ctx := context.Background()
	id, err := c.client.StartBackupDrill(ctx, backupID)
	if err != nil {
		return c.fail(fmt.Errorf("start drill of backup %q: %w", backupID, err))
	}
	if !wait {
		return c.render(map[string]string{"id": id, "backup_id": backupID}, func() {
			_, _ = fmt.Fprintf(stdout, "drill %s started; check \"%s backups drill show %s\"\n", id, prog, id)
		})
	}
	deadline := time.Now().Add(timeout)
	for {
		d, err := c.client.GetBackupDrill(ctx, id)
		if err != nil {
			return c.fail(fmt.Errorf("read drill %q: %w", id, err))
		}
		if d.Status != "running" {
			code := c.render(d, func() { printDrill(stdout, d) })
			if code == exitOK && d.Status != "passed" {
				return exitCheckFailed
			}
			return code
		}
		if time.Now().After(deadline) {
			return c.fail(fmt.Errorf("drill %q is still running after %s", id, timeout))
		}
		time.Sleep(2 * time.Second)
	}
}

func printDrill(out io.Writer, d apiclient.BackupDrillResource) {
	_, _ = fmt.Fprintf(out, "drill %s: %s (stage %s, %d ms)\n", d.ID, d.Status, d.Stage, d.DurationMS)
	_, _ = fmt.Fprintf(out, "  object present and complete: %t\n  checksum matches:            %t\n  restored into scratch:       %t\n  restored content matches:    %t\n  files: %d  bytes: %d\n", d.ObjectOK, d.ChecksumOK, d.RestoreOK, d.ContentOK, d.Files, d.Bytes)
	if d.Error != "" {
		_, _ = fmt.Fprintf(out, "  error: %s\n", d.Error)
	}
}

func runBackupsDrillShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups drill show", backupsDrillUsage(prog), stdout, stderr, lookupEnv)
	rest, exit, ok := c.parse(args, 1)
	if !ok {
		return exit
	}
	d, err := c.client.GetBackupDrill(context.Background(), rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("read drill %q: %w", rest[0], err))
	}
	return c.render(d, func() { printDrill(stdout, d) })
}

func runBackupsDrillList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups drill list", backupsDrillUsage(prog), stdout, stderr, lookupEnv)
	var app, volume, database string
	var limit int
	c.fs.StringVar(&app, "app", "", "only drills of this app's volumes")
	c.fs.StringVar(&volume, "volume", "", "only drills of this volume")
	c.fs.StringVar(&database, "database", "", "only drills of this database")
	c.fs.IntVar(&limit, "limit", 20, "max drills to return")
	if _, exit, ok := c.parse(args, 0); !ok {
		return exit
	}
	drills, err := c.client.ListBackupDrills(context.Background(), app, volume, database, limit)
	if err != nil {
		return c.fail(fmt.Errorf("list drills: %w", err))
	}
	return c.render(drills, func() {
		if len(drills) == 0 {
			_, _ = fmt.Fprintln(stdout, "no drills yet")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tRESOURCE\tTRIGGER\tSTATUS\tSTAGE\tFILES\tSTARTED\tERROR")
		for _, d := range drills {
			res := d.DatabaseName
			if d.ResourceKind == "volume" {
				res = d.ServiceName + "/" + d.VolumeName
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", d.ID, res, d.Trigger, d.Status, d.Stage, d.Files, d.StartedAt, d.Error)
		}
		_ = tw.Flush()
	})
}

// runBackupsVolumes implements "backups volumes list|policy|restore".
func runBackupsVolumes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stderr, backupsVolumesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "list":
		return runBackupsVolumesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "policy":
		return runBackupsVolumesPolicy(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runBackupsVolumesRestore(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown backups volumes subcommand %q\n\n%s", prog, args[0], backupsVolumesUsage(prog))
	return exitUsage
}

func backupsVolumesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups volumes list <app> <volume> [flags]       backup history of one app volume
  %[1]s backups volumes policy get <app> <volume>         show retention counts and hooks
  %[1]s backups volumes policy set <app> <volume> [--retain-daily N --retain-weekly N --retain-monthly N --pre-hook CMD --post-hook CMD --pause]
  %[1]s backups volumes restore <app> <volume> --backup ID [--new-volume NAME] [--target-app APP] [--node NODE]
                                                         restore into a NEW volume (never overwrites), optionally for another app or on another node
`, prog)
}

func runBackupsVolumesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups volumes list", backupsVolumesUsage(prog), stdout, stderr, lookupEnv)
	var limit int
	c.fs.IntVar(&limit, "limit", 0, "max backups to return")
	rest, exit, ok := c.parse(args, 2)
	if !ok {
		return exit
	}
	rows, err := c.client.ListVolumeBackups(context.Background(), rest[0], rest[1], apiclient.ListBackupsOptions{Limit: limit})
	if err != nil {
		return c.fail(fmt.Errorf("list backups of %s/%s: %w", rest[0], rest[1], err))
	}
	return c.render(rows, func() { printAllBackupHistoryTable(stdout, rows) })
}

func runBackupsVolumesPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, backupsVolumesUsage(prog))
		return exitUsage
	}
	verb := args[0]
	c := newProtectionCmd(prog, "backups volumes policy "+verb, backupsVolumesUsage(prog), stdout, stderr, lookupEnv)
	var p apiclient.VolumeBackupPolicyResource
	var pause bool
	if verb == "set" {
		c.fs.IntVar(&p.RetainDaily, "retain-daily", 0, "keep the newest backup of each of the last N days")
		c.fs.IntVar(&p.RetainWeekly, "retain-weekly", 0, "keep the newest backup of each of the last N weeks")
		c.fs.IntVar(&p.RetainMonthly, "retain-monthly", 0, "keep the newest backup of each of the last N months")
		c.fs.StringVar(&p.PreHook, "pre-hook", "", "shell command run inside the app container before the archive")
		c.fs.StringVar(&p.PostHook, "post-hook", "", "shell command run inside the app container after the archive")
		c.fs.BoolVar(&pause, "pause", false, "pause the app container while the archive streams")
	}
	rest, exit, ok := c.parse(args[1:], 2)
	if !ok {
		return exit
	}
	ctx := context.Background()
	var out apiclient.VolumeBackupPolicyResource
	var err error
	if verb == "get" {
		out, err = c.client.GetVolumeBackupPolicy(ctx, rest[0], rest[1])
	} else {
		if pause {
			p.Quiesce = "pause"
		}
		out, err = c.client.SetVolumeBackupPolicy(ctx, rest[0], rest[1], p)
	}
	if err != nil {
		return c.fail(fmt.Errorf("backup policy of %s/%s: %w", rest[0], rest[1], err))
	}
	return c.render(out, func() {
		_, _ = fmt.Fprintf(stdout, "retain daily %d, weekly %d, monthly %d\npre-hook: %s\npost-hook: %s\nquiesce: %s\n",
			out.RetainDaily, out.RetainWeekly, out.RetainMonthly, orDash(out.PreHook), orDash(out.PostHook), orDash(out.Quiesce))
	})
}

func runBackupsVolumesRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups volumes restore", backupsVolumesUsage(prog), stdout, stderr, lookupEnv)
	var req apiclient.VolumeRestoreToRequest
	c.fs.StringVar(&req.BackupID, "backup", "", "id of the volume backup to restore (required)")
	c.fs.StringVar(&req.NewVolumeName, "new-volume", "", "name of the new volume (default: generated)")
	c.fs.StringVar(&req.TargetApp, "target-app", "", "name the new volume for this app so deploying it picks the data up")
	c.fs.StringVar(&req.NodeID, "node", "", "restore onto this node instead of the source app's node")
	rest, exit, ok := c.parse(args, 2)
	if !ok {
		return exit
	}
	if req.BackupID == "" {
		return c.fail(newValidationError("--backup is required"))
	}
	out, err := c.client.RestoreVolumeTo(context.Background(), rest[0], rest[1], req)
	if err != nil {
		return c.fail(fmt.Errorf("restore %s/%s: %w", rest[0], rest[1], err))
	}
	return c.render(out, func() {
		_, _ = fmt.Fprintf(stdout, "restoring into new volume %q (restore %s); follow it with \"%s app-volume-backups clone-restores %s %s\"\n", out.NewVolumeName, out.ID, prog, rest[0], rest[1])
	})
}
