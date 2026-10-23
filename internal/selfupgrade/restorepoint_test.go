package selfupgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPrunePlan(t *testing.T) {
	pts := []RestorePoint{{ID: "a", SizeBytes: 60}, {ID: "b", SizeBytes: 30}, {ID: "c", SizeBytes: 30}, {ID: "d", SizeBytes: 10}}
	tests := []struct {
		name     string
		count    int
		maxBytes int64
		want     []string
	}{
		{"no limits keeps all", 0, 0, nil},
		{"count cap", 2, 0, []string{"c", "d"}},
		{"size cap drops oldest", 0, 100, []string{"c", "d"}},
		{"newest stays when over size", 0, 10, []string{"b", "c", "d"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrunePlan(pts, tc.count, tc.maxBytes); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("PrunePlan = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyVerifyFailureRestoresBinaryAndDatabase(t *testing.T) {
	h := newFakeHost(t)
	h.startMigrates = 11
	d := h.deps()
	rp := NewRestorePoints(h.dir)
	d.RestorePoint = func(_ context.Context, a Attempt, snap string) (string, error) {
		pt, err := rp.Create(RestorePoint{ID: "rp-" + a.ID, Version: a.FromVersion, Schema: a.FromSchema, CreatedAt: a.SnapshotAt}, snap, h.bin, "")
		return pt.ID, err
	}
	d.Verify = func(context.Context, time.Duration, VerifyExpect) (string, error) {
		if h.read(h.bin) == "NEW" {
			return "", errors.New("only 0 of 2 agents reconnected")
		}
		return "ok", nil
	}
	got, err := Apply(context.Background(), d, req())
	if !errors.Is(err, ErrNotHealthy) {
		t.Fatalf("err = %v, want ErrNotHealthy", err)
	}
	if got.Outcome != OutcomeRolledBack || !got.DatabaseRestored {
		t.Fatalf("outcome %s restored %v, want rolled_back with the database restored", got.Outcome, got.DatabaseRestored)
	}
	if h.read(h.bin) != "OLD" || h.read(h.liveDB) != "db-v10" {
		t.Fatalf("binary %q db %q, want OLD and db-v10", h.read(h.bin), h.read(h.liveDB))
	}
	pt, err := rp.Get(got.RestorePointID)
	if err != nil {
		t.Fatalf("restore point not kept: %v", err)
	}
	if info, serr := os.Stat(rp.DBPath(pt.ID)); serr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot file mode %v err %v, want 0600", info, serr)
	}
}

func TestRestoreFromPointKeepsCompatibleDatabase(t *testing.T) {
	h := newFakeHost(t)
	d := h.deps()
	rp := NewRestorePoints(h.dir)
	h.write(filepath.Join(h.dir, "old-bin"), "OLD")
	if _, err := rp.Create(RestorePoint{ID: "rp-1", Version: "v0.4.0", Schema: 10, CreatedAt: time.Now()}, h.liveDB, filepath.Join(h.dir, "old-bin"), ""); err != nil {
		t.Fatal(err)
	}
	h.write(h.bin, "NEW")
	got, err := RestoreFromPoint(context.Background(), d, rp, "rp-1", "rp-att", "tester", "v0.5.0")
	if err != nil || got.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s err %v", got.Outcome, err)
	}
	if h.read(h.bin) != "OLD" || h.restored != 0 || got.DatabaseRestored {
		t.Fatalf("binary %q restores %d, want OLD binary and the compatible database kept", h.read(h.bin), h.restored)
	}
}
