package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDBHealth struct {
	used, limit int
	disk        float64
	err         error
}

func (f fakeDBHealth) DatabaseConnections(context.Context, string) (int, int, error) {
	return f.used, f.limit, f.err
}

func (f fakeDBHealth) DatabaseDiskUsedPercent(context.Context, string) (float64, error) {
	return f.disk, f.err
}

type fakeDBBackups []store.BackupHistory

func (f fakeDBBackups) ListBackupHistory(_ context.Context, _ string, limit int, _ *time.Time) ([]store.BackupHistory, error) {
	if len(f) > limit {
		return f[:limit], nil
	}
	return f, nil
}

func TestEvaluateDatabaseRule(t *testing.T) {
	now := time.Now()
	failed := store.BackupHistory{Status: store.BackupStatusFailed}
	ok := store.BackupHistory{Status: store.BackupStatusSucceeded}
	tests := []struct {
		name       string
		rule       Rule
		health     DatabaseHealthSource
		backups    DatabaseBackupSource
		wantFiring bool
	}{
		{"connections over", Rule{Kind: KindDatabaseConnections, Threshold: 80}, fakeDBHealth{used: 90, limit: 100}, nil, true},
		{"connections under", Rule{Kind: KindDatabaseConnections}, fakeDBHealth{used: 10, limit: 100}, nil, false},
		{"connections default threshold", Rule{Kind: KindDatabaseConnections}, fakeDBHealth{used: 85, limit: 100}, nil, true},
		{"stopped database is quiet", Rule{Kind: KindDatabaseConnections}, fakeDBHealth{err: ErrDatabaseHealthUnavailable}, nil, false},
		{"disk full", Rule{Kind: KindDatabaseDisk, Threshold: 90}, fakeDBHealth{disk: 95}, nil, true},
		{"disk fine", Rule{Kind: KindDatabaseDisk}, fakeDBHealth{disk: 40}, nil, false},
		{"backup failing", Rule{Kind: KindDatabaseBackupFailing}, nil, fakeDBBackups{failed, ok}, true},
		{"backup recovered", Rule{Kind: KindDatabaseBackupFailing}, nil, fakeDBBackups{ok, failed}, false},
		{"two failures needed", Rule{Kind: KindDatabaseBackupFailing, RestartCountThreshold: 2}, nil, fakeDBBackups{failed, ok}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.rule.BackupDatabaseName = "main"
			next, notice, err := EvaluateDatabaseRule(context.Background(), tc.health, tc.backups, tc.rule, now)
			if err != nil {
				t.Fatalf("EvaluateDatabaseRule: %v", err)
			}
			if next.Firing != tc.wantFiring {
				t.Errorf("Firing = %v, want %v", next.Firing, tc.wantFiring)
			}
			if tc.wantFiring && notice == "" {
				t.Error("firing rule produced no notice")
			}
		})
	}
}
