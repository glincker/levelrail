package demo

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type databaseSpec struct {
	name, engine, version, project, node string
	backup                               bool
}

var databaseSpecs = []databaseSpec{
	{"pg-main", "postgres", "16", "proj_storefront", "node_worker_nyc_1", true},
	{"redis-cache", "redis", "7", "proj_storefront", "node_worker_nyc_1", false},
	{"pg-analytics", "postgres", "17", "proj_platform", "node_worker_sgp_1", true},
	{"legacy-mysql", "mysql", "8.0", "proj_internal_tools", "node_worker_sgp_1", false},
}

const demoBackupTargetID = "bt_demo_archive"

func (s *seeder) databases() error {
	target := store.BackupTarget{
		ID: demoBackupTargetID, Name: "Offsite archive", Provider: store.BackupProviderCustom,
		Endpoint: "https://s3.example.invalid", Region: "eu-central-1", Bucket: "acme-demo-backups",
		CreatedAt: s.stamp(s.at(historyDays, 0)),
	}
	if err := s.db.SaveBackupTarget(s.ctx, target); err != nil {
		return err
	}
	for _, d := range databaseSpecs {
		row := store.DesiredDatabase{Name: d.name, Engine: d.engine, Version: d.version}
		if err := s.db.SaveDesiredDatabase(s.ctx, row); err != nil {
			return fmt.Errorf("demo: save database %q: %w", d.name, err)
		}
		if err := s.db.UpdateDatabaseNode(s.ctx, d.name, d.node); err != nil {
			return err
		}
		if err := s.db.UpdateDatabaseProject(s.ctx, d.name, d.project); err != nil {
			return err
		}
		if err := s.db.SetDatabaseEnvironment(s.ctx, d.name, envID(d.project, "production")); err != nil {
			return err
		}
		if !d.backup {
			continue
		}
		if err := s.db.SetDatabaseBackupSchedule(s.ctx, d.name, demoBackupTargetID, "0 3 * * *", 14, 0); err != nil {
			return err
		}
		if err := s.backupHistory(d.name); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) backupHistory(name string) error {
	for day := historyDays; day >= 1; day-- {
		id := fmt.Sprintf("bkp_%s_%02d", name, day)
		started := s.at(day, 3*time.Hour)
		if err := s.db.StartBackupHistory(s.ctx, store.BackupHistory{
			ID: id, DatabaseName: name, TargetID: demoBackupTargetID,
			ObjectKey: fmt.Sprintf("%s/%s.dump.gz", name, started.Format("2006-01-02")), StartedAt: s.stamp(started),
		}); err != nil {
			return err
		}
		status, size, errMsg, sum := store.BackupStatusSucceeded, int64(180<<20+int64(s.rng.intn(40))<<20), "", s.rng.hex(32)
		if day == 6 {
			status, size, errMsg, sum = store.BackupStatusFailed, 0, "upload failed: connection reset by peer", ""
		}
		finished := s.stamp(started.Add(4 * time.Minute))
		if err := s.db.FinishBackupHistory(s.ctx, id, status, size, sum, errMsg, finished); err != nil {
			return err
		}
	}
	return nil
}
