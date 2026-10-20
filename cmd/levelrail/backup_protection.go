package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// drillSchedulerInterval is how often the drill scheduler looks for due work;
// each resource is drilled on its own, much longer, interval.
const drillSchedulerInterval = 15 * time.Minute

func backupDataDir() string {
	if d := os.Getenv("APP_DATA_DIR"); d != "" {
		return d
	}
	return defaultDataDir
}

// newVolumeBackupOptions builds the sealed volume backup behavior: encryption
// key, budgets and pre/post hooks resolved against the app's own container.
func newVolumeBackupOptions(db *store.DB, local docker.Runtime, registry *agent.Registry, logger *slog.Logger) (*backup.VolumeBackupOptions, error) {
	sealer, err := backup.LoadSealer(os.LookupEnv, backupDataDir())
	if err != nil {
		return nil, fmt.Errorf("load backup encryption key: %w", err)
	}
	opts := &backup.VolumeBackupOptions{Sealer: sealer, Policies: db, Logger: logger}
	opts.OptionsFromEnv(os.LookupEnv)
	opts.Quiesce = &backup.ContainerQuiescer{Resolve: func(ctx context.Context, serviceName string) (docker.Runtime, string, error) {
		svc, err := db.GetDesiredService(ctx, serviceName)
		if err != nil {
			return nil, "", fmt.Errorf("load app %q: %w", serviceName, err)
		}
		rt, err := resolveNodeTransport(local, registry, svc.NodeID)
		if err != nil {
			return nil, "", fmt.Errorf("reach node of %q: %w", serviceName, err)
		}
		name := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
		st, err := rt.InspectByName(ctx, name)
		if err != nil {
			return nil, "", fmt.Errorf("inspect container of %q: %w", serviceName, err)
		}
		if st == nil || !st.Running {
			return nil, "", fmt.Errorf("app %q is not running", serviceName)
		}
		return rt, st.ID, nil
	}}
	return opts, nil
}

// backupIdentities returns the age identities that decrypt sealed backups.
func backupIdentities(r *backup.Runner) []age.Identity {
	if r == nil || r.Volume == nil || r.Volume.Sealer == nil {
		return nil
	}
	return r.Volume.Sealer.Identities
}

// newBackupProtection builds the health, drill and restore-to service shared
// by the API and the drill scheduler.
func newBackupProtection(db *store.DB, secretsManager *secrets.Manager, client *docker.Client, runner *backup.Runner, logger *slog.Logger) *backup.Protection {
	var sealer *backup.Sealer
	if runner != nil && runner.Volume != nil {
		sealer = runner.Volume.Sealer
	}
	downloader := backup.S3Downloader{}
	drills := &backup.DrillRunner{
		Store:      db,
		Secrets:    secretsManager,
		Downloader: downloader,
		Prober:     backup.S3Prober{},
		Restorer:   &backup.ContainerVolumeRestorer{Runtime: client},
		Archiver:   &backup.ContainerVolumeArchiver{Runtime: client},
		Volumes:    client,
		Remover:    client,
		Runtime:    client,
		DBRestorer: &backup.ContainerRestorer{Runtime: client},
		DBInfo: func(ctx context.Context, name string) (string, string, error) {
			d, err := db.GetDesiredDatabase(ctx, name)
			if err != nil {
				return "", "", err
			}
			return d.Engine, d.Version, nil
		},
		ImageFor: database.ImageRef,
		Logger:   logger,
	}
	if sealer != nil {
		drills.Identities = sealer.Identities
	}
	return &backup.Protection{
		Store:         db,
		Drills:        drills,
		Prober:        backup.S3ProtectionProber{},
		Secrets:       secretsManager,
		Downloader:    downloader,
		RestoreStore:  db,
		Sealer:        sealer,
		DrillInterval: backup.DrillIntervalFromEnv(os.LookupEnv),
		Logger:        logger,
	}
}

// startBackupDrills runs the restore drill scheduler and registers the
// restore drill alert source.
func startBackupDrills(ctx context.Context, logger *slog.Logger, db *store.DB, prot *backup.Protection, engine *alerting.Engine) {
	engine.SetRestoreDrills(restoreDrillAlertSource{prot: prot})
	sched := &backup.DrillScheduler{
		Store:      db,
		Secrets:    prot.Secrets,
		Runner:     prot.Drills,
		Prober:     backup.S3Prober{},
		Downloader: backup.S3Downloader{},
		Protection: prot.Prober,
		Interval:   prot.DrillInterval,
		StaleAfter: backup.StaleRunningFromEnv(os.LookupEnv),
		Logger:     logger,
	}
	go func() {
		if err := sched.Run(ctx, drillSchedulerInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("backup drill scheduler stopped", slog.String("error", err.Error()))
		}
	}()
}

type restoreDrillAlertSource struct{ prot *backup.Protection }

// RestoreDrillProblems implements alerting.RestoreDrillSource.
func (s restoreDrillAlertSource) RestoreDrillProblems(ctx context.Context) ([]alerting.RestoreDrillProblem, error) {
	failed, err := s.prot.FailedDrills(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]alerting.RestoreDrillProblem, 0, len(failed))
	for _, f := range failed {
		at, _ := time.Parse(time.RFC3339, f.At)
		out = append(out, alerting.RestoreDrillProblem{Resource: f.Resource, Reason: f.Reason, At: at})
	}
	return out, nil
}
