package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDBLive struct {
	fakeDBHealth
	up bool
}

func (f fakeDBLive) DatabaseUp(context.Context, string) (bool, error) { return f.up, nil }

func TestEvaluateDatabaseDownAndBackupStale(t *testing.T) {
	now := time.Now()
	old := store.BackupHistory{Status: store.BackupStatusSucceeded, FinishedAt: now.Add(-48 * time.Hour).UTC().Format(time.RFC3339)}
	fresh := store.BackupHistory{Status: store.BackupStatusSucceeded, FinishedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339)}
	cases := []struct {
		name string
		rule Rule
		live DatabaseHealthSource
		bk   DatabaseBackupSource
		fire bool
	}{
		{"down", Rule{Kind: KindDatabaseDown}, fakeDBLive{up: false}, nil, true},
		{"up", Rule{Kind: KindDatabaseDown}, fakeDBLive{up: true}, nil, false},
		{"no liveness probe", Rule{Kind: KindDatabaseDown}, fakeDBHealth{}, nil, false},
		{"stale", Rule{Kind: KindDatabaseBackupStale}, nil, fakeDBBackups{old}, true},
		{"fresh", Rule{Kind: KindDatabaseBackupStale}, nil, fakeDBBackups{fresh, old}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, notice, err := EvaluateDatabaseRule(context.Background(), tc.live, tc.bk, tc.rule, now)
			if err != nil {
				t.Fatal(err)
			}
			if out.Firing != tc.fire || (notice != "") != tc.fire {
				t.Fatalf("firing=%v notice=%q, want fire=%v", out.Firing, notice, tc.fire)
			}
		})
	}
}
