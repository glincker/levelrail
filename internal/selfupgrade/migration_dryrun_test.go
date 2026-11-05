package selfupgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// seedOlderDB builds a control plane database three migrations behind this binary.
func seedOlderDB(t *testing.T) (path string, behind int) {
	t.Helper()
	newest, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	all, err := store.PendingMigrationList(0, newest)
	if err != nil || len(all) < 4 {
		t.Fatalf("need at least 4 migrations: %d %v", len(all), err)
	}
	ceiling := all[len(all)-4].Version
	path = filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path, store.WithMigrationCeiling(ceiling))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path, 3
}

func liveSum(t *testing.T, path string) string {
	t.Helper()
	st, err := store.ReadSchemaState(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	_ = st
	_, sum, err := store.FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func TestMigrationDryRunAppliesToCopyOnly(t *testing.T) {
	live, behind := seedOlderDB(t)
	before := liveSum(t, live)
	scratch := t.TempDir()
	res, err := MigrationDryRun(context.Background(), MigrationDryRunOptions{LiveDB: live, ScratchDir: scratch})
	if err != nil || !res.OK {
		t.Fatalf("dry run = %+v, %v", res, err)
	}
	if len(res.Applied) != behind || res.ToSchema <= res.FromSchema {
		t.Fatalf("applied %d, schema %d -> %d", len(res.Applied), res.FromSchema, res.ToSchema)
	}
	if after := liveSum(t, live); after != before {
		t.Fatal("the live database changed")
	}
	if left, _ := os.ReadDir(scratch); len(left) != 0 {
		t.Fatalf("scratch copy left behind: %v", left)
	}
}

func TestMigrationDryRunReportsFailingMigration(t *testing.T) {
	live, _ := seedOlderDB(t)
	before := liveSum(t, live)
	newest, _ := store.MaxSchemaVersion()
	bad := []store.MigrationSpec{{Version: newest + 1, Name: "broken", SQL: "CREATE TABLE half (x); INSERT INTO missing_table VALUES (1);"}}
	res, err := MigrationDryRun(context.Background(), MigrationDryRunOptions{
		LiveDB: live, ScratchDir: t.TempDir(),
		Migrate: func(ctx context.Context, p string) ([]store.PendingMigration, error) {
			return store.MigrateFileWith(ctx, p, bad)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || !strings.Contains(res.Error, "broken") {
		t.Fatalf("failure not reported: %+v", res)
	}
	if liveSum(t, live) != before {
		t.Fatal("the live database changed after a failed dry run")
	}
}

func TestMigrationDryRunRefusesWhenDiskIsShort(t *testing.T) {
	live, _ := seedOlderDB(t)
	_, err := MigrationDryRun(context.Background(), MigrationDryRunOptions{
		LiveDB: live, ScratchDir: t.TempDir(), FreeBytes: func() (int64, error) { return 1, nil },
	})
	if !errors.Is(err, ErrDryRunDiskShort) {
		t.Fatalf("err = %v", err)
	}
}
