// Package diskguard watches free space on the volumes the platform writes to, refuses risky operations before they fill a disk and keeps a reserve file so SQLite can still commit when space runs out.
package diskguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Env names for the thresholds, in MiB.
const (
	EnvLowFreeMB      = "APP_DISK_LOW_FREE_MB"
	EnvCriticalFreeMB = "APP_DISK_CRITICAL_FREE_MB"
	EnvReserveMB      = "APP_DISK_RESERVE_MB"
	EnvDockerRoot     = "APP_DISK_DOCKER_ROOT"
)

const (
	defaultLowMB      = 1024
	defaultCriticalMB = 512
	defaultReserveMB  = 16
	defaultDockerRoot = "/var/lib/docker"
	mib               = int64(1) << 20
	reserveFileName   = ".disk-reserve"
	reserveChunk      = 1 << 20

	// Interval is how often Run checks, kept low because a statfs call is cheap.
	Interval = time.Minute
)

// Level is how close a volume is to full.
type Level string

// Levels, worst last.
const (
	LevelUnknown  Level = "unknown"
	LevelOK       Level = "ok"
	LevelLow      Level = "low"
	LevelCritical Level = "critical"
)

// Volume names.
const (
	VolumeData   = "data"
	VolumeDocker = "docker"
)

// Op is a risky operation the guard can refuse.
type Op string

// Operations that write a lot.
const (
	OpBuild  Op = "build"
	OpPull   Op = "pull"
	OpBackup Op = "backup"
)

// Thresholds are free-byte floors.
type Thresholds struct {
	LowFree      int64 `json:"low_free_bytes"`
	CriticalFree int64 `json:"critical_free_bytes"`
	Reserve      int64 `json:"reserve_bytes"`
}

// ThresholdsFromEnv reads the thresholds, falling back to defaults on unset or invalid values.
func ThresholdsFromEnv(lookup func(string) (string, bool)) Thresholds {
	read := func(name string, def int64) int64 {
		if v, ok := lookup(name); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
				return n * mib
			}
		}
		return def * mib
	}
	t := Thresholds{LowFree: read(EnvLowFreeMB, defaultLowMB), CriticalFree: read(EnvCriticalFreeMB, defaultCriticalMB), Reserve: read(EnvReserveMB, defaultReserveMB)}
	if t.CriticalFree > t.LowFree {
		t.CriticalFree = t.LowFree
	}
	return t
}

// DockerRootFromEnv returns the Docker root dir to watch.
func DockerRootFromEnv(lookup func(string) (string, bool)) string {
	if v, ok := lookup(EnvDockerRoot); ok && v != "" {
		return v
	}
	return defaultDockerRoot
}

// Volume is one watched filesystem.
type Volume struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	FreeBytes  int64  `json:"free_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Level      Level  `json:"level"`
}

// Status is a point-in-time reading.
type Status struct {
	Volumes      []Volume   `json:"volumes"`
	Thresholds   Thresholds `json:"thresholds"`
	Worst        Level      `json:"worst"`
	ReserveBytes int64      `json:"reserve_held_bytes"`
}

// StatFunc reads free and total bytes for a path.
type StatFunc func(path string) (free, total int64, err error)

// SystemStat is the real statfs reader.
func SystemStat(path string) (free, total int64, err error) {
	t, used, err := telemetry.HostDiskBytes(path)
	if err != nil {
		return 0, 0, fmt.Errorf("diskguard: %w", err)
	}
	return max(t-used, 0), t, nil
}

// LowDiskError is returned when an operation is refused for lack of space.
type LowDiskError struct {
	Op        Op
	Volume    string
	Path      string
	FreeBytes int64
	NeedBytes int64
}

func (e *LowDiskError) Error() string {
	return fmt.Sprintf("refused %s: only %d MiB free on the %s volume (%s), need at least %d MiB. Use Free space now, then retry", e.Op, e.FreeBytes/mib, e.Volume, e.Path, e.NeedBytes/mib)
}

// IsNoSpace reports whether err is an out-of-space write error.
func IsNoSpace(err error) bool {
	return errors.Is(err, syscall.ENOSPC)
}

type target struct{ name, path string }

// Guard reads volumes on demand; Tick keeps the reserve and the alert state current.
type Guard struct {
	th      Thresholds
	stat    StatFunc
	targets []target
	reserve string
	logger  *slog.Logger
	// Notify receives a plain-text message on a level change to low or critical.
	Notify func(ctx context.Context, text string)

	mu        sync.Mutex
	lastLevel Level
}

// New builds a Guard for the data dir and the Docker root. stat nil means the real reader.
func New(dataDir, dockerRoot string, th Thresholds, stat StatFunc, logger *slog.Logger) *Guard {
	if stat == nil {
		stat = SystemStat
	}
	if logger == nil {
		logger = slog.Default()
	}
	g := &Guard{th: th, stat: stat, logger: logger, lastLevel: LevelOK}
	if dataDir != "" {
		g.targets = append(g.targets, target{VolumeData, dataDir})
		g.reserve = filepath.Join(dataDir, reserveFileName)
	}
	if dockerRoot != "" {
		g.targets = append(g.targets, target{VolumeDocker, dockerRoot})
	}
	return g
}

func (g *Guard) level(free int64) Level {
	switch {
	case free < g.th.CriticalFree:
		return LevelCritical
	case free < g.th.LowFree:
		return LevelLow
	}
	return LevelOK
}

func rank(l Level) int {
	switch l {
	case LevelCritical:
		return 3
	case LevelLow:
		return 2
	case LevelOK:
		return 1
	}
	return 0
}

// Status reads every volume now. An unreadable volume is reported unknown, never as a refusal.
func (g *Guard) Status() Status {
	st := Status{Thresholds: g.th, Worst: LevelOK, Volumes: []Volume{}}
	for _, t := range g.targets {
		v := Volume{Name: t.name, Path: t.path, Level: LevelUnknown}
		if free, total, err := g.stat(t.path); err == nil && total > 0 {
			v.FreeBytes, v.TotalBytes, v.Level = free, total, g.level(free)
		}
		if rank(v.Level) > rank(st.Worst) {
			st.Worst = v.Level
		}
		st.Volumes = append(st.Volumes, v)
	}
	if g.reserve != "" {
		if fi, err := os.Stat(g.reserve); err == nil {
			st.ReserveBytes = fi.Size()
		}
	}
	return st
}

func volumesFor(op Op) []string {
	switch op {
	case OpPull:
		return []string{VolumeDocker}
	case OpBackup:
		return []string{VolumeData}
	}
	return []string{VolumeData, VolumeDocker}
}

// Refuse returns a *LowDiskError when a volume op writes to is below the critical floor.
func (g *Guard) Refuse(op Op) error {
	if g == nil {
		return nil
	}
	want := map[string]bool{}
	for _, n := range volumesFor(op) {
		want[n] = true
	}
	for _, v := range g.Status().Volumes {
		if want[v.Name] && v.Level == LevelCritical {
			return &LowDiskError{Op: op, Volume: v.Name, Path: v.Path, FreeBytes: v.FreeBytes, NeedBytes: g.th.CriticalFree}
		}
	}
	return nil
}

// Tick refreshes the reserve and notifies once per drop into low or critical.
func (g *Guard) Tick(ctx context.Context) error {
	st := g.Status()
	g.syncReserve(st)
	g.mu.Lock()
	prev := g.lastLevel
	g.lastLevel = st.Worst
	g.mu.Unlock()
	if rank(st.Worst) >= rank(LevelLow) && rank(st.Worst) > rank(prev) {
		msg := describe(st)
		g.logger.Warn("diskguard: low disk space", slog.String("level", string(st.Worst)), slog.String("detail", msg))
		if g.Notify != nil {
			g.Notify(ctx, msg)
		}
	}
	return nil
}

// Run ticks on Interval until ctx ends, through the shared loop registry.
func (g *Guard) Run(ctx context.Context) error {
	return footprint.Every(ctx, "diskguard", Interval, g.logger, g.Tick)
}

func describe(st Status) string {
	out := "Low disk space (" + string(st.Worst) + "):"
	for _, v := range st.Volumes {
		if rank(v.Level) >= rank(LevelLow) {
			out += fmt.Sprintf(" %s volume %s has %d MiB free.", v.Name, v.Path, v.FreeBytes/mib)
		}
	}
	return out + " Builds, pulls and backups are refused below the critical floor. Use Free space now on the resilience card."
}

func (g *Guard) syncReserve(st Status) {
	if g.reserve == "" || g.th.Reserve <= 0 {
		return
	}
	var data *Volume
	for i := range st.Volumes {
		if st.Volumes[i].Name == VolumeData {
			data = &st.Volumes[i]
		}
	}
	if data == nil || data.Level == LevelUnknown {
		return
	}
	if data.Level == LevelCritical {
		if err := os.Remove(g.reserve); err == nil {
			g.logger.Warn("diskguard: released the disk reserve so the database can still commit")
		}
		return
	}
	if st.ReserveBytes == g.th.Reserve || data.FreeBytes < g.th.LowFree+g.th.Reserve {
		return
	}
	if err := writeReserve(g.reserve, g.th.Reserve); err != nil {
		g.logger.Warn("diskguard: could not create the disk reserve", slog.String("error", err.Error()))
	}
}

// ReleaseReserve drops the reserve now, for a caller that just saw ENOSPC.
func (g *Guard) ReleaseReserve() bool {
	if g == nil || g.reserve == "" {
		return false
	}
	return os.Remove(g.reserve) == nil
}

func writeReserve(path string, size int64) (err error) {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // path derives from the data dir
	if err != nil {
		return fmt.Errorf("diskguard: create reserve: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	buf := make([]byte, reserveChunk)
	for written := int64(0); written < size; written += reserveChunk {
		if _, err = f.Write(buf[:min(int64(reserveChunk), size-written)]); err != nil {
			_ = f.Close()
			return fmt.Errorf("diskguard: fill reserve: %w", err)
		}
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("diskguard: sync reserve: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("diskguard: close reserve: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("diskguard: publish reserve: %w", err)
	}
	return nil
}
