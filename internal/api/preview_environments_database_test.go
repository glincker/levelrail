package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// setUpPreviewAppWithDatabase is setUpPreviewApp plus a managed database
// with a backup target configured, attached to the production "web"
// service the same way the dashboard's DatabaseAttachmentCard would.
func setUpPreviewAppWithDatabase(t *testing.T) (rt *Router, db *store.DB, secret string, builder *sequencedBuilder, target store.BackupTarget) {
	t.Helper()
	rt, db, secret, builder = setUpPreviewApp(t)

	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt.backupRunner = backupRunner
	rt.cloneRestoreRunner = cloneRunner

	target = seedDatabaseWithBackupTarget(t, db)
	if err := db.UpdateServiceDatabaseAttachment(context.Background(), "web", &store.DatabaseAttachment{
		DatabaseName: "mydb", EnvVar: "DATABASE_URL", Field: "url",
	}); err != nil {
		t.Fatalf("UpdateServiceDatabaseAttachment() error = %v", err)
	}

	return rt, db, secret, builder, target
}

func TestDeployPreviewSingle_ClonesAttachedDatabase(t *testing.T) {
	rt, db, secret, builder, target := setUpPreviewAppWithDatabase(t)
	backupRunner := rt.backupRunner.(*fakeBackupRunner)
	cloneRunner := rt.cloneRestoreRunner.(*fakeCloneRestoreRunner)

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(builder.calls) != 1 {
		t.Fatalf("Deploy called %d times, want 1", len(builder.calls))
	}

	wantClone := "mydb-web-pr-42"

	backupCall := backupRunner.awaitCall(t)
	if backupCall.databaseName != "mydb" || backupCall.targetID != target.ID {
		t.Errorf("backup call = %+v, want databaseName=mydb targetID=%s", backupCall, target.ID)
	}
	cloneCall := cloneRunner.awaitCall(t)
	if cloneCall.sourceDatabaseName != "mydb" || cloneCall.newDatabaseName != wantClone {
		t.Errorf("clone call = %+v, want source=mydb new=%s", cloneCall, wantClone)
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if preview.ClonedDatabaseName != wantClone {
		t.Errorf("ClonedDatabaseName = %q, want %q", preview.ClonedDatabaseName, wantClone)
	}

	previewSvc, err := db.GetDesiredService(context.Background(), "web-pr-42")
	if err != nil {
		t.Fatalf("GetDesiredService(web-pr-42) error = %v", err)
	}
	if previewSvc.DatabaseAttachment == nil {
		t.Fatal("preview service DatabaseAttachment = nil, want it pointed at the clone")
	}
	if previewSvc.DatabaseAttachment.DatabaseName != wantClone {
		t.Errorf("preview DatabaseAttachment.DatabaseName = %q, want %q (never the production database itself)", previewSvc.DatabaseAttachment.DatabaseName, wantClone)
	}
	if previewSvc.DatabaseAttachment.EnvVar != "DATABASE_URL" || previewSvc.DatabaseAttachment.Field != "url" {
		t.Errorf("preview DatabaseAttachment = %+v, want EnvVar/Field carried over from production's own attachment", previewSvc.DatabaseAttachment)
	}

	newDB, err := db.GetDesiredDatabase(context.Background(), wantClone)
	if err != nil {
		t.Fatalf("the cloned database was not created: %v", err)
	}
	if newDB.Engine != "postgres" {
		t.Errorf("cloned database engine = %q, want postgres (copied from the source)", newDB.Engine)
	}
}

func TestDeployPreviewSingle_NoBackupTarget_DeploysWithoutDatabase(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt.backupRunner = backupRunner
	rt.cloneRestoreRunner = cloneRunner

	// A database exists and is attached, but has no backup target
	// configured: cloning it is impossible, so the preview must deploy
	// with no database attachment at all rather than falling back to
	// pointing at production's own live data.
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "mydb", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.UpdateServiceDatabaseAttachment(context.Background(), "web", &store.DatabaseAttachment{
		DatabaseName: "mydb", EnvVar: "DATABASE_URL", Field: "url",
	}); err != nil {
		t.Fatalf("UpdateServiceDatabaseAttachment() error = %v", err)
	}

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(builder.calls) != 1 {
		t.Fatalf("Deploy called %d times, want 1", len(builder.calls))
	}

	select {
	case call := <-backupRunner.calls:
		t.Fatalf("RunBackup was called (%+v), want no clone attempt when no backup target is configured", call)
	default:
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if preview.ClonedDatabaseName != "" {
		t.Errorf("ClonedDatabaseName = %q, want empty", preview.ClonedDatabaseName)
	}
	if preview.Status != store.PreviewStatusActive {
		t.Errorf("Status = %q, want %q (a missing backup target must not fail the whole preview)", preview.Status, store.PreviewStatusActive)
	}

	previewSvc, err := db.GetDesiredService(context.Background(), "web-pr-42")
	if err != nil {
		t.Fatalf("GetDesiredService(web-pr-42) error = %v", err)
	}
	if previewSvc.DatabaseAttachment != nil {
		t.Errorf("preview service DatabaseAttachment = %+v, want nil", previewSvc.DatabaseAttachment)
	}
}

func TestTeardownPullRequestPreview_DeletesClonedDatabase(t *testing.T) {
	rt, db, secret, _, _ := setUpPreviewAppWithDatabase(t)

	opened := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rt.backupRunner.(*fakeBackupRunner).awaitCall(t)
	rt.cloneRestoreRunner.(*fakeCloneRestoreRunner).awaitCall(t)

	wantClone := "mydb-web-pr-42"
	if _, err := db.GetDesiredDatabase(context.Background(), wantClone); err != nil {
		t.Fatalf("precondition: cloned database %q should exist: %v", wantClone, err)
	}

	closed := githubPullRequestBody("closed", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, closed); rec.Code != http.StatusOK {
		t.Fatalf("closed: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := db.GetDesiredDatabase(context.Background(), wantClone); err == nil {
		t.Errorf("cloned database %q still exists after teardown, want it deleted", wantClone)
	}
}
