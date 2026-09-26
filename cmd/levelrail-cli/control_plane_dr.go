package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type drStatus = apiclient.ControlPlaneDR

const drWaitLimit = 30 * time.Minute

// drPollInterval is a variable so tests can poll quickly.
var drPollInterval = 2 * time.Second

func controlPlaneDRUsage(prog string) string {
	return fmt.Sprintf(`Off-box disaster recovery for the control plane database:
  %[1]s control-plane-backups schedule show                    destination, schedule, retention, last backup and drill, warnings
  %[1]s control-plane-backups schedule set [flags]             change the destination, recipients, schedule or retention
  %[1]s control-plane-backups run-now [--no-wait]              take an encrypted off-box backup now
  %[1]s control-plane-backups list --offbox                    list encrypted backups at the destination
  %[1]s control-plane-backups drill run [--no-wait]            restore the newest backup into a temp dir and check it
  %[1]s control-plane-backups drill status                     last drill result (exit 1 if it failed)
  %[1]s control-plane-backups escrow [flags]                   write the master key, encrypted to your recipients, to a file
  %[1]s control-plane-backups escrow open <file> --identity F  decrypt an escrow bundle locally
  %[1]s control-plane-backups keys generate [--out FILE]       make an age keypair for backups (private key stays on this machine)

schedule set flags: --enable | --disable, --destination ID, --recipient KEY (repeat),
--schedule CRON, --drill-schedule CRON, --retain-daily N, --retain-weekly N,
--retain-monthly N, --escrow-destination ID. Only the flags you pass change.
`, prog)
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runControlPlaneDR(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	switch sub {
	case "run-now":
		return runDRRun(prog, "run-now", args, stdout, stderr, lookupEnv)
	case "escrow":
		return runDREscrow(prog, args, stdout, stderr, lookupEnv)
	}
	if len(args) > 0 {
		switch args[0] {
		case "show":
			if sub == "schedule" {
				return runDRShow(prog, args[1:], stdout, stderr, lookupEnv)
			}
		case "set":
			if sub == "schedule" {
				return runDRSet(prog, args[1:], stdout, stderr, lookupEnv)
			}
		case "run":
			if sub == "drill" {
				return runDRRun(prog, "drill run", args[1:], stdout, stderr, lookupEnv)
			}
		case "status":
			if sub == "drill" {
				return runDRDrillStatus(prog, args[1:], stdout, stderr, lookupEnv)
			}
		case "generate":
			if sub == "keys" {
				return runKeysGenerate(prog, args[1:], stdout, stderr)
			}
		}
	}
	_, _ = fmt.Fprint(stderr, controlPlaneDRUsage(prog))
	return exitUsage
}

func drClient(prog string, fs *flag.FlagSet, args []string, p apiFlagPtrs, stderr io.Writer, lookupEnv func(string) (string, bool)) (*Client, bool, outputFlags, int, bool) {
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, p, prog, stderr)
	if !ok {
		return nil, false, outputFlags{}, code, false
	}
	return apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), jsonOut, of, 0, true
}

func runDRShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups schedule show", "print the status as JSON to stdout and nothing else", stderr)
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	st, err := client.GetControlPlaneDR(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get disaster recovery status: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printDRStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func orNone(s string) string {
	if s == "" {
		return "never"
	}
	return s
}

func printDRStatus(out io.Writer, st drStatus) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	row := func(k, v string) { _, _ = fmt.Fprintf(tw, "%s\t%s\n", k, v) }
	row("Enabled", fmt.Sprint(st.Enabled))
	row("Destination", orNone(st.TargetID))
	row("Install ID", orNone(st.InstallID))
	row("Recipients", fmt.Sprintf("%d", len(st.Recipients)))
	row("Backup schedule", st.Schedule)
	row("Retention", fmt.Sprintf("%d daily, %d weekly, %d monthly", st.RetainDaily, st.RetainWeekly, st.RetainMonthly))
	row("Last backup", orNone(st.LastBackupAt))
	if st.LastBackupError != "" {
		row("Last error", st.LastBackupError)
	}
	row("Next backup", orNone(st.NextBackupAt))
	row("Last drill", orNone(st.LastDrill.At))
	if st.LastDrill.At != "" {
		row("Drill result", drillLine(st.LastDrill))
	}
	row("Escrow generated", orNone(st.EscrowGeneratedAt))
	row("Escrow acknowledged", orNone(st.EscrowAckedAt))
	_ = tw.Flush()
	for _, w := range st.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", w.Message)
	}
}

func drillLine(d apiclient.ControlPlaneDRDrill) string {
	switch {
	case !d.OK:
		return "FAILED: " + d.Detail
	case d.Partial:
		return "passed (partial: checksum only)"
	}
	return "passed"
}

func runDRSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups schedule set", "print the new status as JSON to stdout and nothing else", stderr)
	enable := fs.Bool("enable", false, "turn scheduled off-box backups on")
	disable := fs.Bool("disable", false, "turn scheduled off-box backups off")
	dest := fs.String("destination", "", "storage destination ID for backups")
	escrowDest := fs.String("escrow-destination", "", "separate storage destination ID for optional escrow upload")
	schedule := fs.String("schedule", "", "backup cron expression (5 fields)")
	drillSchedule := fs.String("drill-schedule", "", "restore drill cron expression (5 fields)")
	daily := fs.Int("retain-daily", 0, "daily backups to keep")
	weekly := fs.Int("retain-weekly", 0, "weekly backups to keep")
	monthly := fs.Int("retain-monthly", 0, "monthly backups to keep")
	var recipients stringList
	fs.Var(&recipients, "recipient", "age public key backups are encrypted to (repeatable, replaces the list)")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	if *enable && *disable {
		_, _ = fmt.Fprintln(stderr, "--enable and --disable cannot be combined")
		return exitValidation
	}
	ctx := context.Background()
	cur, err := client.GetControlPlaneDR(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get disaster recovery status: %w", err))
	}
	set := drSettingsFrom(cur)
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "enable":
			set.Enabled = true
		case "disable":
			set.Enabled = false
		case "destination":
			set.TargetID = *dest
		case "escrow-destination":
			set.EscrowTargetID = *escrowDest
		case "schedule":
			set.Schedule = *schedule
		case "drill-schedule":
			set.DrillSchedule = *drillSchedule
		case "retain-daily":
			set.RetainDaily = *daily
		case "retain-weekly":
			set.RetainWeekly = *weekly
		case "retain-monthly":
			set.RetainMonthly = *monthly
		case "recipient":
			set.Recipients = recipients
		}
	})
	st, err := client.UpdateControlPlaneDR(ctx, set)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update disaster recovery settings: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printDRStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func drSettingsFrom(st drStatus) apiclient.ControlPlaneDRSettings {
	return apiclient.ControlPlaneDRSettings{
		Enabled: st.Enabled, TargetID: st.TargetID, Recipients: st.Recipients, Schedule: st.Schedule, DrillSchedule: st.DrillSchedule,
		RetainDaily: st.RetainDaily, RetainWeekly: st.RetainWeekly, RetainMonthly: st.RetainMonthly, EscrowTargetID: st.EscrowTargetID,
	}
}

// runDRRun starts a backup or drill and, unless --no-wait, polls until it finishes.
func runDRRun(prog, label string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups "+label, "print the resulting status as JSON to stdout and nothing else", stderr)
	noWait := fs.Bool("no-wait", false, "return as soon as the run has started")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	ctx, cancel := context.WithTimeout(context.Background(), drWaitLimit)
	defer cancel()
	drill := label == "drill run"
	var err error
	if drill {
		err = client.RunControlPlaneDrill(ctx)
	} else {
		err = client.RunControlPlaneOffboxBackup(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start %s: %w", label, err))
	}
	if *noWait {
		_, _ = fmt.Fprintln(stderr, label+" started")
		return exitOK
	}
	st, err := waitDR(ctx, client, drill)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printDRStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if (drill && !st.LastDrill.OK) || (!drill && st.LastBackupError != "") {
		return exitCheckFailed
	}
	return exitOK
}

func waitDR(ctx context.Context, client *Client, drill bool) (drStatus, error) {
	for {
		select {
		case <-ctx.Done():
			return drStatus{}, fmt.Errorf("gave up waiting: %w", ctx.Err())
		case <-time.After(drPollInterval):
		}
		st, err := client.GetControlPlaneDR(ctx)
		if err != nil {
			return drStatus{}, fmt.Errorf("get disaster recovery status: %w", err)
		}
		if (drill && !st.DrillRunning) || (!drill && !st.BackupRunning) {
			return st, nil
		}
	}
}

func runDRDrillStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups drill status", "print the last drill as JSON to stdout and nothing else", stderr)
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	st, err := client.GetControlPlaneDR(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get disaster recovery status: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st.LastDrill, func() {
		if st.LastDrill.At == "" {
			_, _ = fmt.Fprintln(stdout, "no restore drill has run yet")
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s (%d ms)\n", st.LastDrill.At, drillLine(st.LastDrill), st.LastDrill.DurationMs)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if st.LastDrill.At == "" || !st.LastDrill.OK {
		return exitCheckFailed
	}
	return exitOK
}

func listOffbox(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups list --offbox", "print backups as a JSON array to stdout and nothing else", stderr)
	fs.Bool("offbox", true, "list encrypted off-box backups")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	list, err := client.ListControlPlaneOffboxBackups(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list off-box backups: %w", err))
	}
	if list == nil {
		list = []apiclient.ControlPlaneOffboxBackup{}
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no off-box backups yet")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KEY\tSIZE\tCREATED\tCOMPLETE")
		for _, b := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%v\n", b.Key, b.SizeBytes, b.CreatedAt, b.Complete)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
