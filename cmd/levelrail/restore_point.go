package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/rollback"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	envRestorePoints      = "APP_UPGRADE_RESTORE_POINTS"
	envRestorePointsMaxMB = "APP_UPGRADE_RESTORE_POINTS_MAX_MB"
	envVerifyWindow       = "APP_UPGRADE_VERIFY_WINDOW"
	defaultVerifyWindow   = 3 * time.Minute
	unitDir               = "/etc/systemd/system"
	agentPollInterval     = 2 * time.Second
)

// verifyWindow is how long a new release gets to pass verification.
func verifyWindow() time.Duration { return envDuration(envVerifyWindow, defaultVerifyWindow) }

func restoreRetention() (int, int64) {
	return envInt(envRestorePoints, selfupgrade.DefaultRestorePoints),
		int64(envInt(envRestorePointsMaxMB, selfupgrade.DefaultRestoreMaxMB)) << 20
}

// wireSafetyNet adds the restore point, the post-upgrade verification and the
// unit file restore to deps.
func wireSafetyNet(deps *selfupgrade.Deps, name, dataDir, live, healthBase string, logger *slog.Logger) {
	rp := selfupgrade.NewRestorePoints(dataDir)
	unitPath := filepath.Join(unitDir, name+".service")
	deps.RestorePoint = func(_ context.Context, a selfupgrade.Attempt, snapshotPath string) (string, error) {
		pt, err := rp.Create(selfupgrade.RestorePoint{
			ID: "rp-" + a.ID, Version: a.FromVersion, Schema: a.FromSchema, AttemptID: a.ID, CreatedAt: a.SnapshotAt,
		}, snapshotPath, deps.BinaryPath, unitPath)
		if err != nil {
			return "", err
		}
		count, maxBytes := restoreRetention()
		if dropped, perr := rp.Prune(count, maxBytes); perr != nil {
			logger.Warn("restore point pruning failed", slog.String("error", perr.Error()))
		} else if len(dropped) > 0 {
			logger.Info("old restore points removed", slog.Int("removed", len(dropped)))
		}
		return pt.ID, nil
	}
	deps.AgentsOnline = func(ctx context.Context) (int, error) { return store.CountOnlineNodes(ctx, live) }
	deps.Verify = func(ctx context.Context, window time.Duration, want selfupgrade.VerifyExpect) (string, error) {
		return verifyRelease(ctx, healthBase, live, window, want)
	}
	deps.RestoreUnit = func(ctx context.Context, saved string) error { return restoreUnit(ctx, saved, unitPath) }
}

// verifyRelease checks readiness (which includes the reconcile engine), that
// the database is at the release's schema head, and that agents came back.
func verifyRelease(ctx context.Context, healthBase, live string, window time.Duration, want selfupgrade.VerifyExpect) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()
	if err := rollback.WaitHealthy(ctx, healthBase, window); err != nil {
		return "", err
	}
	schema, err := rollback.DBSchemaVersion(ctx, live)
	if err != nil {
		return "", err
	}
	if want.Schema > 0 && schema != want.Schema {
		return "", fmt.Errorf("database is at schema %d but the new release expects %d", schema, want.Schema)
	}
	detail := fmt.Sprintf("readyz ok with the reconcile loop running, schema %d at head", schema)
	if want.AgentsOnline == 0 {
		return detail, nil
	}
	for {
		n, cerr := store.CountOnlineNodes(ctx, live)
		if cerr == nil && n >= want.AgentsOnline {
			return fmt.Sprintf("%s, %d of %d agents reconnected", detail, n, want.AgentsOnline), nil
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("only %d of %d agents reconnected within %s", n, want.AgentsOnline, window)
		case <-time.After(agentPollInterval):
		}
	}
}

func restoreUnit(ctx context.Context, saved, live string) error {
	want, err := os.ReadFile(saved) //nolint:gosec // inside the restore point directory
	if err != nil {
		return fmt.Errorf("read saved unit file: %w", err)
	}
	if have, rerr := os.ReadFile(live); rerr == nil && bytes.Equal(have, want) { //nolint:gosec // fixed systemd unit path
		return nil
	}
	if err := os.WriteFile(live, want, 0o644); err != nil { //nolint:gosec // systemd unit files are world-readable
		return fmt.Errorf("write unit file: %w", err)
	}
	if out, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %w: %s", err, string(bytes.TrimSpace(out)))
	}
	return nil
}

const restorePointUsage = `usage: sudo levelrail restore-point --id <restore point id> [flags]

Puts a pre-upgrade restore point back: the binary and unit file always, the
database only when the running one was migrated past the restore point (the
result says how many minutes of changes that discards). Refuses while an
upgrade is running. Run it on the host as root.

  --id ID             restore point to restore
  --initiator NAME    who asked, recorded in upgrade history
  --json              machine-readable result
  --stop-cmd / --start-cmd / --health-url   same meaning as for rollback
`

// runRestorePoint implements "levelrail restore-point".
func runRestorePoint(ctx context.Context, args []string, dataDir string, stdout io.Writer) error {
	fs := flag.NewFlagSet("restore-point", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	f := selfUpgradeFlags{}
	var id string
	fs.StringVar(&id, "id", "", "")
	fs.StringVar(&f.initiator, "initiator", "", "")
	fs.BoolVar(&f.asJSON, "json", false, "")
	fs.StringVar(&f.stopCmd, "stop-cmd", "", "")
	fs.StringVar(&f.startCmd, "start-cmd", "", "")
	fs.StringVar(&f.healthURL, "health-url", "", "")
	if err := fs.Parse(args); err != nil || id == "" {
		_, _ = fmt.Fprint(stdout, restorePointUsage)
		return errors.New("invalid flags")
	}
	if f.initiator == "" {
		f.initiator = hostActor()
	}
	if !initiatorPattern.MatchString(f.initiator) {
		return errors.New("--initiator may only contain letters, digits and ._@:-")
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	name := filepath.Base(exe)
	live := filepath.Join(dataDir, storeFilename)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	healthBase := f.healthURL
	if healthBase == "" {
		healthBase = "http://" + dashboardDialAddr(serviceHTTPAddr(ctx, name))
	}
	journal := selfupgrade.NewJournal(dataDir)
	deps := selfUpgradeDeps(f, exe, name, dataDir, live, healthBase, journal, logger)
	wireSafetyNet(&deps, name, dataDir, live, healthBase, logger)

	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	attempt, rerr := selfupgrade.RestoreFromPoint(ctx, deps, selfupgrade.NewRestorePoints(dataDir), id, newAttemptID(), f.initiator, version.Version)
	recordAttempt(ctx, live, journal, logger)
	if f.asJSON {
		_ = json.NewEncoder(stdout).Encode(attempt)
	} else {
		for _, s := range attempt.Steps {
			_, _ = fmt.Fprintf(stdout, "  [%s] %s %s\n", s.Status, s.Name, s.Detail)
		}
		_, _ = fmt.Fprintf(stdout, "%s: restore point %s (%s)\n", attempt.Outcome, id, attempt.ToVersion)
	}
	return rerr
}
