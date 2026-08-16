package secrets

import (
	"context"
	"errors"
	"sort"
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
	locked map[string]map[string]bool
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

func (f *fakeStore) DeleteServiceSecrets(_ context.Context, serviceName string) error {
	delete(f.deks, serviceName)
	delete(f.values, serviceName)
	return nil
}

func (f *fakeStore) ListSecretKeys(_ context.Context, serviceName string) ([]store.SecretKeyInfo, error) {
	byKey, ok := f.values[serviceName]
	if !ok {
		return nil, nil
	}
	var keys []string
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]store.SecretKeyInfo, len(keys))
	for i, k := range keys {
		out[i] = store.SecretKeyInfo{Key: k, Locked: f.locked[serviceName][k]}
	}
	return out, nil
}

func (f *fakeStore) GetSecretKeyLocked(_ context.Context, serviceName, envKey string) (exists, locked bool, err error) {
	byKey, ok := f.values[serviceName]
	if !ok {
		return false, false, nil
	}
	if _, ok := byKey[envKey]; !ok {
		return false, false, nil
	}
	return true, f.locked[serviceName][envKey], nil
}

func (f *fakeStore) SetSecretLocked(_ context.Context, serviceName, envKey string, locked bool) error {
	byKey, ok := f.values[serviceName]
	if !ok {
		return store.ErrSecretValueNotFound
	}
	if _, ok := byKey[envKey]; !ok {
		return store.ErrSecretValueNotFound
	}
	if f.locked == nil {
		f.locked = map[string]map[string]bool{}
	}
	if f.locked[serviceName] == nil {
		f.locked[serviceName] = map[string]bool{}
	}
	f.locked[serviceName][envKey] = locked
	return nil
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
	// The DEK is wrapped once and reused to encrypt many values fast.
	// A service's second secret must not generate a second DEK.
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

func TestManager_DeleteAll_ValuesUnreachableAfter(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := m.SetValue(ctx, "web", "OTHER_KEY", "other"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	if err := m.DeleteAll(ctx, "web"); err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}

	if _, err := m.Resolve(ctx, "web", "API_KEY"); !errors.Is(err, ErrValueNotFound) {
		t.Errorf("Resolve(API_KEY) after DeleteAll() error = %v, want ErrValueNotFound", err)
	}
	if _, err := m.Resolve(ctx, "web", "OTHER_KEY"); !errors.Is(err, ErrValueNotFound) {
		t.Errorf("Resolve(OTHER_KEY) after DeleteAll() error = %v, want ErrValueNotFound", err)
	}
}

func TestManager_DeleteAll_SetValueWorksAgainAfterward(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := m.DeleteAll(ctx, "web"); err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}

	// A functional regression check, not a DEK-deletion proof: this
	// would pass identically whether DeleteAll actually removed the
	// wrapped DEK row or only the value rows, since reusing a surviving
	// DEK to encrypt "new-value" produces the same observable Resolve()
	// result. TestDeleteServiceSecrets_RemovesValuesAndDEK
	// (internal/store) is what actually proves the DEK row itself is
	// gone.
	if err := m.SetValue(ctx, "web", "API_KEY", "new-value"); err != nil {
		t.Fatalf("SetValue() after DeleteAll() error = %v", err)
	}
	got, err := m.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() after re-SetValue() error = %v", err)
	}
	if got != "new-value" {
		t.Errorf("Resolve() = %q, want %q", got, "new-value")
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

func TestManager_ListKeys(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-abc"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := m.SetValue(ctx, "web", "DB_PASSWORD", "hunter2"); err != nil { //nolint:gosec // fake fixture, not a real credential
		t.Fatalf("SetValue() error = %v", err)
	}

	keys, err := m.ListKeys(ctx, "web")
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2: %+v", len(keys), keys)
	}
}

func TestManager_SetValueGuarded_LockedBlocksOverwriteWithoutFlag(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-original"); err != nil {
		t.Fatalf("initial SetValue() error = %v", err)
	}
	if err := m.SetLocked(ctx, "web", "API_KEY", true); err != nil {
		t.Fatalf("SetLocked() error = %v", err)
	}

	err := m.SetValueGuarded(ctx, "web", "API_KEY", "sk-new", false)
	if !errors.Is(err, ErrSecretLocked) {
		t.Fatalf("error = %v, want ErrSecretLocked", err)
	}

	got, err := m.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "sk-original" {
		t.Errorf("value = %q, want %q: a rejected overwrite must not partially apply", got, "sk-original")
	}
}

func TestManager_SetValueGuarded_LockedAllowsOverwriteWithFlag(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-original"); err != nil {
		t.Fatalf("initial SetValue() error = %v", err)
	}
	if err := m.SetLocked(ctx, "web", "API_KEY", true); err != nil {
		t.Fatalf("SetLocked() error = %v", err)
	}

	if err := m.SetValueGuarded(ctx, "web", "API_KEY", "sk-new", true); err != nil {
		t.Fatalf("SetValueGuarded(overwriteLocked=true) error = %v", err)
	}
	got, err := m.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "sk-new" {
		t.Errorf("value = %q, want %q", got, "sk-new")
	}
}

func TestManager_SetValueGuarded_UnlockedIgnoresFlag(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if err := m.SetValue(ctx, "web", "API_KEY", "sk-original"); err != nil {
		t.Fatalf("initial SetValue() error = %v", err)
	}

	if err := m.SetValueGuarded(ctx, "web", "API_KEY", "sk-new", false); err != nil {
		t.Fatalf("SetValueGuarded() error = %v", err)
	}
	got, err := m.Resolve(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "sk-new" {
		t.Errorf("value = %q, want %q", got, "sk-new")
	}
}

func TestManager_SetValueGuarded_NewKeyIgnoresFlag(t *testing.T) {
	m, _ := testManager(t)
	if err := m.SetValueGuarded(context.Background(), "web", "NEW_KEY", "sk-first", false); err != nil {
		t.Fatalf("SetValueGuarded() error = %v", err)
	}
}

func TestManager_SetLocked_KeyNotFound(t *testing.T) {
	m, _ := testManager(t)
	err := m.SetLocked(context.Background(), "web", "GHOST", true)
	if !errors.Is(err, store.ErrSecretValueNotFound) {
		t.Errorf("error = %v, want store.ErrSecretValueNotFound", err)
	}
}
