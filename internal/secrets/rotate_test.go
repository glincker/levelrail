package secrets

import (
	"context"
	"testing"
)

func TestRotateStoredDEKs_RoundTrips(t *testing.T) {
	ctx := context.Background()
	oldKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	newKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	fs := newFakeStore()
	oldManager := NewManager(fs, oldKey)
	if err := oldManager.SetValue(ctx, "web", "API_KEY", "sk-abc123"); err != nil { //nolint:gosec // fake fixture value
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := oldManager.SetValue(ctx, "worker", "DB_PASSWORD", "hunter2"); err != nil { //nolint:gosec // fake fixture value
		t.Fatalf("SetValue() error = %v", err)
	}

	if err := RotateStoredDEKs(ctx, fs, oldKey, newKey); err != nil {
		t.Fatalf("RotateStoredDEKs() error = %v", err)
	}

	for name, wrapped := range fs.deks {
		if _, err := oldKey.UnwrapDEK(WrappedDEK(wrapped)); err == nil {
			t.Errorf("service %q's DEK still unwraps under the old key after rotation", name)
		}
	}

	newManager := NewManager(fs, newKey)
	got, err := newManager.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() after rotation error = %v", err)
	}
	if got != "sk-abc123" {
		t.Errorf("Resolve() after rotation = %q, want %q", got, "sk-abc123")
	}
	got, err = newManager.Resolve(ctx, "worker", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("Resolve() after rotation error = %v", err)
	}
	if got != "hunter2" {
		t.Errorf("Resolve() after rotation = %q, want %q", got, "hunter2")
	}
}

func TestRotateStoredDEKs_WrongOldKeyRejectedBeforeAnyRowTouched(t *testing.T) {
	ctx := context.Background()
	realOldKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	wrongOldKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	newKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	fs := newFakeStore()
	m := NewManager(fs, realOldKey)
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-abc123"); err != nil { //nolint:gosec // fake fixture value
		t.Fatalf("SetValue() error = %v", err)
	}
	wrappedBefore := append([]byte(nil), fs.deks["web"]...)

	if err := RotateStoredDEKs(ctx, fs, wrongOldKey, newKey); err == nil {
		t.Fatal("RotateStoredDEKs() with the wrong old key succeeded, want an error")
	}

	if string(fs.deks["web"]) != string(wrappedBefore) {
		t.Error("RotateStoredDEKs() modified the stored DEK despite failing, want the row untouched")
	}
	if _, err := m.Resolve(ctx, "web", "API_KEY"); err != nil {
		t.Errorf("Resolve() with the original key after a failed rotation = %v, want success", err)
	}
}

func TestRotateStoredDEKs_AbortsWholeRotationOnAnyRowFailure(t *testing.T) {
	ctx := context.Background()
	oldKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	newKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	fs := newFakeStore()
	m := NewManager(fs, oldKey)
	// "a-first" sorts before "b-corrupt": proves a row that would have
	// rewrapped successfully, and comes first, still gets rolled back
	// when a later row fails.
	if err := m.SetValue(ctx, "a-first", "KEY", "value-one"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := m.SetValue(ctx, "b-corrupt", "KEY", "value-two"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	wrappedBefore := append([]byte(nil), fs.deks["a-first"]...)
	fs.deks["b-corrupt"] = []byte("not a valid wrapped dek")

	if err := RotateStoredDEKs(ctx, fs, oldKey, newKey); err == nil {
		t.Fatal("RotateStoredDEKs() with one corrupt row succeeded, want an error")
	}

	if string(fs.deks["a-first"]) != string(wrappedBefore) {
		t.Error("RotateStoredDEKs() left the first, otherwise-valid row rewrapped despite the overall rotation failing")
	}
	if _, err := m.Resolve(ctx, "a-first", "KEY"); err != nil {
		t.Errorf("Resolve() for the untouched row after a failed rotation = %v, want success", err)
	}
}

func TestManager_RotateMasterKey_RoundTrips(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-abc123"); err != nil { //nolint:gosec // fake fixture value
		t.Fatalf("SetValue() error = %v", err)
	}
	oldKey := m.mk

	newKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	if _, err := m.RotateMasterKey(ctx, newKey.String()); err != nil {
		t.Fatalf("RotateMasterKey() error = %v", err)
	}

	if _, err := oldKey.UnwrapDEK(WrappedDEK(fs.deks["web"])); err == nil {
		t.Error("DEK still unwraps under the old key after RotateMasterKey")
	}
	got, err := m.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() after RotateMasterKey error = %v", err)
	}
	if got != "sk-abc123" {
		t.Errorf("Resolve() after RotateMasterKey = %q, want %q", got, "sk-abc123")
	}

	if err := m.SetValue(ctx, "web", "OTHER_KEY", "another-value"); err != nil {
		t.Fatalf("SetValue() after RotateMasterKey error = %v", err)
	}
}

func TestManager_RotateMasterKey_InvalidNewKeyRejectedBeforeAnyChange(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-abc123"); err != nil { //nolint:gosec // fake fixture value
		t.Fatalf("SetValue() error = %v", err)
	}
	wrappedBefore := append([]byte(nil), fs.deks["web"]...)

	if _, err := m.RotateMasterKey(ctx, "not a valid age identity"); err == nil {
		t.Fatal("RotateMasterKey() with a malformed key string succeeded, want an error")
	}

	if string(fs.deks["web"]) != string(wrappedBefore) {
		t.Error("RotateMasterKey() modified stored data despite failing to parse the new key")
	}
	if _, err := m.Resolve(ctx, "web", "API_KEY"); err != nil {
		t.Errorf("Resolve() after a rejected rotation = %v, want success", err)
	}
}

func TestManager_RotateMasterKey_StoreFailureLeavesManagerOnOldKey(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-abc123"); err != nil { //nolint:gosec // fake fixture value
		t.Fatalf("SetValue() error = %v", err)
	}
	fs.deks["web"] = []byte("not a valid wrapped dek")

	newKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	if _, err := m.RotateMasterKey(ctx, newKey.String()); err == nil {
		t.Fatal("RotateMasterKey() with a corrupt stored DEK succeeded, want an error")
	}

	// Manager must still be usable under its original key: a failed
	// rotation must never leave it half-switched.
	if err := m.SetValue(ctx, "other", "KEY", "value"); err != nil {
		t.Errorf("SetValue() on an unrelated service after a failed rotation = %v, want success", err)
	}
}
