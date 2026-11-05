package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrDryRunDiskShort means there is not room for a scratch copy of the database.
var ErrDryRunDiskShort = errors.New("not enough free disk for a scratch copy of the database")

// MigrationDryRunResult is what a dry run reports. Failures of the migrations
// themselves are data (OK false, Error set), not a Go error.
type MigrationDryRunResult struct {
	OK         bool                     `json:"ok"`
	Applied    []store.PendingMigration `json:"applied"`
	DurationMS int64                    `json:"duration_ms"`
	Error      string                   `json:"error,omitempty"`
	FromSchema int                      `json:"from_schema"`
	ToSchema   int                      `json:"to_schema"`
}

// MigrationDryRunOptions configures MigrationDryRun.
type MigrationDryRunOptions struct {
	// LiveDB is the control plane database; it is only read.
	LiveDB string
	// ScratchDir holds the temporary copy and should sit on the data volume.
	ScratchDir string
	// FreeBytes reports free space on the scratch volume; nil skips the guard.
	FreeBytes func() (int64, error)
	// Migrate applies migrations to the copy; nil uses the running binary's embedded set.
	Migrate func(ctx context.Context, copyPath string) ([]store.PendingMigration, error)
}

// dryRunDiskFactor is the free space needed as a multiple of the database size.
const dryRunDiskFactor = 2

// MigrationDryRun copies the live database, applies the pending migrations to
// the copy, checks its integrity and removes it. The live file is never written.
func MigrationDryRun(ctx context.Context, o MigrationDryRunOptions) (MigrationDryRunResult, error) {
	fi, err := os.Stat(o.LiveDB)
	if err != nil {
		return MigrationDryRunResult{}, fmt.Errorf("stat live database: %w", err)
	}
	if o.FreeBytes != nil {
		free, err := o.FreeBytes()
		if err != nil {
			return MigrationDryRunResult{}, fmt.Errorf("read free disk: %w", err)
		}
		if free < fi.Size()*dryRunDiskFactor {
			return MigrationDryRunResult{}, fmt.Errorf("%w: %d MiB free, %d MiB needed", ErrDryRunDiskShort, free>>20, (fi.Size()*dryRunDiskFactor)>>20)
		}
	}
	dir, err := os.MkdirTemp(o.ScratchDir, "migration-dryrun-")
	if err != nil {
		return MigrationDryRunResult{}, fmt.Errorf("create scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	copyPath := filepath.Join(dir, "copy.db")
	if err := store.SnapshotFile(ctx, o.LiveDB, copyPath); err != nil {
		return MigrationDryRunResult{}, err
	}
	res := MigrationDryRunResult{Applied: []store.PendingMigration{}}
	if st, err := store.ReadSchemaState(ctx, copyPath); err == nil {
		res.FromSchema = st.Schema
	}
	migrate := o.Migrate
	if migrate == nil {
		migrate = store.MigrateFile
	}
	started := time.Now()
	applied, err := migrate(ctx, copyPath)
	res.DurationMS = time.Since(started).Milliseconds()
	res.Applied = append(res.Applied, applied...)
	res.ToSchema = res.FromSchema
	if st, rerr := store.ReadSchemaState(ctx, copyPath); rerr == nil {
		res.ToSchema = st.Schema
	}
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	res.OK = true
	return res, nil
}
