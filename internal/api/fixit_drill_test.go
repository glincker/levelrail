package api

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type drillListStore struct {
	BackupProtectionStore
	drills []store.BackupDrill
}

func (s drillListStore) ListBackupDrills(context.Context, string, string, string, int) ([]store.BackupDrill, error) {
	return s.drills, nil
}

func TestBackupDrillFindingUsesLatestDrillPerResource(t *testing.T) {
	db := func(status, errText string) store.BackupDrill {
		return store.BackupDrill{ResourceKind: store.BackupResourceKindDatabase, DatabaseName: "main", Status: status, Stage: "download", Error: errText}
	}
	tests := []struct {
		name        string
		drills      []store.BackupDrill
		wantFailing bool
		wantReason  string
	}{
		{"no drills", nil, false, ""},
		{"latest passed hides older failure", []store.BackupDrill{db(store.DrillStatusPassed, ""), db(store.DrillStatusFailed, "403")}, false, ""},
		{"latest failed with credentials error", []store.BackupDrill{db(store.DrillStatusFailed, "AccessDenied"), db(store.DrillStatusPassed, "")}, true, "credentials"},
		{"running is skipped", []store.BackupDrill{db(store.DrillStatusRunning, ""), db(store.DrillStatusFailed, "no space left on device")}, true, "disk space"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{backupProtectionStore: drillListStore{drills: tc.drills}}
			f, err := rt.backupDrillFinding(context.Background())
			if err != nil || f.Failing != tc.wantFailing || !strings.Contains(f.Reason, tc.wantReason) {
				t.Fatalf("finding = %+v, err %v", f, err)
			}
		})
	}
	if f, err := (&Router{}).backupDrillFinding(context.Background()); err != nil || f.Failing {
		t.Fatalf("unconfigured must not fail: %+v %v", f, err)
	}
}
