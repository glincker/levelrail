package selfupgrade

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Trust root states reported to the preflight.
const (
	TrustRootReady       = "ready"
	TrustRootCached      = "cached"
	TrustRootChecking    = "checking"
	TrustRootUnavailable = "unavailable"
)

const (
	trustRootOKTTL   = 10 * time.Minute
	trustRootRetry   = 2 * time.Minute
	trustRootTimeout = 20 * time.Second
	tufCacheDirName  = "sigstore-tuf"
)

// TUFCacheDir is where Sigstore trust metadata is kept under the data dir.
func TUFCacheDir(dataDir string) string { return filepath.Join(dataDir, tufCacheDirName) }

// TrustRoot tracks whether the Sigstore trust root can be loaded, refreshing
// it in the background so callers never wait on the network.
type TrustRoot struct {
	cacheDir string
	load     func(cacheDir string) error
	now      func() time.Time
	timeout  time.Duration
	logger   *slog.Logger

	mu       sync.Mutex
	inflight bool
	checked  bool
	loaded   bool
	lastOK   time.Time
	lastFail time.Time
}

// NewTrustRoot returns a tracker backed by the loader verification uses.
func NewTrustRoot(cacheDir string, logger *slog.Logger) *TrustRoot {
	return newTrustRoot(cacheDir, func(dir string) error {
		_, err := loadTrustedRoot(dir)
		return err
	}, logger)
}

func newTrustRoot(cacheDir string, load func(string) error, logger *slog.Logger) *TrustRoot {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &TrustRoot{cacheDir: cacheDir, load: load, now: time.Now, timeout: trustRootTimeout, logger: logger}
}

// Warm starts a background refresh unless one is running or the last result is fresh.
func (t *TrustRoot) Warm() { _ = t.State() }

// State returns the current state without blocking, and starts a background
// refresh when the last result is missing or stale.
func (t *TrustRoot) State() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.inflight && t.due() {
		t.inflight = true
		go t.refresh()
	}
	switch {
	case t.loaded:
		return TrustRootReady
	case t.cached():
		return TrustRootCached
	case !t.checked || t.inflight:
		return TrustRootChecking
	default:
		return TrustRootUnavailable
	}
}

func (t *TrustRoot) due() bool {
	if !t.checked {
		return true
	}
	now := t.now()
	if t.lastFail.After(t.lastOK) && now.Sub(t.lastFail) < trustRootRetry {
		return false
	}
	return !t.loaded || now.Sub(t.lastOK) >= trustRootOKTTL
}

func (t *TrustRoot) cached() bool {
	entries, err := os.ReadDir(t.cacheDir)
	return err == nil && len(entries) > 0
}

func (t *TrustRoot) refresh() {
	done := make(chan error, 1)
	go func() { done <- t.load(t.cacheDir) }()
	var err error
	timer := time.NewTimer(t.timeout)
	defer timer.Stop()
	select {
	case err = <-done:
	case <-timer.C:
		err = context.DeadlineExceeded
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inflight, t.checked = false, true
	if err != nil {
		t.lastFail = t.now()
		t.logger.Warn("sigstore trust root not loaded, will retry", slog.String("error", err.Error()))
		return
	}
	t.loaded, t.lastOK = true, t.now()
}
