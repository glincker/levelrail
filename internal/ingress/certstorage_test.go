package ingress

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeCertStore is a hand-written, in-memory fake for CertStore, the
// same pattern internal/reconcile/ingress/controller_test.go's fakeStore
// and fakeApplier already establish for this codebase: stateful enough
// for realistic scenarios (including injectable failures for the
// half-succeeded cases this codebase's testing standard requires), not
// just a call counter.
type fakeCertStore struct {
	mu sync.Mutex

	values map[string]store.CertStorageValue
	locks  map[string]time.Time // name -> last touched/acquired time

	saveErr    error
	getErr     error
	deleteErr  error
	existsErr  error
	listErr    error
	statErr    error
	acquireErr error
	touchErr   error
	releaseErr error
}

func newFakeCertStore() *fakeCertStore {
	return &fakeCertStore{
		values: map[string]store.CertStorageValue{},
		locks:  map[string]time.Time{},
	}
}

func (f *fakeCertStore) SaveCertStorageValue(_ context.Context, key string, value []byte) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = store.CertStorageValue{Value: value, ModifiedAt: time.Now()}
	return nil
}

func (f *fakeCertStore) GetCertStorageValue(_ context.Context, key string) (*store.CertStorageValue, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	if !ok {
		return nil, store.ErrCertStorageKeyNotFound
	}
	return &v, nil
}

func (f *fakeCertStore) DeleteCertStorageValue(_ context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, key)
	return nil
}

func (f *fakeCertStore) ExistsCertStorageValue(_ context.Context, key string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[key]
	return ok, nil
}

func (f *fakeCertStore) ListCertStorageKeys(_ context.Context, _ string, _ bool) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.values {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeCertStore) StatCertStorageValue(_ context.Context, key string) (*store.CertStorageKeyInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	if !ok {
		return nil, store.ErrCertStorageKeyNotFound
	}
	return &store.CertStorageKeyInfo{Key: key, ModifiedAt: v.ModifiedAt, Size: int64(len(v.Value)), IsTerminal: true}, nil
}

func (f *fakeCertStore) AcquireCertStorageLock(_ context.Context, name string, staleAfter time.Duration) (bool, error) {
	if f.acquireErr != nil {
		return false, f.acquireErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	last, held := f.locks[name]
	if held && time.Since(last) < staleAfter {
		return false, nil
	}
	f.locks[name] = time.Now()
	return true, nil
}

func (f *fakeCertStore) TouchCertStorageLock(_ context.Context, name string) error {
	if f.touchErr != nil {
		return f.touchErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locks[name] = time.Now()
	return nil
}

func (f *fakeCertStore) ReleaseCertStorageLock(_ context.Context, name string) error {
	if f.releaseErr != nil {
		return f.releaseErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.locks, name)
	return nil
}

func TestSQLiteStorage_StoreLoadRoundTrip(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))
	ctx := context.Background()

	if err := s.Store(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	got, err := s.Load(ctx, "k")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != "v" {
		t.Errorf("Load() = %q, want v", got)
	}
}

func TestSQLiteStorage_Load_MissingKeyReturnsFsErrNotExist(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))

	_, err := s.Load(context.Background(), "missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load() error = %v, want fs.ErrNotExist (certmagic.Storage's own documented contract)", err)
	}
}

func TestSQLiteStorage_Store_PropagatesStoreFailure(t *testing.T) {
	// The "storage read/write failure mid-reconcile" case this codebase's
	// testing standard requires a test for: a real database error
	// (connection lost, disk full) must surface as a real error, not be
	// swallowed.
	fc := newFakeCertStore()
	fc.saveErr = errors.New("disk full")
	s := NewSQLiteStorage(fc, testLogger(t))

	if err := s.Store(context.Background(), "k", []byte("v")); err == nil {
		t.Fatal("Store() error = nil, want the underlying store failure to surface")
	}
}

func TestSQLiteStorage_Load_PropagatesGenuineFailure(t *testing.T) {
	fc := newFakeCertStore()
	fc.getErr = errors.New("connection reset")
	s := NewSQLiteStorage(fc, testLogger(t))

	_, err := s.Load(context.Background(), "k")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load() error = %v, want a real error distinct from fs.ErrNotExist", err)
	}
}

func TestSQLiteStorage_Delete(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))
	ctx := context.Background()

	if err := s.Store(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if s.Exists(ctx, "k") {
		t.Error("Exists() = true after Delete(), want false")
	}
}

func TestSQLiteStorage_Exists_FailureTreatedAsNotFound(t *testing.T) {
	fc := newFakeCertStore()
	fc.existsErr = errors.New("db unavailable")
	s := NewSQLiteStorage(fc, testLogger(t))

	// Exists has no error return in certmagic.Storage's interface; a
	// lookup failure must degrade to false, not panic.
	if s.Exists(context.Background(), "k") {
		t.Error("Exists() = true on a lookup failure, want false")
	}
}

func TestSQLiteStorage_List_EmptyReturnsFsErrNotExist(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))

	_, err := s.List(context.Background(), "acme/nothing", true)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("List() error = %v, want fs.ErrNotExist", err)
	}
}

func TestSQLiteStorage_List_ReturnsKeys(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))
	ctx := context.Background()
	if err := s.Store(ctx, "acme/a", []byte("x")); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	got, err := s.List(ctx, "acme", true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0] != "acme/a" {
		t.Errorf("List() = %v, want [acme/a]", got)
	}
}

func TestSQLiteStorage_Stat(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))
	ctx := context.Background()
	if err := s.Store(ctx, "k", []byte("hello")); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	info, err := s.Stat(ctx, "k")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size != 5 || !info.IsTerminal {
		t.Errorf("Stat() = %+v, want Size=5 IsTerminal=true", info)
	}
}

func TestSQLiteStorage_Stat_MissingReturnsFsErrNotExist(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))

	if _, err := s.Stat(context.Background(), "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat() error = %v, want fs.ErrNotExist", err)
	}
}

func TestSQLiteStorage_LockUnlock_RoundTrip(t *testing.T) {
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))
	ctx := context.Background()

	if err := s.Lock(ctx, "issue-foo.com"); err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	if err := s.Unlock(ctx, "issue-foo.com"); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}

	// A lock released must be immediately re-acquirable.
	if err := s.Lock(ctx, "issue-foo.com"); err != nil {
		t.Fatalf("re-Lock() after Unlock() error = %v", err)
	}
	if err := s.Unlock(ctx, "issue-foo.com"); err != nil {
		t.Fatalf("second Unlock() error = %v", err)
	}
}

func TestSQLiteStorage_Lock_PropagatesAcquireFailure(t *testing.T) {
	fc := newFakeCertStore()
	fc.acquireErr = errors.New("db unavailable")
	s := NewSQLiteStorage(fc, testLogger(t))

	if err := s.Lock(context.Background(), "issue-foo.com"); err == nil {
		t.Fatal("Lock() error = nil, want the underlying acquire failure to surface rather than retrying forever")
	}
}

func TestSQLiteStorage_Lock_RespectsContextCancellation(t *testing.T) {
	fc := newFakeCertStore()
	// Hold the lock so the second Lock call below has to actually wait.
	if _, err := fc.AcquireCertStorageLock(context.Background(), "issue-foo.com", time.Hour); err != nil {
		t.Fatalf("seed AcquireCertStorageLock() error = %v", err)
	}
	s := NewSQLiteStorage(fc, testLogger(t))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := s.Lock(ctx, "issue-foo.com")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Lock() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestSQLiteStorage_Unlock_StopsLockRefreshWithoutPanic(t *testing.T) {
	// Unlock for a name that was never Locked (e.g. a caller cleaning up
	// defensively) must not panic on the internal refresh-goroutine
	// bookkeeping.
	fc := newFakeCertStore()
	s := NewSQLiteStorage(fc, testLogger(t))

	if err := s.Unlock(context.Background(), "never-locked"); err != nil {
		t.Fatalf("Unlock() on a never-locked name error = %v, want nil (ReleaseCertStorageLock is idempotent)", err)
	}
}
