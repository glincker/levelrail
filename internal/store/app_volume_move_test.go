package store

import (
	"context"
	"errors"
	"testing"
)

func TestStartAndFinishAppVolumeMove_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.StartAppVolumeMove(ctx, AppVolumeMove{
		ID: "avm_1", ServiceName: "web", FromNodeID: "", ToNodeID: "node-2", StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartAppVolumeMove() error = %v", err)
	}

	got, err := db.GetAppVolumeMove(ctx, "avm_1")
	if err != nil {
		t.Fatalf("GetAppVolumeMove() error = %v", err)
	}
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}
	if got.ToNodeID != "node-2" || got.ServiceName != "web" {
		t.Fatalf("got = %+v, want ServiceName=web ToNodeID=node-2", got)
	}
	if len(got.Steps) != 0 {
		t.Fatalf("Steps after Start = %+v, want none yet", got.Steps)
	}

	steps := []AppVolumeMoveStep{
		{Name: "stop_app", Status: BackupStatusSucceeded, StartedAt: "2026-08-14T01:00:01Z", FinishedAt: "2026-08-14T01:00:02Z"},
		{Name: "move_volume:data", Status: BackupStatusRunning, StartedAt: "2026-08-14T01:00:03Z"},
	}
	if err := db.UpdateAppVolumeMoveSteps(ctx, "avm_1", steps); err != nil {
		t.Fatalf("UpdateAppVolumeMoveSteps() error = %v", err)
	}

	got, err = db.GetAppVolumeMove(ctx, "avm_1")
	if err != nil {
		t.Fatalf("GetAppVolumeMove() after step update error = %v", err)
	}
	if len(got.Steps) != 2 || got.Steps[1].Status != BackupStatusRunning {
		t.Fatalf("Steps after update = %+v, want the two steps just written", got.Steps)
	}

	if err := db.FinishAppVolumeMove(ctx, "avm_1", BackupStatusSucceeded, "", "2026-08-14T01:00:05Z"); err != nil {
		t.Fatalf("FinishAppVolumeMove() error = %v", err)
	}

	got, err = db.GetAppVolumeMove(ctx, "avm_1")
	if err != nil {
		t.Fatalf("GetAppVolumeMove() after finish error = %v", err)
	}
	if got.Status != BackupStatusSucceeded {
		t.Errorf("status = %q, want %q", got.Status, BackupStatusSucceeded)
	}
	if got.FinishedAt != "2026-08-14T01:00:05Z" {
		t.Errorf("FinishedAt = %q, want the value passed to Finish", got.FinishedAt)
	}
}

func TestFinishAppVolumeMove_Failed_RecordsErrorAndPartialSteps(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.StartAppVolumeMove(ctx, AppVolumeMove{
		ID: "avm_1", ServiceName: "web", FromNodeID: "node-1", ToNodeID: "node-2", StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartAppVolumeMove() error = %v", err)
	}

	failedStep := []AppVolumeMoveStep{
		{Name: "stop_app", Status: BackupStatusSucceeded, StartedAt: "2026-08-14T01:00:01Z", FinishedAt: "2026-08-14T01:00:02Z"},
		{Name: "move_volume:data", Status: BackupStatusFailed, Error: "docker daemon unreachable", StartedAt: "2026-08-14T01:00:03Z", FinishedAt: "2026-08-14T01:00:04Z"},
	}
	if err := db.UpdateAppVolumeMoveSteps(ctx, "avm_1", failedStep); err != nil {
		t.Fatalf("UpdateAppVolumeMoveSteps() error = %v", err)
	}
	if err := db.FinishAppVolumeMove(ctx, "avm_1", BackupStatusFailed, "docker daemon unreachable", "2026-08-14T01:00:04Z"); err != nil {
		t.Fatalf("FinishAppVolumeMove() error = %v", err)
	}

	got, err := db.GetAppVolumeMove(ctx, "avm_1")
	if err != nil {
		t.Fatalf("GetAppVolumeMove() error = %v", err)
	}
	if got.Status != BackupStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, BackupStatusFailed)
	}
	if got.Error != "docker daemon unreachable" {
		t.Errorf("Error = %q, want the failure reason", got.Error)
	}
	if len(got.Steps) != 2 || got.Steps[1].Status != BackupStatusFailed {
		t.Fatalf("Steps = %+v, want the partial step record preserved after failure", got.Steps)
	}
}

func TestFinishAppVolumeMove_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.FinishAppVolumeMove(ctx, "avm_missing", BackupStatusSucceeded, "", "2026-08-14T01:05:00Z")
	if !errors.Is(err, ErrAppVolumeMoveNotFound) {
		t.Fatalf("FinishAppVolumeMove() error = %v, want ErrAppVolumeMoveNotFound", err)
	}
}

func TestUpdateAppVolumeMoveSteps_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateAppVolumeMoveSteps(ctx, "avm_missing", []AppVolumeMoveStep{{Name: "stop_app", Status: BackupStatusRunning}})
	if !errors.Is(err, ErrAppVolumeMoveNotFound) {
		t.Fatalf("UpdateAppVolumeMoveSteps() error = %v, want ErrAppVolumeMoveNotFound", err)
	}
}

func TestGetAppVolumeMove_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetAppVolumeMove(ctx, "avm_missing")
	if !errors.Is(err, ErrAppVolumeMoveNotFound) {
		t.Fatalf("GetAppVolumeMove() error = %v, want ErrAppVolumeMoveNotFound", err)
	}
}

func TestListAppVolumeMoves_NewestFirst_ScopedToService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, m := range []AppVolumeMove{
		{ID: "avm_1", ServiceName: "web", FromNodeID: "", ToNodeID: "node-2", StartedAt: "2026-08-14T01:00:00Z"},
		{ID: "avm_2", ServiceName: "web", FromNodeID: "node-2", ToNodeID: "node-3", StartedAt: "2026-08-14T01:01:00Z"},
		{ID: "avm_3", ServiceName: "api", FromNodeID: "", ToNodeID: "node-2", StartedAt: "2026-08-14T01:02:00Z"},
	} {
		if err := db.StartAppVolumeMove(ctx, m); err != nil {
			t.Fatalf("StartAppVolumeMove(%s) error = %v", m.ID, err)
		}
	}

	got, err := db.ListAppVolumeMoves(ctx, "web")
	if err != nil {
		t.Fatalf("ListAppVolumeMoves() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "avm_2" || got[1].ID != "avm_1" {
		t.Fatalf("ListAppVolumeMoves(web) = %+v, want [avm_2, avm_1] newest first, other service's row excluded", got)
	}
}
