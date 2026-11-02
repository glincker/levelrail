package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func newTestBackupTarget() BackupTarget {
	return BackupTarget{
		ID:        "bkt_test1",
		Name:      "primary backups",
		Provider:  BackupProviderR2,
		Endpoint:  "https://example.r2.cloudflarestorage.com",
		Region:    "auto",
		Bucket:    "levelrail-backups",
		CreatedAt: "2026-08-14T00:00:00Z",
	}
}

func TestSaveAndGetBackupTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, want); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}

	got, err := db.GetBackupTarget(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetBackupTarget() error = %v", err)
	}
	if got != want {
		t.Errorf("GetBackupTarget() = %+v, want %+v", got, want)
	}
}

func TestGetBackupTarget_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetBackupTarget(ctx, "bkt_missing")
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("GetBackupTarget() error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestListBackupTargets_OrderedByCreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestBackupTarget()
	first.ID, first.CreatedAt = "bkt_a", "2026-08-14T00:00:00Z"
	second := newTestBackupTarget()
	second.ID, second.CreatedAt = "bkt_b", "2026-08-14T00:00:01Z"

	if err := db.SaveBackupTarget(ctx, second); err != nil {
		t.Fatalf("SaveBackupTarget(second) error = %v", err)
	}
	if err := db.SaveBackupTarget(ctx, first); err != nil {
		t.Fatalf("SaveBackupTarget(first) error = %v", err)
	}

	got, err := db.ListBackupTargets(ctx)
	if err != nil {
		t.Fatalf("ListBackupTargets() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "bkt_a" || got[1].ID != "bkt_b" {
		t.Fatalf("ListBackupTargets() = %+v, want [bkt_a, bkt_b] in creation order", got)
	}
}

func TestUpdateBackupTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	target := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}

	if err := db.UpdateBackupTarget(ctx, target.ID, "renamed", BackupProviderAWS, "", "us-east-1", "new-bucket"); err != nil {
		t.Fatalf("UpdateBackupTarget() error = %v", err)
	}

	got, err := db.GetBackupTarget(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetBackupTarget() error = %v", err)
	}
	want := BackupTarget{ID: target.ID, Name: "renamed", Provider: BackupProviderAWS, Endpoint: "", Region: "us-east-1", Bucket: "new-bucket", CreatedAt: target.CreatedAt}
	if got != want {
		t.Errorf("GetBackupTarget() after update = %+v, want %+v", got, want)
	}
}

func TestUpdateBackupTarget_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateBackupTarget(ctx, "bkt_missing", "x", BackupProviderAWS, "", "us-east-1", "bucket")
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("UpdateBackupTarget() error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestDeleteBackupTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, want); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	if err := db.DeleteBackupTarget(ctx, want.ID); err != nil {
		t.Fatalf("DeleteBackupTarget() error = %v", err)
	}

	_, err := db.GetBackupTarget(ctx, want.ID)
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("GetBackupTarget() after delete error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestDeleteBackupTarget_HistoryStatus(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{name: "failed only history is deletable", status: BackupStatusFailed},
		{name: "succeeded history blocks", status: BackupStatusSucceeded, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			target := newTestBackupTarget()
			if err := db.SaveBackupTarget(ctx, target); err != nil {
				t.Fatalf("SaveBackupTarget() error = %v", err)
			}
			if err := db.StartBackupHistory(ctx, BackupHistory{
				ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
				ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
			}); err != nil {
				t.Fatalf("StartBackupHistory() error = %v", err)
			}
			if err := db.FinishBackupHistory(ctx, "bkh_1", tc.status, 1, "", "boom", "2026-08-14T00:00:01Z"); err != nil {
				t.Fatalf("FinishBackupHistory() error = %v", err)
			}
			err := db.DeleteBackupTarget(ctx, target.ID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DeleteBackupTarget() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDeleteBackupTarget_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteBackupTarget(ctx, "bkt_missing")
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("DeleteBackupTarget() error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestDeleteBackupTarget_ReferencedByHistory_FailsCleanly(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	target := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	// Foreign key enforcement (store.go's "_pragma foreign_keys(ON)")
	// must reject this, not silently orphan or cascade-delete the
	// history row: see DeleteBackupTarget's own doc comment.
	if err := db.DeleteBackupTarget(ctx, target.ID); err == nil {
		t.Fatal("DeleteBackupTarget() error = nil, want a foreign key constraint error while backup_history still references this target")
	}
}

func TestRetireBackupTarget_KeepsHistoryOfDeletedResources(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	if err := db.StartBackupHistory(ctx, BackupHistory{ID: "bkh_o", DatabaseName: "gone", TargetID: target.ID, ObjectKey: "gone/1", StartedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishBackupHistory(ctx, "bkh_o", BackupStatusSucceeded, 1, "", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatal(err)
	}
	res, err := db.RetireBackupTarget(ctx, target.ID)
	if err != nil || res.Detached != 1 {
		t.Fatalf("RetireBackupTarget() = %+v, %v, want 1 detached, nil", res, err)
	}
	if _, err := db.GetBackupTarget(ctx, target.ID); !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("GetBackupTarget() error = %v, want not found", err)
	}
	if _, err := db.GetBackupHistory(ctx, "bkh_o"); err != nil {
		t.Fatalf("history row lost: %v", err)
	}
}

func TestRetireBackupTarget_BlockedByOtherUsers(t *testing.T) {
	cases := map[string]string{
		"bucket":      `INSERT INTO storage_buckets (id, name, target_id, prefix, created_at) VALUES ('b1', 'media', '%s', 'p/', 'now')`,
		"dr":          `UPDATE control_plane_dr_settings SET escrow_target_id = '%s' WHERE id = 1`,
		"copy":        `UPDATE backup_copy_settings SET target_id = '%s' WHERE id = 1`,
		"predepl":     `INSERT INTO app_predeploy_snapshot_settings (app, mode, target_id) VALUES ('web', 'on', '%s')`,
		"drtarget":    `INSERT OR IGNORE INTO control_plane_dr_settings (id) VALUES (1); UPDATE control_plane_dr_settings SET target_id = '%s' WHERE id = 1`,
		"basebackup":  `INSERT INTO base_backup_history (id, database_name, target_id, object_key, status, started_at) VALUES ('bb1', 'db', '%s', 'k', 'running', 'now')`,
		"drill":       `INSERT INTO backup_drills (id, backup_history_id, resource_kind, target_id, status, started_at) VALUES ('d1', 'h1', 'database', '%s', 'running', 'now')`,
		"copy-queued": `INSERT INTO backup_targets (id, name, provider, bucket, created_at) VALUES ('bkt_other', 'o', 'aws', 'b', 'now'); INSERT INTO backup_history (id, database_name, target_id, object_key, status, started_at, finished_at) VALUES ('h2', 'gone', 'bkt_other', 'k', 'succeeded', 'now', 'now'); INSERT INTO backup_copies (backup_history_id, target_id, status, queued_at) VALUES ('h2', '%s', 'pending', 'now')`,
	}
	for name, stmt := range cases {
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			target := seedBackupTarget(t, db)
			if name == "dr" {
				if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO control_plane_dr_settings (id) VALUES (1)`); err != nil {
					t.Fatal(err)
				}
			}
			for _, one := range strings.Split(strings.ReplaceAll(stmt, "%s", target.ID), "; ") {
				if _, err := db.ExecContext(ctx, one); err != nil {
					t.Fatalf("seed %q: %v", one, err)
				}
			}
			_, err := db.RetireBackupTarget(ctx, target.ID)
			var inUse *BackupTargetInUseError
			if !errors.As(err, &inUse) {
				t.Fatalf("RetireBackupTarget() error = %v, want in-use", err)
			}
			if _, err := db.GetBackupTarget(ctx, target.ID); err != nil {
				t.Fatalf("target must stay live: %v", err)
			}
		})
	}
}

func TestRetireBackupTarget_PruneScopeAndCredentials(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "live", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	for id, dbName := range map[string]string{"bkh_live": "live", "bkh_dead": "dead"} {
		if err := db.StartBackupHistory(ctx, BackupHistory{ID: id, DatabaseName: dbName, TargetID: target.ID, ObjectKey: dbName + "/1", StartedAt: "2026-08-14T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishBackupHistory(ctx, id, BackupStatusFailed, 0, "", "boom", "2026-08-14T00:01:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveServiceDEK(ctx, BackupTargetSecretsKey(target.ID), []byte("dek")); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSecretValue(ctx, BackupTargetSecretsKey(target.ID), "secret_access_key", []byte("sealed")); err != nil {
		t.Fatal(err)
	}

	// The live database's failed attempt stays, and it blocks the retire.
	if _, err := db.RetireBackupTarget(ctx, target.ID); err == nil {
		t.Fatal("want in-use error for history of a live database")
	}
	if _, err := db.GetBackupHistory(ctx, "bkh_live"); err != nil {
		t.Fatalf("live resource history was pruned: %v", err)
	}
	if ok, _ := db.HasSecretValue(ctx, BackupTargetSecretsKey(target.ID), "secret_access_key"); !ok {
		t.Fatal("credentials wiped by a blocked retire")
	}
	if _, err := db.GetBackupHistory(ctx, "bkh_dead"); err != nil {
		t.Fatalf("blocked retire must roll back the prune: %v", err)
	}

	if err := db.DeleteDesiredDatabase(ctx, "live"); err != nil {
		t.Fatal(err)
	}
	res, err := db.RetireBackupTarget(ctx, target.ID)
	if err != nil || res.Pruned != 2 {
		t.Fatalf("RetireBackupTarget() = %+v, %v, want 2 pruned", res, err)
	}
	if ok, _ := db.HasSecretValue(ctx, BackupTargetSecretsKey(target.ID), "secret_access_key"); ok {
		t.Fatal("credentials survived the retire")
	}
}

func TestRetireBackupTarget_WipeIsAtomic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	key := BackupTargetSecretsKey(target.ID)
	if err := db.SaveServiceDEK(ctx, key, []byte("dek")); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSecretValue(ctx, key, "secret_access_key", []byte("sealed")); err != nil {
		t.Fatal(err)
	}
	// A failure inside the transaction, after the target row is gone, must undo everything.
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_dek_delete BEFORE DELETE ON service_secrets BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RetireBackupTarget(ctx, target.ID); err == nil {
		t.Fatal("RetireBackupTarget() error = nil, want the injected failure")
	}
	if _, err := db.GetBackupTarget(ctx, target.ID); err != nil {
		t.Fatalf("target must survive a failed retire: %v", err)
	}
	if ok, _ := db.HasSecretValue(ctx, key, "secret_access_key"); !ok {
		t.Fatal("credentials deleted by a failed retire")
	}
}
