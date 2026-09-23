package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleVerifyVolumeBackup_Success(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	target := seedBackupTargetForAPI(t, db)

	seedSucceededVolumeBackupForAPI(t, db, target.ID, "data")

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
