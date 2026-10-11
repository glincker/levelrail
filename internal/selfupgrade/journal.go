package selfupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// JournalDir is the directory under the data dir that holds attempt files.
const JournalDir = "self-upgrade"

// Journal persists attempts as one JSON file each. It is the source of truth
// while an attempt runs, because the database may be swapped underneath it.
type Journal struct {
	Dir string
}

// NewJournal returns the journal under dataDir.
func NewJournal(dataDir string) *Journal {
	return &Journal{Dir: filepath.Join(dataDir, JournalDir)}
}

func (j *Journal) path(id string) string { return filepath.Join(j.Dir, id+".json") }

// Save writes a atomically with owner-only permissions.
func (j *Journal) Save(a Attempt) error {
	if !validID(a.ID) {
		return fmt.Errorf("invalid attempt id %q", a.ID)
	}
	if err := os.MkdirAll(j.Dir, 0o700); err != nil {
		return fmt.Errorf("create journal directory: %w", err)
	}
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("encode attempt: %w", err)
	}
	tmp := j.path(a.ID) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write attempt journal: %w", err)
	}
	if err := os.Rename(tmp, j.path(a.ID)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install attempt journal: %w", err)
	}
	return nil
}

// List returns every readable attempt, oldest first. Unreadable files are
// skipped: a half-written journal must not block an upgrade.
func (j *Journal) List() ([]Attempt, error) {
	entries, err := os.ReadDir(j.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read journal directory: %w", err)
	}
	var out []Attempt
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(j.Dir, e.Name())) //nolint:gosec // entry of the journal directory
		if err != nil {
			continue
		}
		var a Attempt
		if json.Unmarshal(raw, &a) != nil || !validID(a.ID) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].StartedAt.Before(out[k].StartedAt) })
	return out, nil
}

// Remove deletes an attempt's file.
func (j *Journal) Remove(id string) {
	if validID(id) {
		_ = os.Remove(j.path(id))
	}
}

// Running returns attempts that never reached a final outcome.
func (j *Journal) Running() ([]Attempt, error) {
	all, err := j.List()
	if err != nil {
		return nil, err
	}
	var out []Attempt
	for _, a := range all {
		if a.Outcome == OutcomeRunning {
			out = append(out, a)
		}
	}
	return out, nil
}

func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// AttemptStore is the persistence ImportJournal needs; *store.DB satisfies it.
type AttemptStore interface {
	UpsertSelfUpgradeAttempt(ctx context.Context, a store.SelfUpgradeAttempt) error
}

// ImportJournal copies every journaled attempt into the database and removes
// the files of finished ones. Running attempts stay on disk so a recovery can
// still read their paths. It returns how many rows were written.
func ImportJournal(ctx context.Context, j *Journal, db AttemptStore) (int, error) {
	all, err := j.List()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range all {
		if err := db.UpsertSelfUpgradeAttempt(ctx, ToStore(a)); err != nil {
			return n, err
		}
		n++
		if a.Outcome != OutcomeRunning {
			j.Remove(a.ID)
		}
	}
	return n, nil
}

// ToStore converts an attempt to its database row.
func ToStore(a Attempt) store.SelfUpgradeAttempt {
	row := store.SelfUpgradeAttempt{
		ID: a.ID, FromVersion: a.FromVersion, ToVersion: a.ToVersion, FromSchema: a.FromSchema, ToSchema: a.ToSchema,
		Initiator: a.Initiator, Outcome: a.Outcome, FailedStep: a.FailedStep, Error: a.Error, BackupName: a.BackupName,
		AckedJSON: a.AckedJSON(), StepsJSON: a.StepsJSON(), StartedAt: a.StartedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if !a.FinishedAt.IsZero() {
		row.FinishedAt = a.FinishedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return row
}
