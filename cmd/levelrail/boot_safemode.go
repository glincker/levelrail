package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/safemode"
	"github.com/GLINCKER/levelrail/internal/version"
)

// safeModeError carries a boot failure that should end in the recovery page, not a restart.
type safeModeError struct {
	entry safemode.Entry
	err   error
}

func (e *safeModeError) Error() string { return e.err.Error() }
func (e *safeModeError) Unwrap() error { return e.err }

// prepareBoot applies a staged restore, consults the boot ledger and checks the database file.
// A non-nil entry means this boot must go straight to safe mode.
func prepareBoot(ctx context.Context, logger *slog.Logger) (*safemode.Ledger, safemode.Policy, *safemode.Entry) {
	dataDir := dataDirFromEnv()
	if applied, err := integrity.ApplyPending(ctx, dataDir, storeFilename, time.Now()); err != nil {
		logger.Error("staged restore was not applied, booting with the current database", slog.String("error", err.Error()))
	} else if applied != nil {
		logger.Warn("restored the control plane database from a verified snapshot",
			slog.String("snapshot", applied.Pending.Name), slog.String("previous_database", applied.Aside))
	}

	ledger := safemode.NewLedger(dataDir)
	policy := safemode.PolicyFromEnv(os.Getenv)
	prev := ledger.Begin()
	if policy.SafeMode(prev.Failures) {
		return ledger, policy, &safemode.Entry{Reason: safemode.ReasonCrashLoop, Detail: prev.LastError, Failures: prev.Failures}
	}
	if d := policy.Backoff(prev.Failures); d > 0 {
		logger.Warn("previous boot did not become healthy, waiting before starting again",
			slog.Int("failed_boots", prev.Failures), slog.Duration("wait", d))
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
	}
	err := integrity.BootCheck(ctx, filepath.Join(dataDir, storeFilename))
	if errors.Is(err, integrity.ErrCorrupt) {
		ledger.NoteError(err)
		return ledger, policy, &safemode.Entry{Reason: safemode.ReasonDBCorrupt, Detail: err.Error(), Failures: prev.Failures}
	}
	if err != nil {
		logger.Warn("boot integrity check could not run", slog.String("error", err.Error()))
	}
	return ledger, policy, nil
}

// runSafeMode serves the recovery page until the operator recovers or the process is stopped.
func runSafeMode(ctx context.Context, stop context.CancelFunc, logger *slog.Logger, ledger *safemode.Ledger, policy safemode.Policy, entry safemode.Entry) error {
	dataDir := dataDirFromEnv()
	name := "Control plane"
	if b, err := loadBrand(); err == nil && b.Name != "" {
		name = b.Name
	}
	var restart atomic.Bool
	cfg := safemode.Config{
		DataDir: dataDir, Brand: name, Version: version.Version, Entry: entry,
		Backups: cpbackup.NewManager(nil, dataDir), Ledger: ledger, Logger: logger,
	}
	if restartSupervised(os.LookupEnv, fileExists) {
		cfg.Restart = func() { restart.Store(true); stop() }
	}
	srv, err := safemode.New(cfg)
	if err != nil {
		return err
	}
	logger.Error("entering safe mode", slog.String("reason", entry.Reason), slog.String("detail", entry.Detail), slog.Int("failed_boots", entry.Failures))
	serveErr := srv.Serve(ctx, httpAddr())
	if restart.Load() {
		return errRestartRequested
	}
	ledger.ExitSafeMode(policy.Threshold)
	return serveErr
}

// classifyStoreError turns a failed store open into a safe mode entry, leaving the downgrade refusal to its own path.
func classifyStoreError(err error, failures int) *safeModeError {
	reason := safemode.ReasonStoreFailed
	if errors.Is(err, integrity.ErrCorrupt) {
		reason = safemode.ReasonDBCorrupt
	} else if strings.Contains(err.Error(), "store: migrate") {
		reason = safemode.ReasonMigrationFailed
	}
	return &safeModeError{entry: safemode.Entry{Reason: reason, Detail: err.Error(), Failures: failures}, err: err}
}
