package dokployextras

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Record is what a journal keeps for one imported resource: names, ids and shapes, never a value.
type Record struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ID          string `json:"id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Cron        string `json:"cron,omitempty"`
	// PauseFailed marks a resource created but left running or enabled, kept so undo can remove it.
	PauseFailed bool `json:"pause_failed,omitempty"`
}

// Journal records what an apply created so a later run resumes and undo removes only that.
type Journal interface {
	FindCreated(kind, name string) (Record, bool)
	Created(r Record)
	Failed(kind, name string)
	// Resumed is called when an earlier run's record still matches, so it stays undoable.
	Resumed(r Record)
}

func findCreated(jr Journal, kind, name string) (Record, bool) {
	if jr == nil {
		return Record{}, false
	}
	return jr.FindCreated(kind, name)
}

func created(jr Journal, r Record) {
	if jr != nil {
		jr.Created(r)
	}
}

func resumed(jr Journal, r Record) {
	if jr != nil {
		jr.Resumed(r)
	}
}

func failed(jr Journal, kind, name string) {
	if jr != nil {
		jr.Failed(kind, name)
	}
}

func isNotFound(err error) bool {
	var apiErr *apiclient.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// FingerprintOf is the fingerprint of a target as the API returns it.
func FingerprintOf(t apiclient.BackupTargetResource) string {
	return TargetFingerprint(t.Provider, t.Endpoint, t.Bucket, t.Region)
}

// ChannelFingerprint identifies a channel by id and timestamps, so any edit changes it.
func ChannelFingerprint(ch apiclient.NotificationChannelResource) string {
	sum := sha256.Sum256([]byte(ch.Kind + "\n" + ch.Name + "\n" + ch.ID + "\n" + ch.CreatedAt + "\n" + ch.UpdatedAt))
	return fingerprintPrefix + hex.EncodeToString(sum[:])
}

func pending(state string) bool {
	return state == StateNeedsSecret || state == StateWouldCreate
}

// ApplyBackups creates through the same API calls as a manual target create and schedule
// set, so scoping, encryption of keys, validation and audit match a manual create.
func ApplyBackups(ctx context.Context, client *apiclient.Client, imp *BackupImport, jr Journal) {
	if imp == nil {
		return
	}
	existing, err := client.ListBackupTargets(ctx)
	if err != nil {
		for i := range imp.Targets {
			if pending(imp.Targets[i].State) {
				imp.Targets[i].State, imp.Targets[i].Detail = StateFailed, "could not list backup targets: "+CleanDetail(err.Error())
			}
		}
	}
	ids := map[string]string{}
	for _, t := range existing {
		ids[t.Name] = t.ID
	}
	for i := range imp.Targets {
		applyTarget(ctx, client, imp, &imp.Targets[i], ids, jr)
	}
	targets := map[string]*BackupTarget{}
	for i := range imp.Targets {
		targets[imp.Targets[i].Name] = &imp.Targets[i]
	}
	for i := range imp.Schedules {
		applySchedule(ctx, client, &imp.Schedules[i], targets[imp.Schedules[i].Target], jr)
	}
}

// resumeTarget reuses a target only when its recorded id still reads back with the
// recorded fingerprint. A 404 may mean hidden from this caller, so it is never re-created.
func resumeTarget(ctx context.Context, client *apiclient.Client, t *BackupTarget, r Record, jr Journal) {
	if r.ID == "" || r.Fingerprint == "" {
		t.State, t.Detail = StateReview, "the journal records this target without an id and fingerprint; check it by hand"
		return
	}
	got, err := client.GetBackupTarget(ctx, r.ID)
	switch {
	case err == nil && FingerprintOf(got) == r.Fingerprint:
		t.ID, t.State, t.Detail = got.ID, StateCreated, "created by this run earlier"
		resumed(jr, r)
	case err == nil:
		t.State, t.Detail = StateExists, "the target this run created changed since, left unchanged"
	default:
		t.State, t.Detail = StateReview, "cannot tell whether the target this run created still exists (not visible to this token); not created again"
	}
}

func applyTarget(ctx context.Context, client *apiclient.Client, imp *BackupImport, t *BackupTarget, ids map[string]string, jr Journal) {
	if !pending(t.State) {
		return
	}
	if r, ok := findCreated(jr, KindBackupTarget, t.Name); ok {
		resumeTarget(ctx, client, t, r, jr)
		return
	}
	if _, ok := ids[t.Name]; ok {
		t.State, t.Detail = StateExists, "a backup target with this name already exists and was left unchanged; its schedules are not imported"
		return
	}
	if !imp.UseKeys || !t.HasKeys || imp.Creds == nil {
		t.State, t.Detail = StateNeedsSecret, TargetHint(t)
		return
	}
	access, secret, err := imp.Creds(ctx, t.SourceID)
	if err != nil {
		t.State, t.Detail = StateNeedsSecret, "the source returned no usable keys; "+TargetHint(t)
		return
	}
	got, err := client.CreateBackupTarget(ctx, apiclient.CreateBackupTargetRequest{Name: t.Name, Provider: t.Provider, Endpoint: t.Endpoint,
		Region: t.Region, Bucket: t.Bucket, AccessKeyID: access, SecretAccessKey: secret})
	if err != nil {
		t.State, t.Detail = StateFailed, redactAll(err.Error(), access, secret)
		failed(jr, KindBackupTarget, t.Name)
		return
	}
	t.ID, t.State, t.Detail = got.ID, StateCreated, ""
	created(jr, Record{Kind: KindBackupTarget, Name: t.Name, ID: got.ID, Fingerprint: FingerprintOf(got)})
}

func applySchedule(ctx context.Context, client *apiclient.Client, s *BackupSchedule, t *BackupTarget, jr Journal) {
	if s.State != StateWouldCreate {
		return
	}
	if t == nil || t.State != StateCreated {
		s.State, s.Detail = StatePending, "bind it once the target exists: "+ScheduleHint(s, "<id>")
		return
	}
	db, err := client.GetDatabase(ctx, s.Database)
	switch {
	case isNotFound(err):
		s.State, s.Detail = StatePending, "database "+s.Database+" is not on the target yet; after importing it run: "+ScheduleHint(s, t.ID)
		return
	case err != nil:
		s.State, s.Detail = StateFailed, CleanDetail(err.Error())
		return
	case db.BackupSchedule != "":
		if r, ok := findCreated(jr, KindBackupSchedule, s.Database); ok && r.ID == db.BackupTargetID && r.Cron == db.BackupSchedule {
			s.State, s.Detail = StateCreated, "created by this run earlier"
			resumed(jr, r)
			return
		}
		s.State, s.Detail = StateExists, "the database already has a backup schedule, left unchanged"
		return
	}
	if _, err := client.SetBackupSchedule(ctx, s.Database, apiclient.SetBackupScheduleRequest{TargetID: t.ID, Schedule: s.Cron, Retain: s.Retain}); err != nil {
		s.State, s.Detail = StateFailed, CleanDetail(err.Error())
		failed(jr, KindBackupSchedule, s.Database)
		return
	}
	if s.Paused {
		if _, err := client.PauseBackupSchedule(ctx, s.Database, true); err != nil {
			pauseFailed(ctx, client, s, t.ID, err, jr)
			return
		}
	}
	s.State = StateCreated
	s.Detail = "bound to existing database " + s.Database + ", which this run did not create; check it is the right one"
	created(jr, Record{Kind: KindBackupSchedule, Name: s.Database, ID: t.ID, Cron: s.Cron})
}

// pauseFailed removes a schedule that could not be paused. If that fails too, the journal
// keeps it as created so undo can clear it, with the pause step failed.
func pauseFailed(ctx context.Context, client *apiclient.Client, s *BackupSchedule, targetID string, pauseErr error, jr Journal) {
	if clearErr := client.ClearBackupSchedule(ctx, s.Database); clearErr != nil {
		s.State, s.Detail = StateFailed, "could not pause the imported schedule or remove it, so the schedule may still be running: "+CleanDetail(errors.Join(pauseErr, clearErr).Error())
		created(jr, Record{Kind: KindBackupSchedule, Name: s.Database, ID: targetID, Cron: s.Cron, PauseFailed: true})
		return
	}
	s.State, s.Detail = StateFailed, "could not pause the imported schedule, so it was removed: "+CleanDetail(pauseErr.Error())
	failed(jr, KindBackupSchedule, s.Database)
}

// redactAll removes each value from a server message before it is shown.
func redactAll(msg string, values ...string) string {
	for _, v := range values {
		if len(v) >= 4 {
			msg = strings.ReplaceAll(msg, v, "<redacted>")
		}
	}
	return CleanDetail(msg)
}

// RedactNotify removes the notify URL and its path and query parts from a server message.
func RedactNotify(msg, notify string) string {
	if notify == "" {
		return CleanDetail(msg)
	}
	parts := []string{notify, url.QueryEscape(notify)}
	if u, err := url.Parse(notify); err == nil {
		parts = append(parts, u.RawQuery, u.EscapedPath(), u.Path)
		parts = append(parts, strings.Split(u.Path, "/")...)
		for _, vs := range u.Query() {
			parts = append(parts, vs...)
		}
	}
	return redactAll(msg, parts...)
}

// ApplyChannels creates through the same API call as a manual channel create, so scoping,
// URL validation, secret storage and audit match a manual create.
func ApplyChannels(ctx context.Context, client *apiclient.Client, imp *ChannelImport, jr Journal) {
	if imp == nil {
		return
	}
	existing, err := client.ListNotificationChannels(ctx)
	if err != nil {
		for i := range imp.Channels {
			if pending(imp.Channels[i].State) {
				imp.Channels[i].State, imp.Channels[i].Detail = StateFailed, "could not list channels: "+CleanDetail(err.Error())
			}
		}
		return
	}
	for i := range imp.Channels {
		applyChannel(ctx, client, imp, &imp.Channels[i], existing, jr)
	}
}

// resumeChannel reuses a channel only when its recorded id still lists with the recorded fingerprint.
func resumeChannel(c *Channel, r Record, existing []apiclient.NotificationChannelResource, jr Journal) {
	for _, ch := range existing {
		if ch.ID == r.ID && r.Fingerprint != "" && ChannelFingerprint(ch) == r.Fingerprint {
			c.ID, c.State, c.Detail = ch.ID, StateCreated, "created by this run earlier"
			resumed(jr, r)
			return
		}
	}
	c.State, c.Detail = StateReview, "cannot tell whether the channel this run created still exists unchanged; not created again"
}

func applyChannel(ctx context.Context, client *apiclient.Client, imp *ChannelImport, c *Channel, existing []apiclient.NotificationChannelResource, jr Journal) {
	if !pending(c.State) {
		return
	}
	if r, ok := findCreated(jr, KindChannel, c.Name); ok {
		resumeChannel(c, r, existing, jr)
		return
	}
	for _, ch := range existing {
		if ch.Name == c.Name {
			c.State, c.Detail = StateExists, "warning: a channel with this name already exists and was left unchanged"
			return
		}
	}
	notify := c.Recipient
	if c.Kind != "email" {
		if !imp.UseSecrets || !c.HasSecret || imp.Target == nil {
			c.State, c.Detail = StateNeedsSecret, ChannelHint(c)
			return
		}
		tgt, err := imp.Target(ctx, c.SourceID)
		if err != nil || tgt.URL == "" {
			c.State, c.Detail = StateNeedsSecret, "the source returned no usable token; "+ChannelHint(c)
			return
		}
		if tgt.Kind != c.SourceKind || tgt.Server != c.Server {
			c.State, c.Detail = StateReview, "source changed since the plan; run the import again"
			return
		}
		notify = tgt.URL
		if imp.CheckURL != nil {
			if err := imp.CheckURL(notify); err != nil {
				c.State, c.Detail = StateReview, "refused by the import network policy: "+RedactNotify(err.Error(), notify)
				return
			}
		}
	}
	enabled := c.Enabled
	got, err := client.CreateNotificationChannel(ctx, apiclient.CreateNotificationChannelRequest{Name: c.Name, Kind: c.Kind, NotifyURL: notify, Enabled: &enabled})
	if err != nil {
		c.State, c.Detail = StateFailed, RedactNotify(err.Error(), notify)
		failed(jr, KindChannel, c.Name)
		return
	}
	if got.Enabled && !enabled {
		unwantedEnabled(ctx, client, c, got, jr)
		return
	}
	c.ID, c.State, c.Detail = got.ID, StateCreated, ""
	created(jr, Record{Kind: KindChannel, Name: c.Name, ID: got.ID, Fingerprint: ChannelFingerprint(got)})
}

// unwantedEnabled deletes a channel the server created enabled against the request, so an
// import never starts sending. If the delete fails, the journal keeps it for undo.
func unwantedEnabled(ctx context.Context, client *apiclient.Client, c *Channel, got apiclient.NotificationChannelResource, jr Journal) {
	if err := client.DeleteNotificationChannel(ctx, got.ID); err != nil {
		c.ID, c.State = got.ID, StateFailed
		c.Detail = "the server created it enabled although disabled was requested and it could not be deleted; delete it with channels delete " + ShQuote(got.ID) + ": " + CleanDetail(err.Error())
		created(jr, Record{Kind: KindChannel, Name: c.Name, ID: got.ID, Fingerprint: ChannelFingerprint(got), PauseFailed: true})
		return
	}
	c.State, c.Detail = StateFailed, "the server created it enabled although disabled was requested, so it was deleted"
	failed(jr, KindChannel, c.Name)
}

// PlanUndo says whether a journaled resource may be removed: only when it still reads back
// by its recorded id with the recorded shape. A 404 may mean hidden, so it is never "gone".
func PlanUndo(ctx context.Context, client *apiclient.Client, r Record) (remove bool, reason string) {
	if r.Kind == KindChannel {
		list, err := client.ListNotificationChannels(ctx)
		return PlanChannelUndo(list, err, r)
	}
	if r.ID == "" || (r.Kind == KindBackupTarget && r.Fingerprint == "") || (r.Kind == KindBackupSchedule && r.Cron == "") {
		return false, "the journal holds no id and fingerprint for it, never removed"
	}
	switch r.Kind {
	case KindBackupTarget:
		got, err := client.GetBackupTarget(ctx, r.ID)
		switch {
		case isNotFound(err):
			return false, "not visible to this token by its recorded id, cannot tell whether it is gone; nothing removed"
		case err != nil:
			return false, "could not read it: " + CleanDetail(err.Error())
		case FingerprintOf(got) != r.Fingerprint:
			return false, "changed since the run, never removed"
		}
	case KindBackupSchedule:
		db, err := client.GetDatabase(ctx, r.Name)
		switch {
		case isNotFound(err):
			return false, "database not visible to this token, cannot tell whether the schedule is gone; nothing removed"
		case err != nil:
			return false, "could not read it: " + CleanDetail(err.Error())
		case db.BackupSchedule == "":
			return false, "the database has no backup schedule any more"
		case db.BackupTargetID != r.ID || db.BackupSchedule != r.Cron:
			return false, "the schedule changed since the run, never removed"
		}
	default:
		return false, "unknown kind"
	}
	return true, ""
}

// PlanChannelUndo decides on one channel from a list read once per undo run.
func PlanChannelUndo(list []apiclient.NotificationChannelResource, err error, r Record) (bool, string) {
	if r.ID == "" || r.Fingerprint == "" {
		return false, "the journal holds no id and fingerprint for it, never removed"
	}
	switch {
	case err != nil:
		return false, "could not list channels: " + CleanDetail(err.Error())
	case len(list) == 0:
		return false, "no channels are visible to this token, cannot tell whether it is gone; nothing removed"
	}
	for _, ch := range list {
		if ch.ID != r.ID {
			continue
		}
		if ChannelFingerprint(ch) != r.Fingerprint {
			return false, "changed since the run, never removed"
		}
		return true, ""
	}
	return false, "not visible to this token by its recorded id, cannot tell whether it is gone; nothing removed"
}

// Undo removes one journaled resource by its recorded id; call PlanUndo first.
func Undo(ctx context.Context, client *apiclient.Client, r Record) error {
	switch r.Kind {
	case KindChannel:
		return client.DeleteNotificationChannel(ctx, r.ID)
	case KindBackupSchedule:
		return client.ClearBackupSchedule(ctx, r.Name)
	default:
		return client.DeleteBackupTarget(ctx, r.ID)
	}
}
