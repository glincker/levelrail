package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const rotateRoute = "/api/v1/system/master-key/rotate"

func TestHandleRotateMasterKey_RotatorFailure_LeavesKeyFileUntouchedAndNoStagedFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(keyPath, []byte("old-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &fakeMasterKeyRotator{rotateErr: errors.New("unwrap DEK for \"web\": wrong key")}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, keyPath)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, rotateRoute, `{"newMasterKey":"super-secret-new-key"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if data, _ := os.ReadFile(keyPath); string(data) != "old-key" { //nolint:gosec // test fixture path
		t.Errorf("key file = %q, want unchanged old-key", data)
	}
	if _, err := os.Stat(keyPath + ".new"); !os.IsNotExist(err) {
		t.Errorf("staged .new file left behind after a failed rotation: %v", err)
	}
}

func TestHandleRotateMasterKey_StageFailure_AbortsBeforeRotating(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "missing-dir", "master.key")
	fake := &fakeMasterKeyRotator{rotatedAt: time.Now().UTC()}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, keyPath)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, rotateRoute, `{"newMasterKey":"super-secret-new-key"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if fake.callCnt != 0 {
		t.Errorf("RotateMasterKey called %d times despite failing to stage the key file, want 0", fake.callCnt)
	}
	if strings.Contains(rec.Body.String(), "super-secret-new-key") || strings.Contains(rec.Body.String(), keyPath) {
		t.Errorf("body leaks key or path: %s", rec.Body.String())
	}
}

func TestHandleRotateMasterKey_KeyNeverInResponseOrAudit(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "master.key")
	fake := &fakeMasterKeyRotator{rotatedAt: time.Now().UTC()}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, keyPath)
	cookie := loginTestSession(t, rt, db)

	const secret = "super-secret-new-key"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, rotateRoute, `{"newMasterKey":"`+secret+`"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Error("response echoes the new master key")
	}
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	for _, e := range entries {
		if b, _ := json.Marshal(e); strings.Contains(string(b), secret) {
			t.Errorf("audit entry contains the new master key: %s", b)
		}
	}
	if _, err := os.Stat(keyPath + ".new"); !os.IsNotExist(err) {
		t.Errorf(".new still present after a successful rotation: %v", err)
	}
}
