package store

import (
	"context"
	"testing"
	"time"
)

func TestCPDRSettings_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)

	got, err := db.GetCPDRSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.RetainDaily != 0 || len(got.Recipients) != 0 {
		t.Fatalf("seeded defaults wrong: %+v", got)
	}

	want := CPDRSettings{Enabled: true, TargetID: "t1", Recipients: []string{"age1a", "age1b"}, Schedule: "0 3 * * *",
		RetainDaily: 3, RetainWeekly: 2, RetainMonthly: 1, DrillSchedule: "0 4 * * 0"}
	if err := db.UpdateCPDRConfig(ctx, want, now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCPDRInstallID(ctx, "inst-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCPDRInstallID(ctx, "inst-2"); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordCPDRBackup(ctx, now, "cp-backups/x", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordCPDRBackup(ctx, now.Add(time.Hour), "", "boom"); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordCPDRDrill(ctx, now, true, true, "partial", 42); err != nil {
		t.Fatal(err)
	}
	if err := db.AckCPDREscrow(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordCPDREscrow(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := db.AckCPDREscrow(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	got, err = db.GetCPDRSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.InstallID != "inst-1" || !got.Enabled || got.TargetID != "t1" || len(got.Recipients) != 2 || got.RetainWeekly != 2 {
		t.Errorf("config not persisted: %+v", got)
	}
	if !got.LastBackupAt.Equal(now) || got.LastBackupKey != "cp-backups/x" || got.LastBackupError != "boom" {
		t.Errorf("backup outcome wrong: %+v", got)
	}
	if !got.LastDrillOK || !got.LastDrillPartial || got.LastDrillMs != 42 {
		t.Errorf("drill outcome wrong: %+v", got)
	}
	if !got.EscrowAckedAt.Equal(now.Add(time.Minute)) {
		t.Errorf("escrow ack = %v", got.EscrowAckedAt)
	}
}
