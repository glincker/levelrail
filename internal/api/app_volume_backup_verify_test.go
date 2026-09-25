package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleVerifyVolumeBackup_Success(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), "bkh_1", store.BackupStatusSucceeded, 9, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("finish backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var got backupVerificationResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.BackupHistoryID != "bkh_1" {
		t.Errorf("BackupHistoryID = %q, want %q", got.BackupHistoryID, "bkh_1")
	}
}
