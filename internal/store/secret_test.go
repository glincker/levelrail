package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSaveAndGetServiceDEK(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	wrapped := []byte("fake-wrapped-dek-bytes")
	if err := db.SaveServiceDEK(ctx, "web", wrapped); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}

	got, err := db.GetServiceDEK(ctx, "web")
	if err != nil {
		t.Fatalf("GetServiceDEK() error = %v", err)
	}
	if string(got) != string(wrapped) {
		t.Errorf("got %q, want %q", got, wrapped)
	}
}

func TestGetServiceDEK_NotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetServiceDEK(context.Background(), "ghost"); !errors.Is(err, ErrServiceDEKNotFound) {
		t.Errorf("error = %v, want ErrServiceDEKNotFound", err)
	}
}

func TestSaveAndGetSecretValue(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	ciphertext := []byte("fake-ciphertext-bytes")
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", ciphertext); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	got, err := db.GetSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(got) != string(ciphertext) {
		t.Errorf("got %q, want %q", got, ciphertext)
	}
}

func TestSaveSecretValue_UpsertReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}

	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("old")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("new")); err != nil {
		t.Fatalf("SaveSecretValue() (rotate) error = %v", err)
	}

	got, err := db.GetSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(got) != "new" {
		t.Errorf("got %q, want %q (rotation must replace, not accumulate)", got, "new")
	}
}

func TestGetSecretValue_NotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetSecretValue(context.Background(), "web", "MISSING"); !errors.Is(err, ErrSecretValueNotFound) {
		t.Errorf("error = %v, want ErrSecretValueNotFound", err)
	}
}

func TestHasSecretValue(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	exists, err := db.HasSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("HasSecretValue() error = %v", err)
	}
	if exists {
		t.Error("HasSecretValue() = true before any value saved, want false")
	}

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ciphertext")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	exists, err = db.HasSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("HasSecretValue() error = %v", err)
	}
	if !exists {
		t.Error("HasSecretValue() = false after saving a value, want true")
	}
}

func TestDeleteServiceSecrets_RemovesValuesAndDEK(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek-web")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ciphertext-1")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "OTHER_KEY", []byte("ciphertext-2")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	if err := db.DeleteServiceSecrets(ctx, "web"); err != nil {
		t.Fatalf("DeleteServiceSecrets() error = %v", err)
	}

	if _, err := db.GetServiceDEK(ctx, "web"); !errors.Is(err, ErrServiceDEKNotFound) {
		t.Errorf("GetServiceDEK() after delete error = %v, want ErrServiceDEKNotFound", err)
	}
	if _, err := db.GetSecretValue(ctx, "web", "API_KEY"); !errors.Is(err, ErrSecretValueNotFound) {
		t.Errorf("GetSecretValue(API_KEY) after delete error = %v, want ErrSecretValueNotFound", err)
	}
	if _, err := db.GetSecretValue(ctx, "web", "OTHER_KEY"); !errors.Is(err, ErrSecretValueNotFound) {
		t.Errorf("GetSecretValue(OTHER_KEY) after delete error = %v, want ErrSecretValueNotFound", err)
	}
}

func TestDeleteServiceSecrets_UnknownServiceIsNotAnError(t *testing.T) {
	db := openTestDB(t)
	if err := db.DeleteServiceSecrets(context.Background(), "ghost"); err != nil {
		t.Errorf("DeleteServiceSecrets() on an unknown service error = %v, want nil (idempotent)", err)
	}
}

func TestDeleteServiceSecrets_DoesNotAffectOtherServices(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek-web")); err != nil {
		t.Fatalf("SaveServiceDEK(web) error = %v", err)
	}
	if err := db.SaveServiceDEK(ctx, "worker", []byte("dek-worker")); err != nil {
		t.Fatalf("SaveServiceDEK(worker) error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "worker", "API_KEY", []byte("ciphertext")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	if err := db.DeleteServiceSecrets(ctx, "web"); err != nil {
		t.Fatalf("DeleteServiceSecrets() error = %v", err)
	}

	if _, err := db.GetServiceDEK(ctx, "worker"); err != nil {
		t.Errorf("GetServiceDEK(worker) after deleting web's secrets error = %v, want nil", err)
	}
	if _, err := db.GetSecretValue(ctx, "worker", "API_KEY"); err != nil {
		t.Errorf("GetSecretValue(worker) after deleting web's secrets error = %v, want nil", err)
	}
}

func TestSaveServiceDEK_TwoServicesGetIndependentDEKs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek-web")); err != nil {
		t.Fatalf("SaveServiceDEK(web) error = %v", err)
	}
	if err := db.SaveServiceDEK(ctx, "worker", []byte("dek-worker")); err != nil {
		t.Fatalf("SaveServiceDEK(worker) error = %v", err)
	}

	webDEK, err := db.GetServiceDEK(ctx, "web")
	if err != nil {
		t.Fatalf("GetServiceDEK(web) error = %v", err)
	}
	workerDEK, err := db.GetServiceDEK(ctx, "worker")
	if err != nil {
		t.Fatalf("GetServiceDEK(worker) error = %v", err)
	}
	if string(webDEK) == string(workerDEK) {
		t.Error("two different services got the same DEK bytes, want independent per-app keys")
	}
}

func TestListSecretKeys(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ct1")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "DB_PASSWORD", []byte("ct2")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	keys, err := db.ListSecretKeys(ctx, "web")
	if err != nil {
		t.Fatalf("ListSecretKeys() error = %v", err)
	}
	wantKeys := []string{"API_KEY", "DB_PASSWORD"}
	if len(keys) != len(wantKeys) {
		t.Fatalf("got %d keys, want %d: %+v", len(keys), len(wantKeys), keys)
	}
	for i, k := range keys {
		if k.Key != wantKeys[i] {
			t.Errorf("keys[%d].Key = %q, want %q", i, k.Key, wantKeys[i])
		}
		if k.Locked {
			t.Errorf("keys[%d].Locked = true, want false", i)
		}
		if k.UpdatedAt.IsZero() {
			t.Errorf("keys[%d].UpdatedAt is zero, want a real timestamp from SaveSecretValue", i)
		}
		if time.Since(k.UpdatedAt) > time.Minute {
			t.Errorf("keys[%d].UpdatedAt = %v, want close to now (SaveSecretValue just ran)", i, k.UpdatedAt)
		}
	}
}

// TestListSecretKeys_UpdatedAtMovesOnRotation confirms SecretKeyInfo.
// UpdatedAt reflects a rotation, not just the original SaveSecretValue:
// the same "rotation moves the age forward" behavior the frontend's
// staleness badge depends on.
func TestListSecretKeys_UpdatedAtMovesOnRotation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("v1")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	keys, err := db.ListSecretKeys(ctx, "web")
	if err != nil {
		t.Fatalf("ListSecretKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}
	firstUpdatedAt := keys[0].UpdatedAt

	// Force a measurable gap: SQLite's strftime('%f') has millisecond
	// resolution, and two upserts in the same test can otherwise land in
	// the same millisecond.
	time.Sleep(5 * time.Millisecond)

	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("v2")); err != nil {
		t.Fatalf("SaveSecretValue() (rotate) error = %v", err)
	}
	keys, err = db.ListSecretKeys(ctx, "web")
	if err != nil {
		t.Fatalf("ListSecretKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}
	if !keys[0].UpdatedAt.After(firstUpdatedAt) {
		t.Errorf("UpdatedAt after rotation = %v, want strictly after the original %v", keys[0].UpdatedAt, firstUpdatedAt)
	}
}

func TestListSecretKeys_Empty(t *testing.T) {
	db := openTestDB(t)
	keys, err := db.ListSecretKeys(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("ListSecretKeys() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("got %d keys, want 0", len(keys))
	}
}

func TestSetSecretLocked(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ct1")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	if err := db.SetSecretLocked(ctx, "web", "API_KEY", true); err != nil {
		t.Fatalf("SetSecretLocked(true) error = %v", err)
	}
	keys, err := db.ListSecretKeys(ctx, "web")
	if err != nil {
		t.Fatalf("ListSecretKeys() error = %v", err)
	}
	if len(keys) != 1 || !keys[0].Locked {
		t.Fatalf("got %+v, want one locked key", keys)
	}

	// Reversible: unlocking must work too, not just locking.
	if err := db.SetSecretLocked(ctx, "web", "API_KEY", false); err != nil {
		t.Fatalf("SetSecretLocked(false) error = %v", err)
	}
	keys, err = db.ListSecretKeys(ctx, "web")
	if err != nil {
		t.Fatalf("ListSecretKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0].Locked {
		t.Fatalf("got %+v, want unlocked key", keys)
	}
}

func TestSetSecretLocked_KeyNotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.SetSecretLocked(context.Background(), "web", "GHOST_KEY", true)
	if !errors.Is(err, ErrSecretValueNotFound) {
		t.Errorf("error = %v, want ErrSecretValueNotFound", err)
	}
}

func TestGetSecretKeyLocked(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ct1")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	exists, locked, err := db.GetSecretKeyLocked(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretKeyLocked() error = %v", err)
	}
	if !exists || locked {
		t.Errorf("exists=%v locked=%v, want exists=true locked=false", exists, locked)
	}

	if err := db.SetSecretLocked(ctx, "web", "API_KEY", true); err != nil {
		t.Fatalf("SetSecretLocked() error = %v", err)
	}
	exists, locked, err = db.GetSecretKeyLocked(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretKeyLocked() error = %v", err)
	}
	if !exists || !locked {
		t.Errorf("exists=%v locked=%v, want exists=true locked=true", exists, locked)
	}
}

func TestGetSecretKeyLocked_NotFound(t *testing.T) {
	db := openTestDB(t)
	exists, locked, err := db.GetSecretKeyLocked(context.Background(), "web", "GHOST")
	if err != nil {
		t.Fatalf("GetSecretKeyLocked() error = %v", err)
	}
	if exists || locked {
		t.Errorf("exists=%v locked=%v, want both false", exists, locked)
	}
}

func TestRotateServiceDEKs_RewrapsEveryRowAndRecordsTimestamp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("wrapped-web-v1")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveServiceDEK(ctx, "worker", []byte("wrapped-worker-v1")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}

	if _, ok, err := db.GetMasterKeyRotatedAt(ctx); err != nil || ok {
		t.Fatalf("GetMasterKeyRotatedAt() before any rotation = (ok=%v, err=%v), want ok=false, err=nil", ok, err)
	}

	seen := map[string]bool{}
	rewrap := func(serviceName string, wrapped []byte) ([]byte, error) {
		seen[serviceName] = true
		return append(wrapped, "-v2"...), nil
	}
	if err := db.RotateServiceDEKs(ctx, rewrap); err != nil {
		t.Fatalf("RotateServiceDEKs() error = %v", err)
	}
	if !seen["web"] || !seen["worker"] {
		t.Errorf("RotateServiceDEKs() rewrap called for %v, want both web and worker", seen)
	}

	got, err := db.GetServiceDEK(ctx, "web")
	if err != nil {
		t.Fatalf("GetServiceDEK() error = %v", err)
	}
	if string(got) != "wrapped-web-v1-v2" {
		t.Errorf("GetServiceDEK() after rotation = %q, want %q", got, "wrapped-web-v1-v2")
	}

	rotatedAt, ok, err := db.GetMasterKeyRotatedAt(ctx)
	if err != nil {
		t.Fatalf("GetMasterKeyRotatedAt() error = %v", err)
	}
	if !ok {
		t.Fatal("GetMasterKeyRotatedAt() ok = false after a successful rotation, want true")
	}
	if rotatedAt.IsZero() {
		t.Error("GetMasterKeyRotatedAt() returned a zero time after a successful rotation")
	}
}

func TestRotateServiceDEKs_RollsBackEverythingOnAnyRowFailure(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "a-first", []byte("wrapped-a-v1")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveServiceDEK(ctx, "b-second", []byte("wrapped-b-v1")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}

	wantErr := errors.New("simulated unwrap failure")
	rewrap := func(serviceName string, wrapped []byte) ([]byte, error) {
		if serviceName == "b-second" {
			return nil, wantErr
		}
		return append(wrapped, "-v2"...), nil
	}
	if err := db.RotateServiceDEKs(ctx, rewrap); !errors.Is(err, wantErr) {
		t.Fatalf("RotateServiceDEKs() error = %v, want to wrap %v", err, wantErr)
	}

	gotA, err := db.GetServiceDEK(ctx, "a-first")
	if err != nil {
		t.Fatalf("GetServiceDEK(a-first) error = %v", err)
	}
	if string(gotA) != "wrapped-a-v1" {
		t.Errorf("GetServiceDEK(a-first) after a failed rotation = %q, want unchanged %q", gotA, "wrapped-a-v1")
	}

	if _, ok, err := db.GetMasterKeyRotatedAt(ctx); err != nil || ok {
		t.Errorf("GetMasterKeyRotatedAt() after a failed rotation = (ok=%v, err=%v), want ok=false, err=nil", ok, err)
	}
}

func TestSecretCiphertexts_CountListReplace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	prefix := []byte{0xB7, 0x1C}
	for _, svc := range []string{"a", "b"} {
		if err := db.SaveServiceDEK(ctx, svc, []byte("dek")); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []SecretCiphertext{
		{"a", "K1", append([]byte{0xB7, 0x1C}, 'x')},
		{"a", "K2", []byte("legacy")},
		{"b", "K1", []byte{0xB7}},
	} {
		if err := db.SaveSecretValue(ctx, row.ServiceName, row.EnvKey, row.Ciphertext); err != nil {
			t.Fatal(err)
		}
	}

	total, withPrefix, err := db.CountSecretValuesByPrefix(ctx, prefix)
	if err != nil || total != 3 || withPrefix != 1 {
		t.Fatalf("CountSecretValuesByPrefix() = (%d, %d, %v), want (3, 1, nil)", total, withPrefix, err)
	}

	page, err := db.ListSecretCiphertexts(ctx, "", "", 2)
	if err != nil || len(page) != 2 || page[0].EnvKey != "K1" || page[1].EnvKey != "K2" {
		t.Fatalf("ListSecretCiphertexts(first page) = (%+v, %v)", page, err)
	}
	rest, err := db.ListSecretCiphertexts(ctx, page[1].ServiceName, page[1].EnvKey, 2)
	if err != nil || len(rest) != 1 || rest[0].ServiceName != "b" {
		t.Fatalf("ListSecretCiphertexts(after a/K2) = (%+v, %v)", rest, err)
	}

	keysBefore, err := db.ListSecretKeys(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := db.ReplaceSecretCiphertext(ctx, "a", "K2", []byte("stale"), []byte("new")); err != nil || ok {
		t.Fatalf("ReplaceSecretCiphertext(stale old) = (%t, %v), want (false, nil)", ok, err)
	}
	if ok, err := db.ReplaceSecretCiphertext(ctx, "a", "K2", []byte("legacy"), []byte("new")); err != nil || !ok {
		t.Fatalf("ReplaceSecretCiphertext(current old) = (%t, %v), want (true, nil)", ok, err)
	}
	got, err := db.GetSecretValue(ctx, "a", "K2")
	if err != nil || string(got) != "new" {
		t.Fatalf("GetSecretValue after replace = (%q, %v)", got, err)
	}
	keysAfter, err := db.ListSecretKeys(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if !keysAfter[1].UpdatedAt.Equal(keysBefore[1].UpdatedAt) {
		t.Error("ReplaceSecretCiphertext bumped updated_at, re-encryption must not count as a rotation")
	}
}

func TestGetMasterKeyRotatedAt_NeverRotated(t *testing.T) {
	db := openTestDB(t)
	if _, ok, err := db.GetMasterKeyRotatedAt(context.Background()); err != nil || ok {
		t.Errorf("GetMasterKeyRotatedAt() on a fresh store = (ok=%v, err=%v), want ok=false, err=nil", ok, err)
	}
}
