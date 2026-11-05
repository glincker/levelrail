package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/store"
)

type drillFixRecorder struct {
	failed   []store.BackupDrill
	targets  []store.BackupTarget
	repoint  map[string]string
	dropped  []string
	audits   []string
	repointN int
}

func (r *drillFixRecorder) deps() drillTargetDeps {
	return drillTargetDeps{
		Failed:  func(context.Context) ([]store.BackupDrill, error) { return r.failed, nil },
		Targets: func(context.Context) ([]store.BackupTarget, error) { return r.targets, nil },
		Repoint: func(_ context.Context, d store.BackupDrill, id string) (bool, error) {
			r.repointN++
			r.repoint[backupDrillKey(d)] = id
			return true, nil
		},
		Drop: func(_ context.Context, id string) (int, error) {
			r.dropped = append(r.dropped, id)
			r.failed = nil
			return 1, nil
		},
		Audit: func(_ context.Context, _, action string, _ map[string]string) { r.audits = append(r.audits, action) },
		CLI:   "levelrail",
	}
}

func backupDrillKey(d store.BackupDrill) string { return d.ResourceKind + "/" + d.DatabaseName }

func danglingDrill(name string) store.BackupDrill {
	return store.BackupDrill{
		ResourceKind: store.BackupResourceKindDatabase, DatabaseName: name, TargetID: "bkt_gone", Status: store.DrillStatusFailed,
		Stage: "object", Error: `get backup target "bkt_gone": store: backup target not found`,
	}
}

func TestDrillFixRepointsToTheOnlyValidTarget(t *testing.T) {
	r := &drillFixRecorder{
		failed:  []store.BackupDrill{danglingDrill("orders")},
		targets: []store.BackupTarget{{ID: "bkt_new", Name: "Main bucket"}},
		repoint: map[string]string{},
	}
	f := drillRepointFix(r.deps())
	if f.Kind != fixit.KindGuided {
		t.Fatalf("repoint kind = %s, want guided", f.Kind)
	}
	a, err := f.Detect(context.Background())
	if err != nil || !a.Failing || a.Param == nil || a.Param.Default != "bkt_new" {
		t.Fatalf("detect = %+v, %v", a, err)
	}
	if err := f.Run(context.Background(), fixit.Input{Actor: "tester"}); err != nil {
		t.Fatal(err)
	}
	if r.repoint["database/orders"] != "bkt_new" || len(r.dropped) != 1 {
		t.Fatalf("repoint = %v dropped = %v", r.repoint, r.dropped)
	}
	if len(r.audits) != 2 || r.audits[0] != drillAuditRepointed || r.audits[1] != drillAuditRemoved {
		t.Fatalf("audits = %v", r.audits)
	}
}

func TestDrillFixRepointNeedsAChoiceWithSeveralTargets(t *testing.T) {
	r := &drillFixRecorder{
		failed:  []store.BackupDrill{danglingDrill("orders")},
		targets: []store.BackupTarget{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}},
		repoint: map[string]string{},
	}
	f := drillRepointFix(r.deps())
	if err := f.Run(context.Background(), fixit.Input{}); err == nil {
		t.Fatal("expected an error without a chosen target")
	}
	if err := f.Run(context.Background(), fixit.Input{Params: map[string]string{"target_id": "zzz"}}); err == nil {
		t.Fatal("expected an error for an unknown target")
	}
	if r.repointN != 0 {
		t.Fatal("nothing may change without a valid choice")
	}
}

func TestDrillFixRemovesStaleReferenceWhenNoTargetExists(t *testing.T) {
	r := &drillFixRecorder{failed: []store.BackupDrill{danglingDrill("orders")}, repoint: map[string]string{}}
	f := drillStaleRefFix(r.deps())
	if f.Kind != fixit.KindAuto || !f.Reversible || f.Risk != fixit.RiskSafe {
		t.Fatalf("stale reference fix must be a safe reversible auto fix: %+v", f)
	}
	a, err := f.Detect(context.Background())
	if err != nil || !a.Failing || len(a.Changes) != 1 {
		t.Fatalf("detect = %+v, %v", a, err)
	}
	if err := f.Run(context.Background(), fixit.Input{Actor: "tester"}); err != nil {
		t.Fatal(err)
	}
	if len(r.dropped) != 1 || r.dropped[0] != "bkt_gone" || r.repointN != 0 {
		t.Fatalf("dropped = %v repoints = %d", r.dropped, r.repointN)
	}
	if len(r.audits) != 1 || r.audits[0] != drillAuditRemoved {
		t.Fatalf("audits = %v", r.audits)
	}
	if a, _ := f.Detect(context.Background()); a.Failing {
		t.Fatal("still failing after removal")
	}
}

func TestDrillFixLeavesRealFailuresAndLiveTargetsAlone(t *testing.T) {
	bad := danglingDrill("orders")
	bad.Error = "checksum of the stored object does not match"
	live := danglingDrill("users")
	live.TargetID = "bkt_new"
	r := &drillFixRecorder{
		failed:  []store.BackupDrill{bad, live},
		targets: []store.BackupTarget{{ID: "bkt_new", Name: "Main"}},
		repoint: map[string]string{},
	}
	for _, f := range []fixit.Fix{drillStaleRefFix(r.deps()), drillRepointFix(r.deps())} {
		if a, err := f.Detect(context.Background()); err != nil || a.Failing {
			t.Fatalf("%s must not offer a change: %+v %v", f.ID, a, err)
		}
		if err := f.Run(context.Background(), fixit.Input{}); !errors.Is(err, fixit.ErrCannotFixHere) {
			t.Fatalf("%s run = %v, want cannot fix here", f.ID, err)
		}
	}
	if len(r.dropped) != 0 || r.repointN != 0 {
		t.Fatal("a real restore failure must never be touched")
	}
}

func TestDrillFixDropRemovesOnlyFailedRowsOfTheMissingTarget(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	rt := NewRouter(discardLogger(), testBrand(), db, WithFixit(db))
	for _, d := range []store.BackupDrill{
		{ID: "d1", BackupHistoryID: "h1", ResourceKind: store.BackupResourceKindDatabase, DatabaseName: "orders", TargetID: "bkt_gone"},
		{ID: "d2", BackupHistoryID: "h2", ResourceKind: store.BackupResourceKindDatabase, DatabaseName: "users", TargetID: "bkt_live"},
	} {
		if err := db.StartBackupDrill(ctx, d); err != nil {
			t.Fatal(err)
		}
		d.Status, d.Stage, d.Error = store.DrillStatusFailed, "object", "boom"
		d.StartedAt, d.FinishedAt = "2026-01-01T00:00:00Z", "2026-01-01T00:00:01Z"
		if err := db.FinishBackupDrill(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	n, err := rt.dropDrillsForTarget(ctx, "bkt_gone")
	if err != nil || n != 1 {
		t.Fatalf("dropped %d, %v", n, err)
	}
	left, err := db.ListBackupDrills(ctx, "", "", "", 10)
	if err != nil || len(left) != 1 || left[0].ID != "d2" {
		t.Fatalf("left = %+v, %v", left, err)
	}
}
