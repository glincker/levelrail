// Package integrity checks the control plane SQLite database for corruption and stages a verified snapshot to replace it.
package integrity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// Check modes.
const (
	ModeQuick = "quick"
	ModeFull  = "full"
)

// Environment variables that tune the checks.
const (
	EnvCheckInterval = "APP_INTEGRITY_CHECK_INTERVAL"
	EnvFullInterval  = "APP_INTEGRITY_FULL_INTERVAL"
	loopName         = "db-integrity"
	checkTimeout     = 2 * time.Minute
)

// ErrCorrupt means SQLite reported the database file as damaged.
var ErrCorrupt = errors.New("database file is corrupt")

// Querier is the slice of *sql.DB a check needs.
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Status is the outcome of the most recent check.
type Status struct {
	Healthy    bool      `json:"healthy"`
	Mode       string    `json:"mode"`
	CheckedAt  time.Time `json:"checked_at"`
	Error      string    `json:"error,omitempty"`
	Checks     int64     `json:"checks"`
	Failures   int64     `json:"failures"`
	LastFullAt time.Time `json:"last_full_at"`
}

// Checker runs quick and full integrity checks and remembers the result.
type Checker struct {
	db     Querier
	logger *slog.Logger
	now    func() time.Time

	mu     sync.Mutex
	status Status
}

// NewChecker returns a Checker over db. Before the first check it reports healthy.
func NewChecker(db Querier, logger *slog.Logger) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{db: db, logger: logger, now: time.Now, status: Status{Healthy: true, Mode: ModeQuick}}
}

// Status returns the latest result.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Check runs one check in the given mode (anything but ModeFull runs the cheap quick_check) and records it.
func (c *Checker) Check(ctx context.Context, mode string) Status {
	if mode != ModeFull {
		mode = ModeQuick
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	err := RunPragma(ctx, c.db, mode)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now().UTC()
	c.status.Mode, c.status.CheckedAt = mode, now
	c.status.Checks++
	if mode == ModeFull {
		c.status.LastFullAt = now
	}
	if err != nil {
		c.status.Healthy = false
		c.status.Error = err.Error()
		c.status.Failures++
		c.logger.Error("database integrity check failed", slog.String("mode", mode), slog.String("error", err.Error()))
		return c.status
	}
	c.status.Healthy, c.status.Error = true, ""
	return c.status
}

// Run checks on every interval until ctx ends, upgrading to a full check once fullEvery has passed since the last one.
func (c *Checker) Run(ctx context.Context, interval, fullEvery time.Duration) {
	if interval <= 0 {
		return
	}
	_ = footprint.Every(ctx, loopName, interval, c.logger, func(ctx context.Context) error {
		mode := ModeQuick
		if last := c.Status().LastFullAt; fullEvery > 0 && c.now().Sub(last) >= fullEvery {
			mode = ModeFull
		}
		if st := c.Check(ctx, mode); !st.Healthy {
			return errors.New(st.Error)
		}
		return nil
	})
}

// IntervalsFromEnv reads the quick and full check intervals; zero disables the loop.
func IntervalsFromEnv(getenv func(string) string) (quick, full time.Duration) {
	quick, full = 6*time.Hour, 7*24*time.Hour
	if d, err := time.ParseDuration(getenv(EnvCheckInterval)); err == nil && d >= 0 {
		quick = d
	}
	if d, err := time.ParseDuration(getenv(EnvFullInterval)); err == nil && d >= 0 {
		full = d
	}
	return quick, full
}

// RunPragma runs quick_check or integrity_check and returns an ErrCorrupt wrapped error unless SQLite answers ok.
func RunPragma(ctx context.Context, db Querier, mode string) error {
	pragma := "PRAGMA quick_check"
	if mode == ModeFull {
		pragma = "PRAGMA integrity_check"
	}
	var result string
	if err := db.QueryRowContext(ctx, pragma).Scan(&result); err != nil {
		if isCorruption(err) {
			return fmt.Errorf("%w: %s", ErrCorrupt, err.Error())
		}
		return fmt.Errorf("run %s: %w", strings.ToLower(pragma), err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrCorrupt, result)
	}
	return nil
}

func isCorruption(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"malformed", "not a database", "corrupt", "disk image"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// BootCheck runs a quick_check on the database file before the store opens it. A missing file is fine.
func BootCheck(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	sdb, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = sdb.Close() }()
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	return RunPragma(ctx, sdb, ModeQuick)
}
