// Package spool owns the directory where upload handlers stage request bodies, so a crash
// cannot leave unbounded orphan files and one upload cannot fill the disk.
package spool

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	envDir      = "LEVELRAIL_SPOOL_DIR"
	envMaxAge   = "LEVELRAIL_SPOOL_MAX_AGE_SECONDS"
	envQuota    = "LEVELRAIL_SPOOL_QUOTA_BYTES"
	namePrefix  = "lr-spool-"
	fallbackAge = 3600
)

// ErrQuota is returned by Create when the spool already holds its byte quota.
var ErrQuota = errors.New("spool: quota exceeded")

// Runtime overrides win over env, so the settings API can change them live. Zero means unset.
var (
	maxAgeOverride atomic.Int64
	quotaOverride  atomic.Int64
)

// SetMaxAge sets the orphan age in seconds at runtime; 0 reverts to env.
func SetMaxAge(secs int64) { maxAgeOverride.Store(secs) }

// SetQuota sets the quota in bytes at runtime; 0 reverts to env.
func SetQuota(bytes int64) { quotaOverride.Store(bytes) }

// Dir returns the spool directory, created on demand. Only files with our prefix are ever
// counted or swept, so sharing the OS temp dir is safe.
func Dir() string {
	d := os.Getenv(envDir)
	if d == "" {
		d = os.TempDir()
	}
	_ = os.MkdirAll(d, 0o700) //nolint:gosec // operator-set spool dir, not request input
	return d
}

// MaxAge is how old a spool file must be before the sweeper treats it as an orphan.
func MaxAge() time.Duration {
	if v := maxAgeOverride.Load(); v > 0 {
		return time.Duration(v) * time.Second
	}
	if n, err := strconv.ParseInt(os.Getenv(envMaxAge), 10, 64); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return fallbackAge * time.Second
}

// Quota is the byte cap for the whole spool; 0 means unlimited.
func Quota() int64 {
	if v := quotaOverride.Load(); v > 0 {
		return v
	}
	if n, err := strconv.ParseInt(os.Getenv(envQuota), 10, 64); err == nil && n > 0 {
		return n
	}
	return 0
}

// Stats is the spool state shown in control plane health.
type Stats struct {
	Files      int   `json:"files"`
	Bytes      int64 `json:"bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
	Orphans    int   `json:"orphans"`
}

// Usage walks the spool once. Orphans are files older than MaxAge.
func Usage() Stats {
	st := Stats{QuotaBytes: Quota()}
	entries, err := os.ReadDir(Dir())
	if err != nil {
		return st
	}
	cutoff := time.Now().Add(-MaxAge())
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || !strings.HasPrefix(e.Name(), namePrefix) {
			continue
		}
		st.Files++
		st.Bytes += info.Size()
		if info.ModTime().Before(cutoff) {
			st.Orphans++
		}
	}
	return st
}

// Create opens a new spool file, refusing when the quota is already used up.
func Create(kind string) (*os.File, error) {
	if q := Quota(); q > 0 && Usage().Bytes >= q {
		return nil, ErrQuota
	}
	return os.CreateTemp(Dir(), namePrefix+kind+"-*")
}

// Sweep removes spool files older than MaxAge and returns how many it deleted.
func Sweep() int {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-MaxAge())
	removed := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || !strings.HasPrefix(e.Name(), namePrefix) || info.ModTime().After(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			removed++
		}
	}
	return removed
}
