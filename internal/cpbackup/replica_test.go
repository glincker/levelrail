package cpbackup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newReplica(t *testing.T, e *drEnv) *Replicator {
	t.Helper()
	e.configure(nil)
	return &Replicator{Svc: e.svc, DBPath: filepath.Join(e.dir, "levelrail.db"), Interval: time.Second}
}

func TestReplicaIntervalParsing(t *testing.T) {
	tests := []struct {
		in   string
		set  bool
		want time.Duration
	}{
		{"", false, 0},
		{"junk", true, 0},
		{"-1s", true, 0},
		{"1s", true, minReplicaInterval},
		{"30s", true, 30 * time.Second},
	}
	for _, tc := range tests {
		got := ReplicaInterval(func(string) (string, bool) { return tc.in, tc.set })
		if got != tc.want {
			t.Errorf("ReplicaInterval(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestReplicaShipsOnlyWhenChangedAndRestores(t *testing.T) {
	e := newDREnv(t)
	r := newReplica(t, e)
	ctx := context.Background()

	if shipped, err := r.Tick(ctx); err != nil || !shipped {
		t.Fatalf("first tick shipped=%v err=%v, want a ship", shipped, err)
	}
	if shipped, err := r.Tick(ctx); err != nil || shipped {
		t.Fatalf("idle tick shipped=%v err=%v, want no ship", shipped, err)
	}

	if err := e.db.SaveDesiredService(ctx, store.DesiredService{Name: "after-ship", Image: "img:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(3 * time.Second)
	if shipped, err := r.Tick(ctx); err != nil || !shipped {
		t.Fatalf("tick after a write shipped=%v err=%v, want a ship", shipped, err)
	}
	st := r.Status()
	if st.LastShipAt == nil || st.LastError != "" || st.PendingSince != nil {
		t.Fatalf("status = %+v", st)
	}

	live := filepath.Join(t.TempDir(), "levelrail.db")
	b := newBucket(t, e.srv)
	if _, err := RestoreLatestReplica(ctx, b, e.settings().InstallID, []age.Identity{e.id}, RestoreOptions{LivePath: live}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := store.Open(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if _, err := restored.GetDesiredService(ctx, "after-ship"); err != nil {
		t.Fatalf("restored database lacks the write made before the last ship: %v", err)
	}
}

func TestReplicaFallsBackToOlderSlotWhenNewestIsCorrupt(t *testing.T) {
	e := newDREnv(t)
	r := newReplica(t, e)
	ctx := context.Background()
	for i, name := range []string{"one", "two"} {
		if err := e.db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		e.clock.Advance(time.Duration(i+1) * time.Minute)
		if shipped, err := r.Tick(ctx); err != nil || !shipped {
			t.Fatalf("tick %s shipped=%v err=%v", name, shipped, err)
		}
	}
	id := e.settings().InstallID
	flipBit(t, e, ReplicaKey(id, "b"))

	live := filepath.Join(t.TempDir(), "levelrail.db")
	if _, err := RestoreLatestReplica(ctx, newBucket(t, e.srv), id, []age.Identity{e.id}, RestoreOptions{LivePath: live}); err != nil {
		t.Fatalf("restore with one corrupt slot: %v", err)
	}
	restored, err := store.Open(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if _, err := restored.GetDesiredService(ctx, "one"); err != nil {
		t.Fatalf("older slot should hold the first write: %v", err)
	}
}

func TestReplicaNoReplicaYet(t *testing.T) {
	e := newDREnv(t)
	newReplica(t, e)
	_, err := RestoreLatestReplica(context.Background(), newBucket(t, e.srv), "inst-none", []age.Identity{e.id}, RestoreOptions{LivePath: filepath.Join(t.TempDir(), "x.db")})
	if !errors.Is(err, ErrNoReplica) {
		t.Fatalf("err = %v, want ErrNoReplica", err)
	}
}

func TestReplicaMeasuredShipTime(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement run, not a correctness check")
	}
	e := newDREnv(t)
	r := newReplica(t, e)
	ctx := context.Background()
	if _, err := e.db.ExecContext(ctx, `CREATE TABLE replica_bench (id INTEGER PRIMARY KEY, payload TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, rows := range []int{0, 50_000, 200_000} {
		if rows > 0 {
			if _, err := e.db.ExecContext(ctx, `WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c LIMIT ?) INSERT INTO replica_bench(payload) SELECT hex(randomblob(48)) FROM c`, rows); err != nil {
				t.Fatal(err)
			}
		}
		start := time.Now()
		if shipped, err := r.Tick(ctx); err != nil || !shipped {
			t.Fatalf("tick shipped=%v err=%v", shipped, err)
		}
		st := r.Status()
		t.Logf("db rows=%d shipped=%dKB ship=%v", rows, st.LastSizeBytes/1024, time.Since(start).Round(time.Millisecond))
	}
}
