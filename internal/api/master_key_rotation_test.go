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

// fakeMasterKeyRotator is a hand-written fake for MasterKeyRotator, the
// same pattern fakeDockerPruner (system_prune_test.go) already
// establishes in this package.
type fakeMasterKeyRotator struct {
	rotateErr error
	rotatedAt time.Time
	gotNewKey string
	callCnt   int
	historyAt time.Time
	historyOK bool
}

func (f *fakeMasterKeyRotator) RotateMasterKey(_ context.Context, newMasterKey string) (time.Time, error) {
	f.callCnt++
	f.gotNewKey = newMasterKey
	if f.rotateErr != nil {
		return time.Time{}, f.rotateErr
	}
	return f.rotatedAt, nil
}

func (f *fakeMasterKeyRotator) GetMasterKeyRotatedAt(_ context.Context) (time.Time, bool, error) {
	return f.historyAt, f.historyOK, nil
}

func newTestRouterWithMasterKeyRotator(t *testing.T, r MasterKeyRotator, masterKeyFilePath string) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithMasterKeyRotation(r, masterKeyFilePath)), db
}

func TestHandleRotateMasterKey_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithMasterKeyRotation
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/master-key/rotate", `{"newMasterKey":"x"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestRotateMasterKeyRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithMasterKeyRotator(t, &fakeMasterKeyRotator{}, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/master-key/rotate", strings.NewReader(`{"newMasterKey":"x"}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleRotateMasterKey_PlainWriteToken_Forbidden proves this route
// sits behind AbilityRoot, not AbilityWrite: rotating the one key every
// other secret depends on is not something a narrower, per-app-scoped
// write token should ever reach.
func TestHandleRotateMasterKey_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithMasterKeyRotator(t, &fakeMasterKeyRotator{}, "")
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/master-key/rotate", strings.NewReader(`{"newMasterKey":"x"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach master key rotation", rec.Code, http.StatusForbidden)
	}
}

func TestHandleRotateMasterKey_MissingKey(t *testing.T) {
	fake := &fakeMasterKeyRotator{}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, "")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/master-key/rotate", `{"newMasterKey":"  "}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if fake.callCnt != 0 {
		t.Errorf("RotateMasterKey called %d times for a blank key, want 0", fake.callCnt)
	}
}

func TestHandleRotateMasterKey_RotatorError(t *testing.T) {
	fake := &fakeMasterKeyRotator{rotateErr: errors.New("unwrap DEK for \"web\": wrong key")}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, "")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/master-key/rotate", `{"newMasterKey":"new-key"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "wrong key") {
		t.Errorf("body = %s, want it to surface the rotator's own error", rec.Body.String())
	}
}

func TestHandleRotateMasterKey_EnvSourced_WarnsInsteadOfPersisting(t *testing.T) {
	rotatedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	fake := &fakeMasterKeyRotator{rotatedAt: rotatedAt}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, "") // "" means env-sourced
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/master-key/rotate", `{"newMasterKey":"new-key-value"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.gotNewKey != "new-key-value" {
		t.Errorf("RotateMasterKey called with %q, want %q", fake.gotNewKey, "new-key-value")
	}

	var got rotateMasterKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PersistedToFile {
		t.Error("PersistedToFile = true for an env-sourced master key, want false")
	}
	if got.Warning == "" || !strings.Contains(got.Warning, "APP_MASTER_KEY") {
		t.Errorf("Warning = %q, want it to mention APP_MASTER_KEY", got.Warning)
	}
	if !got.RotatedAt.Equal(rotatedAt) {
		t.Errorf("RotatedAt = %v, want %v", got.RotatedAt, rotatedAt)
	}
}

func TestHandleRotateMasterKey_FileSourced_PersistsNewKeyToFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "master.key")
	fake := &fakeMasterKeyRotator{rotatedAt: time.Now().UTC()}
	rt, db := newTestRouterWithMasterKeyRotator(t, fake, keyPath)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/master-key/rotate", `{"newMasterKey":"new-key-value"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got rotateMasterKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.PersistedToFile {
		t.Error("PersistedToFile = false, want true when a master key file path is configured")
	}
	if got.Warning != "" {
		t.Errorf("Warning = %q, want empty when the file write succeeds", got.Warning)
	}

	data, err := os.ReadFile(keyPath) //nolint:gosec // keyPath is this test's own t.TempDir() fixture path
	if err != nil {
		t.Fatalf("read persisted key file: %v", err)
	}
	if string(data) != "new-key-value" {
		t.Errorf("persisted key file contents = %q, want %q", data, "new-key-value")
	}
}
