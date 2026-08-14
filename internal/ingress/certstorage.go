package ingress

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"

	"github.com/GLINCKER/levelrail/internal/store"
)

// CertStore is the narrow surface SQLiteStorage needs from
// internal/store's SQLite-backed *store.DB (CLAUDE.md 4.7), so tests can
// fake it without a real database. *store.DB satisfies this.
type CertStore interface {
	SaveCertStorageValue(ctx context.Context, key string, value []byte) error
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
	DeleteCertStorageValue(ctx context.Context, key string) error
	ExistsCertStorageValue(ctx context.Context, key string) (bool, error)
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	StatCertStorageValue(ctx context.Context, key string) (*store.CertStorageKeyInfo, error)
	AcquireCertStorageLock(ctx context.Context, name string, staleAfter time.Duration) (bool, error)
	TouchCertStorageLock(ctx context.Context, name string) error
	ReleaseCertStorageLock(ctx context.Context, name string) error
}

const (
	// lockPollInterval is how often Lock retries after losing a race for
	// a currently-held, still-fresh lock. Matches the order of magnitude
	// certmagic's own FileStorage.obtainLock waits between attempts.
	lockPollInterval = 250 * time.Millisecond
	// lockStaleAfter is how long a lock can go unrefreshed before a
	// competitor is allowed to treat it as abandoned and take it over.
	// Comfortably longer than lockRefreshEvery so a live holder never
	// loses its own lock to normal scheduling jitter.
	lockStaleAfter = 2 * time.Minute
	// lockRefreshEvery is how often a held lock's row is touched to keep
	// it from looking stale to a competitor.
	lockRefreshEvery = 30 * time.Second
)

// SQLiteStorage implements certmagic.Storage over internal/store's
// embedded SQLite (CLAUDE.md 4.7), replacing Caddy's default FileStorage
// module for certificates and ACME account state. This is the concrete
// shape CLAUDE.md 4.5 and internal/reconcile/ingress/controller.go's own
// package doc comment have both already named as the real requirement
// (TASKS.md 3.6): every ingress-driving process pointed at the same
// database file sees the same certificates and issuance locks through
// this type, so two of them never independently re-obtain a certificate
// for a domain the other already has one for.
type SQLiteStorage struct {
	store  CertStore
	logger *slog.Logger

	mu          sync.Mutex
	refreshStop map[string]chan struct{}
}

var _ certmagic.Storage = (*SQLiteStorage)(nil)

// NewSQLiteStorage builds a SQLiteStorage backed by certStore. A nil
// logger falls back to slog.Default(), matching this package's Driver
// convention (see driver.go's New).
func NewSQLiteStorage(certStore CertStore, logger *slog.Logger) *SQLiteStorage {
	if logger == nil {
		logger = slog.Default()
	}
	return &SQLiteStorage{store: certStore, logger: logger}
}

// Store implements certmagic.Storage.
func (s *SQLiteStorage) Store(ctx context.Context, key string, value []byte) error {
	if err := s.store.SaveCertStorageValue(ctx, key, value); err != nil {
		return fmt.Errorf("ingress: cert storage: store %q: %w", key, err)
	}
	return nil
}

// Load implements certmagic.Storage.
func (s *SQLiteStorage) Load(ctx context.Context, key string) ([]byte, error) {
	v, err := s.store.GetCertStorageValue(ctx, key)
	if errors.Is(err, store.ErrCertStorageKeyNotFound) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("ingress: cert storage: load %q: %w", key, err)
	}
	return v.Value, nil
}

// Delete implements certmagic.Storage.
func (s *SQLiteStorage) Delete(ctx context.Context, key string) error {
	if err := s.store.DeleteCertStorageValue(ctx, key); err != nil {
		return fmt.Errorf("ingress: cert storage: delete %q: %w", key, err)
	}
	return nil
}

// Exists implements certmagic.Storage. The interface has no error
// return, so a lookup failure is treated as "doesn't exist" rather than
// panicking, logged so it's never silently swallowed: a false negative
// here just makes certmagic re-store/re-issue, safe, if wasteful, unlike
// a false positive, which could skip issuing a certificate that's
// actually missing.
func (s *SQLiteStorage) Exists(ctx context.Context, key string) bool {
	exists, err := s.store.ExistsCertStorageValue(ctx, key)
	if err != nil {
		s.logger.WarnContext(ctx, "ingress: cert storage: exists check failed, treating as not found",
			slog.String("key", key), slog.String("error", err.Error()))
		return false
	}
	return exists
}

// List implements certmagic.Storage. Mirrors certmagic.FileStorage.List:
// listing a prefix that matches nothing returns fs.ErrNotExist, the same
// error a filepath.Walk over a missing directory would surface.
func (s *SQLiteStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	keys, err := s.store.ListCertStorageKeys(ctx, prefix, recursive)
	if err != nil {
		return nil, fmt.Errorf("ingress: cert storage: list %q: %w", prefix, err)
	}
	if len(keys) == 0 {
		return nil, fs.ErrNotExist
	}
	return keys, nil
}

// Stat implements certmagic.Storage.
func (s *SQLiteStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	info, err := s.store.StatCertStorageValue(ctx, key)
	if errors.Is(err, store.ErrCertStorageKeyNotFound) {
		return certmagic.KeyInfo{}, fs.ErrNotExist
	}
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("ingress: cert storage: stat %q: %w", key, err)
	}
	return certmagic.KeyInfo{
		Key:        info.Key,
		Modified:   info.ModifiedAt,
		Size:       info.Size,
		IsTerminal: info.IsTerminal,
	}, nil
}

// Lock implements certmagic.Locker (embedded in certmagic.Storage). It
// blocks, polling every lockPollInterval, until it claims name's lock or
// ctx is cancelled. While held, a background goroutine refreshes the
// lock row every lockRefreshEvery so a live holder's lock never looks
// stale to a competitor; a holder that crashes without calling Unlock
// stops refreshing, and the row is treated as abandoned (and may be
// taken over) once it is older than lockStaleAfter, mirroring
// certmagic's own FileStorage doc comment on stale-lock handling.
func (s *SQLiteStorage) Lock(ctx context.Context, name string) error {
	for {
		acquired, err := s.store.AcquireCertStorageLock(ctx, name, lockStaleAfter)
		if err != nil {
			return fmt.Errorf("ingress: cert storage: lock %q: %w", name, err)
		}
		if acquired {
			s.startLockRefresh(name)
			return nil
		}
		select {
		case <-time.After(lockPollInterval):
		case <-ctx.Done():
			return fmt.Errorf("ingress: cert storage: lock %q: %w", name, ctx.Err())
		}
	}
}

// Unlock implements certmagic.Locker.
func (s *SQLiteStorage) Unlock(ctx context.Context, name string) error {
	s.stopLockRefresh(name)
	if err := s.store.ReleaseCertStorageLock(ctx, name); err != nil {
		return fmt.Errorf("ingress: cert storage: unlock %q: %w", name, err)
	}
	return nil
}

// startLockRefresh starts the background goroutine that keeps name's
// lock row from looking stale for as long as it is held. Deliberately
// not tied to the ctx passed into Lock: that context belongs to whatever
// ACME operation asked for the lock and may be cancelled independently
// of the lock actually being released, and a refresh that stopped the
// moment that context ended (rather than when Unlock is actually called)
// would make an in-progress, uncancelled operation's lock go stale out
// from under it.
func (s *SQLiteStorage) startLockRefresh(name string) {
	stop := make(chan struct{})
	s.mu.Lock()
	if s.refreshStop == nil {
		s.refreshStop = make(map[string]chan struct{})
	}
	s.refreshStop[name] = stop
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(lockRefreshEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refreshCtx, cancel := context.WithTimeout(context.Background(), lockPollInterval)
				if err := s.store.TouchCertStorageLock(refreshCtx, name); err != nil {
					s.logger.WarnContext(refreshCtx, "ingress: cert storage: lock refresh failed",
						slog.String("lock", name), slog.String("error", err.Error()))
				}
				cancel()
			case <-stop:
				return
			}
		}
	}()
}

// stopLockRefresh stops name's refresh goroutine, if one is running.
// Safe to call even if startLockRefresh was never called for name.
func (s *SQLiteStorage) stopLockRefresh(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stop, ok := s.refreshStop[name]; ok {
		close(stop)
		delete(s.refreshStop, name)
	}
}
