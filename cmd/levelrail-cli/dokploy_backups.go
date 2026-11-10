package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/platformimport/dokployextras"
)

// Backup import; the planning and create calls live in dokployextras, shared with the dashboard import.
const (
	backupWouldCreate = dokployextras.StateWouldCreate
	backupNeedsSecret = dokployextras.StateNeedsSecret
	backupCreated     = dokployextras.StateCreated
	backupExists      = dokployextras.StateExists
	backupPending     = dokployextras.StatePending
	backupSkipped     = dokployextras.StateSkipped
	backupReview      = dokployextras.StateReview
	backupFailed      = dokployextras.StateFailed

	journalKindBackupTarget   = dokployextras.KindBackupTarget
	journalKindBackupSchedule = dokployextras.KindBackupSchedule

	envMigrateBackupLimit = dokployextras.EnvLimit
	backupEndpointWarning = dokployextras.EndpointWarning
)

type (
	backupImport           = dokployextras.BackupImport
	importedBackupTarget   = dokployextras.BackupTarget
	importedBackupSchedule = dokployextras.BackupSchedule
	backupPlanOptions      = dokployextras.BackupOptions
)

// cliImportPolicy reads the private network opt-ins the CLI takes from env.
func cliImportPolicy(lookupEnv func(string) (string, bool)) platformimport.NetworkPolicy {
	return platformimport.NetworkPolicy{AllowPrivate: envIsTrue(lookupEnv, platformimport.AllowPrivateEnv), AllowLoopback: envIsTrue(lookupEnv, platformimport.AllowLoopbackEnv)}
}

func backupPlanOptionsFromEnv(ctx context.Context, enable bool, lookupEnv func(string) (string, bool)) backupPlanOptions {
	return dokployextras.NewBackupOptions(ctx, enable, cliImportPolicy(lookupEnv), lookupEnv)
}

func shQuote(s string) string {
	return dokployextras.ShQuote(s)
}

func importName(raw string) (string, bool) {
	return dokployextras.ImportName(raw)
}

func backupTargetFingerprint(provider, endpoint, bucket, region string) string {
	return dokployextras.TargetFingerprint(provider, endpoint, bucket, region)
}

func fingerprintOf(t backupTargetResource) string {
	return dokployextras.FingerprintOf(t)
}

func planDokployBackups(disc *platformimport.Discovery, o backupPlanOptions) *backupImport {
	return dokployextras.PlanBackups(disc, o)
}

func applyBackupImport(ctx context.Context, client *Client, imp *backupImport, jr *applyJournal) {
	dokployextras.ApplyBackups(ctx, client, imp, jr.extras())
}

// cliExtrasJournal adapts the file journal to the shared importer's journal.
type cliExtrasJournal struct{ jr *applyJournal }

func (jr *applyJournal) extras() dokployextras.Journal {
	if jr == nil {
		return nil
	}
	return cliExtrasJournal{jr: jr}
}

func (c cliExtrasJournal) FindCreated(kind, name string) (dokployextras.Record, bool) {
	r := c.jr.findCreated(kind, name)
	if r == nil {
		return dokployextras.Record{}, false
	}
	return dokployextras.Record{Kind: r.Kind, Name: r.Name, ID: r.TargetID, Fingerprint: r.Fingerprint, Cron: r.Cron}, true
}

func (c cliExtrasJournal) Created(rec dokployextras.Record) {
	if r := c.jr.recordBackup(rec.Kind, rec.Name, stateDone, rec.ID, rec.Cron, rec.Fingerprint); r != nil && rec.PauseFailed {
		c.jr.record(r, stepPaused, stateFailed)
	}
}

// Resumed needs nothing here: the file journal already holds the earlier record.
func (cliExtrasJournal) Resumed(dokployextras.Record) {}

func (c cliExtrasJournal) Failed(kind, name string) {
	c.jr.recordBackup(kind, name, stateFailed, "", "", "")
}

func printBackupImport(out io.Writer, imp *backupImport) {
	if imp == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "\nBackups: %d destination(s), %d schedule(s)\n", len(imp.Targets), len(imp.Schedules))
	for _, t := range imp.Targets {
		where := "bucket " + cleanDetail(t.Bucket)
		if t.Endpoint != "" {
			where = cleanDetail(t.Endpoint) + " " + where
		}
		if t.Region != "" {
			where += " region " + cleanDetail(t.Region)
		}
		_, _ = fmt.Fprintf(out, "  - target %s (from %q, %s, %s): %s\n", t.Name, t.SourceName, t.Provider, where, t.State)
		if t.Detail != "" {
			_, _ = fmt.Fprintf(out, "      %s\n", t.Detail)
		}
		if t.Endpoint != "" {
			_, _ = fmt.Fprintf(out, "      warning: %s\n", backupEndpointWarning)
		}
	}
	for _, s := range imp.Schedules {
		mode := "enabled"
		if s.Paused {
			mode = "paused"
		}
		_, _ = fmt.Fprintf(out, "  - schedule %s -> %s (%s, keep %d, %s): %s\n", s.Database, s.Target, cleanDetail(s.Cron), s.Retain, mode, s.State)
		if s.Detail != "" {
			_, _ = fmt.Fprintf(out, "      %s\n", s.Detail)
		}
	}
}

func journalRecord(r *journalResource) dokployextras.Record {
	return dokployextras.Record{Kind: r.Kind, Name: r.Name, ID: r.TargetID, Fingerprint: r.Fingerprint, Cron: r.Cron}
}

// planBackupUndo removes only what still reads back with the recorded id and shape.
func planBackupUndo(ctx context.Context, client *Client, r *journalResource) (action, reason string) {
	if remove, why := dokployextras.PlanUndo(ctx, client, journalRecord(r)); !remove {
		return undoSkip, why
	}
	return undoRemove, ""
}

func undoBackupResource(ctx context.Context, client *Client, r *journalResource) error {
	return dokployextras.Undo(ctx, client, journalRecord(r))
}
