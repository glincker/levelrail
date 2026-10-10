package upgradehistory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the persistence Recorder needs; *store.DB satisfies it.
type Store interface {
	InsertUpgradeHistory(ctx context.Context, e store.UpgradeHistoryEntry) (store.UpgradeHistoryEntry, bool, error)
	LatestUpgradeHistory(ctx context.Context) (store.UpgradeHistoryEntry, bool, error)
	CompleteUpgradeNotes(ctx context.Context, id, notes, state string) error
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// Observed is what the migration step saw before this boot, when it ran.
type Observed struct {
	SchemaBefore int
	BackupName   string
}

// Recorder compares the running version with the last recorded one and
// appends a history row when they differ.
type Recorder struct {
	Store   Store
	Notes   NotesFetcher
	DataDir string
	Version string
	Logger  *slog.Logger
	Now     func() time.Time
	// PriorData reports that the database existed before this feature, so a
	// first row is "adopted" rather than a fresh install.
	PriorData bool
}

// Record writes at most one row for this boot. It never fails the caller's
// boot: errors are returned for logging only. The notes snapshot is fetched
// on a goroutine and stored when it completes.
func (r *Recorder) Record(ctx context.Context, schemaAfter int, obs Observed) (store.UpgradeHistoryEntry, bool, error) {
	now := r.now()
	last, hasLast, err := r.Store.LatestUpgradeHistory(ctx)
	if err != nil {
		return store.UpgradeHistoryEntry{}, false, fmt.Errorf("read last recorded version: %w", err)
	}
	if hasLast && last.ToVersion == r.Version {
		ConsumeMarker(r.DataDir, r.Version, now)
		return store.UpgradeHistoryEntry{}, false, nil
	}
	marker, hasMarker := ConsumeMarker(r.DataDir, r.Version, now)
	kind := Classify(last.ToVersion, r.Version, hasLast, hasMarker && marker.Method == MethodRollback)
	if kind == KindInstalled && r.PriorData {
		kind = KindAdopted
	}
	e := store.UpgradeHistoryEntry{
		Kind: kind, FromVersion: last.ToVersion, ToVersion: r.Version, Channel: ChannelOf(r.Version),
		SchemaBefore: -1, SchemaAfter: schemaAfter, OccurredAt: now,
		Initiator: InitiatorUnknown, Health: HealthBooted,
	}
	if hasLast {
		e.SchemaBefore = last.SchemaAfter
	}
	if obs.SchemaBefore >= 0 {
		e.SchemaBefore = obs.SchemaBefore
	}
	e.BackupName = obs.BackupName
	if hasMarker {
		e.Initiator = orDefault(marker.Initiator, InitiatorUnknown)
		e.Method = marker.Method
		if marker.BackupName != "" {
			e.BackupName = marker.BackupName
		}
		if marker.Channel != "" && e.Channel != "" {
			e.Channel = marker.Channel
		}
	}
	stored, inserted, err := r.Store.InsertUpgradeHistory(ctx, e)
	if err != nil || !inserted {
		return stored, false, err
	}
	r.audit(ctx, stored)
	if r.Notes != nil && IsRelease(stored.ToVersion) {
		go r.fillNotes(context.WithoutCancel(ctx), stored.ID, stored.ToVersion)
	} else {
		r.settle(ctx, stored.ID, "", store.UpgradeNotesUnavailable)
	}
	return stored, true, nil
}

func (r *Recorder) fillNotes(parent context.Context, id, tag string) {
	ctx, cancel := context.WithTimeout(parent, notesTimeout+time.Second)
	defer cancel()
	notes, err := r.Notes(ctx, tag)
	if err != nil || notes == "" {
		r.log().Info("upgrade history: release notes unavailable", slog.String("id", id), slog.String("version", tag))
		r.settle(ctx, id, "", store.UpgradeNotesUnavailable)
		return
	}
	r.settle(ctx, id, notes, store.UpgradeNotesFetched)
}

func (r *Recorder) settle(ctx context.Context, id, notes, state string) {
	if err := r.Store.CompleteUpgradeNotes(ctx, id, notes, state); err != nil {
		r.log().Warn("upgrade history: store release notes failed", slog.String("id", id), slog.String("error", err.Error()))
	}
}

func (r *Recorder) audit(ctx context.Context, e store.UpgradeHistoryEntry) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		r.log().Warn("upgrade history: audit id failed", slog.String("error", err.Error()))
		return
	}
	err = r.Store.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "boot", ActorName: "system", Ability: "system",
		Method: "SYSTEM", Path: fmt.Sprintf("%s: %s %s -> %s (id %s, initiator %s)", AuditActionRecorded, e.Kind, orDefault(e.FromVersion, "none"), e.ToVersion, e.ID, e.Initiator),
		StatusCode: 200, RemoteAddr: "local", CreatedAt: store.FormatAuditTime(r.now()), ClientKind: "system",
	})
	if err != nil {
		r.log().Warn("upgrade history: audit entry failed", slog.String("id", e.ID), slog.String("error", err.Error()))
	}
}

func (r *Recorder) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Recorder) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
