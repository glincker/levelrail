package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

const danglingErr = `get backup target "bkt_gone": store: backup target not found`

type fakeVolumeSchedules struct {
	ServiceVolumeBackupScheduleStore
	cfg    store.ServiceVolumeBackupConfig
	getErr error
	setTo  string
}

func (f *fakeVolumeSchedules) GetServiceVolumeBackupSchedule(context.Context, string, string) (store.ServiceVolumeBackupConfig, error) {
	return f.cfg, f.getErr
}

func (f *fakeVolumeSchedules) SetServiceVolumeBackupSchedule(_ context.Context, _, _, targetID, _ string, _, _ int, _ string) error {
	f.setTo = targetID
	f.cfg.BackupTargetID = targetID
	return nil
}

func drillTestRouter(t *testing.T, opts ...Option) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, append([]Option{WithFixit(db)}, opts...)...)
	rt.backupProtectionStore = db
	return rt, db
}

func addFailedDrill(t *testing.T, db *store.DB, id, started, errText string) {
	t.Helper()
	d := store.BackupDrill{
		ID: id, BackupHistoryID: "h_" + id, ResourceKind: store.BackupResourceKindVolume, ServiceName: "app", VolumeName: "data",
		TargetID: "bkt_gone", StartedAt: started,
	}
	if err := db.StartBackupDrill(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	d.Status, d.Stage, d.Error, d.FinishedAt = store.DrillStatusFailed, "object", errText, started
	if err := db.FinishBackupDrill(context.Background(), d); err != nil {
		t.Fatal(err)
	}
}

func addLiveTarget(t *testing.T, db *store.DB, id string) {
	t.Helper()
	err := db.SaveBackupTarget(context.Background(), store.BackupTarget{
		ID: id, Name: "Main bucket", Provider: "aws", Endpoint: "https://s3.example.test", Region: "r", Bucket: "b", CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func drillIDs(t *testing.T, db *store.DB) map[string]bool {
	t.Helper()
	rows, err := db.ListBackupDrills(context.Background(), "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[r.ID] = true
	}
	return out
}

func TestDrillRepointRunsThroughEngineOnlyWithTheChosenTarget(t *testing.T) {
	t.Parallel()
	setup := func() (*Router, *store.DB, *fakeVolumeSchedules) {
		rt, db := drillTestRouter(t)
		vol := &fakeVolumeSchedules{cfg: store.ServiceVolumeBackupConfig{BackupTargetID: "bkt_gone", BackupSchedule: "0 3 * * *"}}
		rt.serviceVolumeBackupSchedule = vol
		addLiveTarget(t, db, "bkt_new")
		addFailedDrill(t, db, "d1", "2026-01-02T00:00:00Z", danglingErr)
		return rt, db, vol
	}
	ctx := context.Background()

	rt0, db0, vol0 := setup()
	if _, err := rt0.fixitEngine().Apply(ctx, drillRepointFixID, "tester", nil); err != nil {
		t.Fatal(err)
	}
	if vol0.setTo != "" || !drillIDs(t, db0)["d1"] {
		t.Fatalf("apply without a chosen target changed something: set=%q", vol0.setTo)
	}
	rt, db, vol := setup()
	res, err := rt.fixitEngine().Apply(ctx, drillRepointFixID, "tester", map[string]string{"target_id": "bkt_new"})
	if err != nil {
		t.Fatal(err)
	}
	if vol.setTo != "bkt_new" || drillIDs(t, db)["d1"] {
		t.Fatalf("schedule target = %q, drill kept = %v, outcome %s %s", vol.setTo, drillIDs(t, db)["d1"], res.Outcome, res.Detail)
	}
}

func TestDrillRepointKeepsEvidenceWhenTheScheduleCannotBeRead(t *testing.T) {
	t.Parallel()
	rt, db := drillTestRouter(t)
	rt.serviceVolumeBackupSchedule = &fakeVolumeSchedules{getErr: errors.New("database is locked")}
	addLiveTarget(t, db, "bkt_new")
	addFailedDrill(t, db, "d1", "2026-01-02T00:00:00Z", danglingErr)
	if _, err := rt.fixitEngine().Apply(context.Background(), drillRepointFixID, "tester", map[string]string{"target_id": "bkt_new"}); err != nil {
		t.Fatal(err)
	}
	if !drillIDs(t, db)["d1"] {
		t.Fatal("the drill row was dropped although the schedule could not be checked")
	}
}

func TestDrillStaleReferenceRemovesOnlyTheDanglingRows(t *testing.T) {
	t.Parallel()
	rt, db := drillTestRouter(t)
	addFailedDrill(t, db, "old_real", "2026-01-01T00:00:00Z", "AccessDenied 403")
	addFailedDrill(t, db, "d1", "2026-01-02T00:00:00Z", danglingErr)
	v, err := rt.fixitEngine().Preview(context.Background(), drillStaleRefFixID)
	if err != nil || !v.Failing || v.Reversible {
		t.Fatalf("preview = %+v, %v (history cannot be restored, so not reversible)", v, err)
	}
	if _, err := rt.fixitEngine().Apply(context.Background(), drillStaleRefFixID, "tester", nil); err != nil {
		t.Fatal(err)
	}
	got := drillIDs(t, db)
	if got["d1"] || !got["old_real"] {
		t.Fatalf("remaining drills = %v, want only old_real", got)
	}
}

func TestControlPlaneDREnableRunsThroughEngineOnlyWithConfirmation(t *testing.T) {
	t.Parallel()
	newFake := func() *fakeDR {
		return &fakeDR{status: cpbackup.Status{Configured: true, TargetID: "bkt_dr", Recipients: []string{"age1x"}, Schedule: "0 2 * * *"}}
	}
	ctx := context.Background()
	fake0 := newFake()
	rt0, _ := drillTestRouter(t, WithControlPlaneDR(fake0, nil))
	if _, err := rt0.fixitEngine().Apply(ctx, "control_plane.dr.enable", "tester", nil); err != nil {
		t.Fatal(err)
	}
	if fake0.updated.Enabled {
		t.Fatal("enabled without a confirmation")
	}
	fake := newFake()
	rt, _ := drillTestRouter(t, WithControlPlaneDR(fake, nil))
	if _, err := rt.fixitEngine().Apply(ctx, "control_plane.dr.enable", "tester", map[string]string{"confirm": "enable"}); err != nil {
		t.Fatal(err)
	}
	if !fake.updated.Enabled || fake.updated.TargetID != "bkt_dr" || len(fake.updated.Recipients) != 1 {
		t.Fatalf("updated = %+v", fake.updated)
	}
}

func TestDrillSeverityIgnoresMixedAndLiveTargets(t *testing.T) {
	if c := backup.ClassifyDrillFailure(danglingErr + ": AccessDenied 403"); c.Kind == backup.DrillFailureTargetMissing {
		t.Fatalf("an error with another failure class was downgraded: %+v", c)
	}
}

func TestDrillFixAuditNamesTheRealActorAndRootAbility(t *testing.T) {
	t.Parallel()
	rt, db := drillTestRouter(t)
	ctx := withFixitActor(context.Background(), store.PrincipalTypeUser, "u1", "Ada")
	rt.auditDrillFix(ctx, drillAuditRemoved, map[string]string{"drill_ids": "d1"})
	rows, err := db.ListAuditEntries(context.Background(), 10, nil, store.AuditEntryFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("audit rows = %d, %v", len(rows), err)
	}
	if r := rows[0]; r.ActorType != auditActorSession || r.ActorID != "u1" || r.Ability != AbilityRoot || r.Action != drillAuditRemoved {
		t.Fatalf("audit row = %+v", r)
	}
}

func TestDrillDisabledMessageNamesEachMissingPiece(t *testing.T) {
	tests := []struct {
		name string
		st   cpbackup.Status
		want string
	}{
		{"nothing", cpbackup.Status{}, "no off-box destination is defined and no encryption recipient is defined"},
		{"no recipient", cpbackup.Status{TargetID: "t"}, "no encryption recipient is defined"},
		{"no destination", cpbackup.Status{Recipients: []string{"r"}}, "no off-box destination is defined"},
		{"off", cpbackup.Status{Configured: true, TargetID: "t", Recipients: []string{"r"}}, "set up but switched off"},
	}
	for _, tc := range tests {
		if got := drDisabledMessage(tc.st); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q lacks %q", tc.name, got, tc.want)
		}
	}
}
