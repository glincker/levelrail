package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// Backup import outcomes. No type here has a credential field: keys go from the source
// straight into the target's create call and are never stored, printed or journaled.
const (
	backupWouldCreate = "would_create"
	backupNeedsSecret = "needs_secret"
	backupCreated     = "created"
	backupExists      = "exists"
	backupPending     = "pending"
	backupSkipped     = "skipped"
	backupReview      = "review"
	backupFailed      = "failed"

	journalKindBackupTarget   = "backup_target"
	journalKindBackupSchedule = "backup_schedule"

	envMigrateBackupLimit     = "APP_MIGRATE_BACKUP_LIMIT"
	defaultMigrateBackupLimit = 200
	ceilingMigrateBackupLimit = 5000

	backupEndpointWarning = "the server connects to this endpoint from its own network: confirm it before enabling schedules"
)

// s3BucketRE is the S3 bucket naming rule; anything else is never put into a create call or a hint.
var s3BucketRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

type backupImport struct {
	Targets   []importedBackupTarget   `json:"targets"`
	Schedules []importedBackupSchedule `json:"schedules"`
	// useKeys and creds are set only for --apply with --include-secret-values.
	useKeys bool
	creds   func(ctx context.Context, sourceID string) (string, string, error)
}

type importedBackupTarget struct {
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Endpoint   string `json:"endpoint,omitempty"`
	Region     string `json:"region,omitempty"`
	Bucket     string `json:"bucket"`
	ID         string `json:"id,omitempty"`
	State      string `json:"state"`
	Detail     string `json:"detail,omitempty"`
	sourceID   string
	hasKeys    bool
}

type importedBackupSchedule struct {
	Database string `json:"database"`
	Target   string `json:"target"`
	Cron     string `json:"cron"`
	Retain   int    `json:"retain,omitempty"`
	// Paused is true unless --enable-backup-schedules was passed and the source schedule was on.
	Paused bool   `json:"paused"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type backupPlanOptions struct {
	enableSchedules bool
	limit           int
	// checkEndpoint is the import network policy check; tests swap it to avoid DNS.
	checkEndpoint func(string) error
}

func backupPlanOptionsFromEnv(ctx context.Context, enable bool, lookupEnv func(string) (string, bool)) backupPlanOptions {
	policy := platformimport.NetworkPolicy{AllowPrivate: envIsTrue(lookupEnv, platformimport.AllowPrivateEnv), AllowLoopback: envIsTrue(lookupEnv, platformimport.AllowLoopbackEnv)}
	return backupPlanOptions{enableSchedules: enable,
		limit:         envPositiveInt(lookupEnv, envMigrateBackupLimit, defaultMigrateBackupLimit, ceilingMigrateBackupLimit),
		checkEndpoint: func(raw string) error { return platformimport.ValidateDestinationEndpoint(ctx, raw, policy) }}
}

// shQuote single-quotes a value so a printed hint command is safe to paste into a shell.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// importName maps a source name onto a journal-safe name; ok is false for names a person must pick.
func importName(raw string) (string, bool) {
	if strings.Trim(invalidNameChars.ReplaceAllString(strings.ToLower(raw), ""), "-") == "" {
		return "", false
	}
	name := sanitizeServiceName(raw)
	return name, journalNameRE.MatchString(name)
}

func backupTargetFingerprint(provider, endpoint, bucket, region string) string {
	sum := sha256.Sum256([]byte(provider + "\n" + endpoint + "\n" + bucket + "\n" + region))
	return journalFingerprintPfx + hex.EncodeToString(sum[:])
}

func fingerprintOf(t apiclient.BackupTargetResource) string {
	return backupTargetFingerprint(t.Provider, t.Endpoint, t.Bucket, t.Region)
}

// mapBackupProvider picks the target provider from the endpoint host, since source provider labels vary.
func mapBackupProvider(endpoint string) (provider, keptEndpoint string) {
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	switch {
	case host == "":
		return "aws", ""
	case strings.HasSuffix(host, ".r2.cloudflarestorage.com"):
		return "r2", endpoint
	case host == "s3.amazonaws.com" || strings.HasSuffix(host, ".amazonaws.com"):
		return "aws", ""
	default:
		return "custom", endpoint
	}
}

func planBackupTarget(d platformimport.BackupDestination, o backupPlanOptions, budget int, used map[string]bool) importedBackupTarget {
	t := importedBackupTarget{SourceName: d.Name, Region: d.Region, Bucket: d.Bucket, sourceID: d.SourceID, hasKeys: d.HasCredentials, State: backupNeedsSecret}
	t.Provider, t.Endpoint = mapBackupProvider(d.Endpoint)
	name, nameOK := importName(d.Name)
	t.Name = name
	var endpointErr error
	if d.Endpoint != "" && o.checkEndpoint != nil {
		endpointErr = o.checkEndpoint(d.Endpoint)
	}
	switch {
	case budget <= 0:
		t.State, t.Detail = backupSkipped, "over the per-run backup import limit ("+envMigrateBackupLimit+")"
	case !nameOK:
		t.State, t.Detail = backupReview, "the source name is blank, symbols only or over 63 characters; create this target by hand"
	case used[t.Name]:
		t.State, t.Detail = backupSkipped, "another source destination maps to the same name; create this one by hand"
	case !s3BucketRE.MatchString(t.Bucket):
		t.State, t.Detail = backupReview, "the bucket name is not a valid S3 bucket name"
	case hasControlChar(t.Region) || hasControlChar(d.Endpoint):
		t.State, t.Detail = backupReview, "the region or endpoint holds control characters"
	case endpointErr != nil:
		t.State, t.Detail = backupReview, "endpoint refused by the import network policy: "+cleanDetail(endpointErr.Error())
	case t.Provider == "custom" && t.Region == "":
		t.State, t.Detail = backupReview, "a custom endpoint needs a region; create it with backup-targets create --region"
	}
	return t
}

// planDokployBackups is pure apart from the endpoint check: names, endpoints and buckets only.
func planDokployBackups(disc *platformimport.Discovery, o backupPlanOptions) *backupImport {
	if len(disc.Destinations) == 0 && !discoveryHasBackups(disc) {
		return nil
	}
	imp := &backupImport{}
	used := map[string]bool{}
	budget := o.limit
	bySource := map[string]int{}
	for _, d := range disc.Destinations {
		t := planBackupTarget(d, o, budget, used)
		if t.Name != "" {
			used[t.Name] = true
		}
		budget--
		bySource[d.SourceID] = len(imp.Targets)
		imp.Targets = append(imp.Targets, t)
	}
	scheduled := map[string]bool{}
	for _, db := range disc.Databases {
		name, nameOK := importName(db.Name)
		for _, b := range db.Backups {
			s := importedBackupSchedule{Database: name, Cron: b.Schedule, Retain: b.KeepLatest, Paused: !o.enableSchedules || !b.Enabled, State: backupWouldCreate}
			idx, ok := bySource[b.DestinationID]
			if ok {
				s.Target = imp.Targets[idx].Name
			}
			_, cronErr := cronexpr.Parse(b.Schedule)
			switch {
			case budget <= 0:
				s.State, s.Detail = backupSkipped, "over the per-run backup import limit ("+envMigrateBackupLimit+")"
			case !nameOK:
				s.State, s.Detail = backupReview, "the database name is blank, symbols only or over 63 characters"
			case !ok:
				s.State, s.Detail = backupReview, "its destination was not found on the source"
			case cronErr != nil:
				s.State, s.Detail = backupReview, "cron expression rejected: "+cleanDetail(cronErr.Error())
			case scheduled[name]:
				s.State, s.Detail = backupSkipped, "a database keeps one schedule here; the first source schedule was kept"
			case imp.Targets[idx].State == backupSkipped || imp.Targets[idx].State == backupReview:
				s.State, s.Detail = backupPending, "its backup target is not imported"
			}
			if s.State == backupWouldCreate {
				scheduled[name] = true
			}
			budget--
			imp.Schedules = append(imp.Schedules, s)
		}
	}
	return imp
}

func discoveryHasBackups(disc *platformimport.Discovery) bool {
	for _, db := range disc.Databases {
		if len(db.Backups) > 0 {
			return true
		}
	}
	return false
}

func createTargetHint(t *importedBackupTarget) string {
	cmd := "keys not imported; run: backup-targets create --name " + shQuote(t.Name) + " --provider " + shQuote(t.Provider) + " --bucket " + shQuote(t.Bucket)
	if t.Endpoint != "" {
		cmd += " --endpoint " + shQuote(t.Endpoint)
	}
	if t.Region != "" {
		cmd += " --region " + shQuote(t.Region)
	}
	return cmd + " --access-key-id <id> --secret-access-key <key>"
}

func scheduleHint(s *importedBackupSchedule, targetID string) string {
	cmd := "backups schedule set " + shQuote(s.Database) + " --target " + shQuote(targetID) + " --cron " + shQuote(s.Cron)
	if s.Retain > 0 {
		cmd += fmt.Sprintf(" --retain %d", s.Retain)
	}
	return cmd
}

// finishPlan marks which targets an apply would create with keys and prints the manual
// command for the rest; the command carries placeholders, never a value.
func (imp *backupImport) finishPlan(useKeys bool) {
	if imp == nil {
		return
	}
	imp.useKeys = useKeys
	for i := range imp.Targets {
		t := &imp.Targets[i]
		if t.State != backupNeedsSecret {
			continue
		}
		if useKeys && t.hasKeys {
			t.State = backupWouldCreate
			continue
		}
		t.Detail = createTargetHint(t)
	}
}

func pendingTarget(state string) bool {
	return state == backupNeedsSecret || state == backupWouldCreate
}

// applyBackupImport creates through the same API calls as backup-targets create and backups
// schedule set, so scoping, encryption of keys, validation and audit match a manual create.
func applyBackupImport(ctx context.Context, client *Client, imp *backupImport, jr *applyJournal) {
	if imp == nil {
		return
	}
	existing, err := client.ListBackupTargets(ctx)
	if err != nil {
		for i := range imp.Targets {
			if pendingTarget(imp.Targets[i].State) {
				imp.Targets[i].State, imp.Targets[i].Detail = backupFailed, "could not list backup targets: "+cleanDetail(err.Error())
			}
		}
	}
	ids := map[string]string{}
	for _, t := range existing {
		ids[t.Name] = t.ID
	}
	for i := range imp.Targets {
		applyBackupTarget(ctx, client, imp, &imp.Targets[i], ids, jr)
	}
	targets := map[string]*importedBackupTarget{}
	for i := range imp.Targets {
		targets[imp.Targets[i].Name] = &imp.Targets[i]
	}
	for i := range imp.Schedules {
		applyBackupSchedule(ctx, client, &imp.Schedules[i], targets[imp.Schedules[i].Target], jr)
	}
}

// resumeBackupTarget reuses a target only when its recorded id still reads back with the
// recorded fingerprint. A 404 may mean hidden from this token, so it is never re-created.
func resumeBackupTarget(ctx context.Context, client *Client, t *importedBackupTarget, r *journalResource) {
	if r.TargetID == "" || r.Fingerprint == "" {
		t.State, t.Detail = backupReview, "the journal records this target without an id and fingerprint; check it by hand"
		return
	}
	got, err := client.GetBackupTarget(ctx, r.TargetID)
	switch {
	case err == nil && fingerprintOf(got) == r.Fingerprint:
		t.ID, t.State, t.Detail = got.ID, backupCreated, "created by this run earlier"
	case err == nil:
		t.State, t.Detail = backupExists, "the target this run created changed since, left unchanged"
	default:
		t.State, t.Detail = backupReview, "cannot tell whether the target this run created still exists (not visible to this token); not created again"
	}
}

func applyBackupTarget(ctx context.Context, client *Client, imp *backupImport, t *importedBackupTarget, ids map[string]string, jr *applyJournal) {
	if !pendingTarget(t.State) {
		return
	}
	if r := jr.findCreated(journalKindBackupTarget, t.Name); r != nil {
		resumeBackupTarget(ctx, client, t, r)
		return
	}
	if _, ok := ids[t.Name]; ok {
		t.State, t.Detail = backupExists, "a backup target with this name already exists and was left unchanged; its schedules are not imported"
		return
	}
	if !imp.useKeys || !t.hasKeys || imp.creds == nil {
		t.State, t.Detail = backupNeedsSecret, createTargetHint(t)
		return
	}
	access, secret, err := imp.creds(ctx, t.sourceID)
	if err != nil {
		t.State, t.Detail = backupNeedsSecret, "the source returned no usable keys; "+createTargetHint(t)
		return
	}
	created, err := client.CreateBackupTarget(ctx, createBackupTargetRequest{Name: t.Name, Provider: t.Provider, Endpoint: t.Endpoint,
		Region: t.Region, Bucket: t.Bucket, AccessKeyID: access, SecretAccessKey: secret})
	if err != nil {
		t.State, t.Detail = backupFailed, cleanDetail(err.Error())
		jr.recordBackup(journalKindBackupTarget, t.Name, stateFailed, "", "", "")
		return
	}
	t.ID, t.State, t.Detail = created.ID, backupCreated, ""
	jr.recordBackup(journalKindBackupTarget, t.Name, stateDone, created.ID, "", fingerprintOf(created))
}

func applyBackupSchedule(ctx context.Context, client *Client, s *importedBackupSchedule, t *importedBackupTarget, jr *applyJournal) {
	if s.State != backupWouldCreate {
		return
	}
	if t == nil || t.State != backupCreated {
		s.State, s.Detail = backupPending, "bind it once the target exists: "+scheduleHint(s, "<id>")
		return
	}
	db, err := client.GetDatabase(ctx, s.Database)
	switch {
	case isAPINotFound(err):
		s.State, s.Detail = backupPending, "database "+s.Database+" is not on the target yet; after importing it run: "+scheduleHint(s, t.ID)
		return
	case err != nil:
		s.State, s.Detail = backupFailed, cleanDetail(err.Error())
		return
	case db.BackupSchedule != "":
		if r := jr.findCreated(journalKindBackupSchedule, s.Database); r != nil && r.TargetID == db.BackupTargetID && r.Cron == db.BackupSchedule {
			s.State, s.Detail = backupCreated, "created by this run earlier"
			return
		}
		s.State, s.Detail = backupExists, "the database already has a backup schedule, left unchanged"
		return
	}
	if _, err := client.SetBackupSchedule(ctx, s.Database, setBackupScheduleRequest{TargetID: t.ID, Schedule: s.Cron, Retain: s.Retain}); err != nil {
		s.State, s.Detail = backupFailed, cleanDetail(err.Error())
		jr.recordBackup(journalKindBackupSchedule, s.Database, stateFailed, "", "", "")
		return
	}
	if s.Paused {
		if _, err := client.PauseBackupSchedule(ctx, s.Database, true); err != nil {
			pauseFailed(ctx, client, s, t.ID, err, jr)
			return
		}
	}
	s.State = backupCreated
	s.Detail = "bound to existing database " + s.Database + ", which this run did not create; check it is the right one"
	jr.recordBackup(journalKindBackupSchedule, s.Database, stateDone, t.ID, s.Cron, "")
}

// pauseFailed removes a schedule that could not be paused. If that fails too, the journal
// keeps it as created so undo can clear it, with the pause step failed.
func pauseFailed(ctx context.Context, client *Client, s *importedBackupSchedule, targetID string, pauseErr error, jr *applyJournal) {
	if clearErr := client.ClearBackupSchedule(ctx, s.Database); clearErr != nil {
		s.State, s.Detail = backupFailed, "could not pause the imported schedule or remove it, so the schedule may still be running: "+cleanDetail(errors.Join(pauseErr, clearErr).Error())
		r := jr.recordBackup(journalKindBackupSchedule, s.Database, stateDone, targetID, s.Cron, "")
		if r != nil {
			jr.record(r, stepPaused, stateFailed)
		}
		return
	}
	s.State, s.Detail = backupFailed, "could not pause the imported schedule, so it was removed: "+cleanDetail(pauseErr.Error())
	jr.recordBackup(journalKindBackupSchedule, s.Database, stateFailed, "", "", "")
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

// planBackupUndo removes only what still reads back with the recorded id and shape. A 404 may
// mean hidden from this token, so it is reported as unknown, never as already gone.
func planBackupUndo(ctx context.Context, client *Client, r *journalResource) (action, reason string) {
	if r.TargetID == "" || (r.Kind == journalKindBackupTarget && r.Fingerprint == "") || (r.Kind == journalKindBackupSchedule && r.Cron == "") {
		return undoSkip, "the journal holds no id and fingerprint for it, never removed"
	}
	switch r.Kind {
	case journalKindBackupTarget:
		got, err := client.GetBackupTarget(ctx, r.TargetID)
		switch {
		case isAPINotFound(err):
			return undoSkip, "not visible to this token by its recorded id, cannot tell whether it is gone; nothing removed"
		case err != nil:
			return undoSkip, "could not read it: " + cleanDetail(err.Error())
		case fingerprintOf(got) != r.Fingerprint:
			return undoSkip, "changed since the run, never removed"
		}
	case journalKindBackupSchedule:
		db, err := client.GetDatabase(ctx, r.Name)
		switch {
		case isAPINotFound(err):
			return undoSkip, "database not visible to this token, cannot tell whether the schedule is gone; nothing removed"
		case err != nil:
			return undoSkip, "could not read it: " + cleanDetail(err.Error())
		case db.BackupSchedule == "":
			return undoSkip, "the database has no backup schedule any more"
		case db.BackupTargetID != r.TargetID || db.BackupSchedule != r.Cron:
			return undoSkip, "the schedule changed since the run, never removed"
		}
	default:
		return undoSkip, "unknown kind"
	}
	return undoRemove, ""
}

func undoBackupResource(ctx context.Context, client *Client, r *journalResource) error {
	if r.Kind == journalKindBackupSchedule {
		return client.ClearBackupSchedule(ctx, r.Name)
	}
	return client.DeleteBackupTarget(ctx, r.TargetID)
}
