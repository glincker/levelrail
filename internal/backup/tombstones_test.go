package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type memTombstones struct {
	rows   []store.BackupTombstone
	nextID int64
}

func (m *memTombstones) AddBackupTombstone(_ context.Context, targetID, key, reason string) error {
	for i := range m.rows {
		if m.rows[i].TargetID == targetID && m.rows[i].ObjectKey == key {
			m.rows[i].Reason = reason
			return nil
		}
	}
	m.nextID++
	m.rows = append(m.rows, store.BackupTombstone{ID: m.nextID, TargetID: targetID, ObjectKey: key, Reason: reason})
	return nil
}

func (m *memTombstones) ListBackupTombstones(context.Context, int) ([]store.BackupTombstone, error) {
	return append([]store.BackupTombstone(nil), m.rows...), nil
}

func (m *memTombstones) ResolveBackupTombstone(_ context.Context, id int64) error {
	for i, r := range m.rows {
		if r.ID == id {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *memTombstones) BumpBackupTombstone(_ context.Context, id int64, reason string) error {
	for i := range m.rows {
		if m.rows[i].ID == id {
			m.rows[i].Attempts++
			m.rows[i].Reason = reason
		}
	}
	return nil
}

func TestPrunedObjectDeleteFailureLeavesTombstoneThatSweepClears(t *testing.T) {
	ts := &memTombstones{}
	deleter := &fakeDeleter{err: errors.New("bucket unreachable")}
	s := &Scheduler{Runner: &fakeScheduledRunner{}, Deleter: deleter, Tombstones: ts}
	ctx := context.Background()

	s.deletePrunedObjects(ctx, "db1", []store.PrunedBackup{{TargetID: "t1", ObjectKey: "db1/a.dump"}, {TargetID: "t1", ObjectKey: "db1/b.dump"}})
	if len(ts.rows) != 2 {
		t.Fatalf("want 2 tombstones after failed deletes, got %d", len(ts.rows))
	}

	s.sweepTombstones(ctx)
	if len(ts.rows) != 2 || ts.rows[0].Attempts != 1 {
		t.Fatalf("failed sweep must keep tombstones and count the attempt, got %+v", ts.rows)
	}

	deleter.err = nil
	s.sweepTombstones(ctx)
	if len(ts.rows) != 0 {
		t.Fatalf("successful sweep must clear tombstones, got %+v", ts.rows)
	}
}

func TestResolveFailureTombstonesObject(t *testing.T) {
	ts := &memTombstones{}
	runner := &fakeScheduledRunner{resolveErr: errors.New("target gone")}
	s := &Scheduler{Runner: runner, Deleter: &fakeDeleter{}, Tombstones: ts}

	s.deletePrunedObjects(context.Background(), "db1", []store.PrunedBackup{{TargetID: "t1", ObjectKey: "db1/a.dump"}})
	if len(ts.rows) != 1 || ts.rows[0].ObjectKey != "db1/a.dump" {
		t.Fatalf("want one tombstone for the unresolved object, got %+v", ts.rows)
	}
}

func TestRecordTombstoneIgnoresEmptyKeyAndNilStore(t *testing.T) {
	ts := &memTombstones{}
	recordTombstone(context.Background(), ts, nil, "t1", "", errors.New("x"))
	recordTombstone(context.Background(), nil, nil, "t1", "k", errors.New("x"))
	if len(ts.rows) != 0 {
		t.Fatalf("nothing should be recorded, got %+v", ts.rows)
	}
}

func TestFinishHistoryWithRetryRecoversFromTransientError(t *testing.T) {
	saved := finishRetryDelays
	finishRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	defer func() { finishRetryDelays = saved }()

	calls := 0
	err := finishHistoryWithRetry(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("database is locked")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("want success on third call, got err=%v calls=%d", err, calls)
	}

	calls = 0
	err = finishHistoryWithRetry(context.Background(), func() error {
		calls++
		return errors.New("still locked")
	})
	if err == nil || calls != 4 {
		t.Fatalf("want error after 4 calls, got err=%v calls=%d", err, calls)
	}
}
