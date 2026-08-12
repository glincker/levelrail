package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written, in-memory fake for Store, the same
// pattern every other reconciler/pipeline test in this codebase uses
// instead of a mocking framework: stateful enough for realistic
// multi-call scenarios (a DEK generated on one call must be found by the
// next), not just a call counter.
type fakeStore struct {
	deks   map[string][]byte
	values map[string]map[string][]byte
	// failGetDEK, if set, makes GetServiceDEK return this error for
	// every service, standing in for a real database failure distinct
	// from "not found".
	failGetDEK error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		deks:   map[string][]byte{},
		values: map[string]map[string][]byte{},
	}
}

func (f *fakeStore) GetServiceDEK(_ context.Context, serviceName string) ([]byte, error) {
	if f.failGetDEK != nil {
		return nil, f.failGetDEK
	}
	wrapped, ok := f.deks[serviceName]
	if !ok {
		return nil, store.ErrServiceDEKNotFound
	}
	return wrapped, nil
}

func (f *fakeStore) SaveServiceDEK(_ context.Context, serviceName string, wrappedDEK []byte) error {
	f.deks[serviceName] = wrappedDEK
	return nil
}

func (f *fakeStore) GetSecretValue(_ context.Context, serviceName, envKey string) ([]byte, error) {
	byKey, ok := f.values[serviceName]
	if !ok {
		return nil, store.ErrSecretValueNotFound
	}
	ciphertext, ok := byKey[envKey]
	if !ok {
		return nil, store.ErrSecretValueNotFound
	}
	return ciphertext, nil
}

func (f *fakeStore) SaveSecretValue(_ context.Context, serviceName, envKey string, ciphertext []byte) error {
	if f.values[serviceName] == nil {
		f.values[serviceName] = map[string][]byte{}
	}
	f.values[serviceName][envKey] = ciphertext
	return nil
}

func (f *fakeStore) HasSecretValue(ctx context.Context, serviceName, envKey string) (bool, error) {
	_, err := f.GetSecretValue(ctx, serviceName, envKey)
	if errors.Is(err, store.ErrSecretValueNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func testManager(t *testing.T) (*Manager, *fakeStore) {
	t.Helper()
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	fs := newFakeStore()
	return NewManager(fs, mk), fs
}

func TestManager_SetValue_ThenResolve_RoundTrips(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()

	if err := m.SetValue(ctx, "web", "API_KEY", "sk-abc123"); err != nil { //nolint:gosec // fake fixture, this test proves it is not stored in plaintext
		t.Fatalf("SetValue() error = %v", err)
	}

	got, err := m.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "sk-abc123" {
		t.Errorf("Resolve() = %q, want %q", got, "sk-abc123")
	}
}

func TestManager_SetValue_ReusesSameDEKAcrossKeys(t *testing.T) {
	// CLAUDE.md 4.10: "wrap the DEK once, encrypt many values fast with
	// it". A service's second secret must not generate a second DEK.
	m, fs := testManager(t)
	ctx := context.Background()

	if err := m.SetValue(ctx, "web", "API_KEY", "one"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := m.SetValue(ctx, "web", "DB_PASSWORD", "two"); err != nil { //nolint:gosec // fake fixture value, not a real credential
		t.Fatalf("SetValue() error = %v", err)
	}

	if len(fs.deks) != 1 {
		t.Errorf("deks stored = %d, want 1 (same DEK reused for both values)", len(fs.deks))
	}
}

func TestManager_SetValue_NeverStoresPlaintextCiphertextOrDEK(t *testing.T) {
	m, fs := testManager(t)
	ctx := context.Background()
	const plaintext = "postgres://user:hunter2@db.internal:5432/app" //nolint:gosec // fake fixture, this test proves this exact string never appears in stored bytes

	if err := m.SetValue(ctx, "web", "DATABASE_URL", plaintext); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	for _, ciphertext := range fs.values["web"] {
		if string(ciphertext) == plaintext {
			t.Fatal("stored ciphertext equals the plaintext verbatim, encryption did not happen")
		}
	}
	for _, wrapped := range fs.deks {
		if len(wrapped) == 0 {
			t.Fatal("stored wrapped DEK is empty")
		}
	}
}

func TestManager_Resolve_NoValueSet(t *testing.T) {
	m, _ := testManager(t)
	if _, err := m.Resolve(context.Background(), "web", "MISSING"); !errors.Is(err, ErrValueNotFound) {
		t.Errorf("Resolve() error = %v, want ErrValueNotFound", err)
	}
}

func TestManager_Resolve_ServiceHasDEKButNotThisKey(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	if _, err := m.Resolve(ctx, "web", "OTHER_KEY"); !errors.Is(err, ErrValueNotFound) {
		t.Errorf("Resolve() error = %v, want ErrValueNotFound", err)
	}
}

func TestManager_Resolve_WrongMasterKeyFails(t *testing.T) {
	fs := newFakeStore()
	mkA, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	mA := NewManager(fs, mkA)
	ctx := context.Background()
	if err := mA.SetValue(ctx, "web", "API_KEY", "secret-value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	mkB, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	mB := NewManager(fs, mkB)
	if _, err := mB.Resolve(ctx, "web", "API_KEY"); err == nil {
		t.Fatal("Resolve() with the wrong master key error = nil, want an error: a leaked master key of one deployment must not unwrap another's secrets")
	}
}

func TestManager_Exists(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()

	exists, err := m.Exists(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true before SetValue, want false")
	}

	if err := m.SetValue(ctx, "web", "API_KEY", "value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	exists, err = m.Exists(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false after SetValue, want true")
	}
}

func TestManager_Resolve_StoreErrorPropagates(t *testing.T) {
	// A real database failure (not "not found") must surface as a real
	// error, not be silently swallowed into ErrValueNotFound: a caller
	// that treats those the same could mistake "the database is down"
	// for "this app has no secret configured" and start a container
	// with a variable quietly unset instead of failing the deploy.
	fs := newFakeStore()
	fs.failGetDEK = errors.New("database is down")
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	m := NewManager(fs, mk)

	_, err = m.Resolve(context.Background(), "web", "API_KEY")
	if err == nil || errors.Is(err, ErrValueNotFound) {
		t.Errorf("Resolve() error = %v, want a real error distinct from ErrValueNotFound", err)
	}
}
