// Package rehydrate plans and runs the data half of a whole-server recovery:
// after the control plane itself is restored, it pulls each database and app
// volume back from its latest usable backup and reports what happened.
package rehydrate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Environment variables that tune a rehydrate pass.
const (
	EnvMaxBackupAge = "APP_REHYDRATE_MAX_BACKUP_AGE"
	EnvConcurrency  = "APP_REHYDRATE_CONCURRENCY"

	defaultMaxBackupAge = 7 * 24 * time.Hour
	defaultConcurrency  = 2
)

// Item kinds.
const (
	KindDatabase = "database"
	KindVolume   = "volume"
)

// Item statuses. Planned is the dry-run state of an item a run would restore.
const (
	StatusPlanned   = "planned"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
	StatusBlocked   = "blocked"
)

// Reasons an item is skipped or blocked.
const (
	ReasonNoBackup        = "no_backup"
	ReasonBackupTooOld    = "backup_too_old"
	ReasonNoBackupsEngine = "engine_keeps_no_data"
)

// Report states.
const (
	StateRunning   = "running"
	StatePlanned   = "planned"
	StateSucceeded = "succeeded"
	StatePartial   = "partial"
	StateFailed    = "failed"
)

// Modes of a report.
const (
	ModePlan = "plan"
	ModeRun  = "run"
)

// ErrBusy is returned when a run is already in progress.
var ErrBusy = errors.New("rehydrate: a run is already in progress")

// ErrNotConfigured is returned when restores cannot run on this control plane.
var ErrNotConfigured = errors.New("rehydrate: restores are not configured")

// Config holds the env-driven limits.
type Config struct {
	// MaxBackupAge is the oldest backup a run will restore from; 0 disables the check.
	MaxBackupAge time.Duration
	Concurrency  int
}

// ConfigFromEnv reads the limits, falling back to defaults when unset.
func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{MaxBackupAge: defaultMaxBackupAge, Concurrency: defaultConcurrency}
	if v, ok := lookup(EnvMaxBackupAge); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return cfg, fmt.Errorf("rehydrate: %s must be a non-negative duration", EnvMaxBackupAge)
		}
		cfg.MaxBackupAge = d
	}
	if v, ok := lookup(EnvConcurrency); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("rehydrate: %s must be a positive integer", EnvConcurrency)
		}
		cfg.Concurrency = n
	}
	return cfg, nil
}

// Database is one desired database.
type Database struct {
	Name   string
	Engine string
	// Stateless engines keep nothing on disk and have nothing to restore.
	Stateless bool
}

// Volume is one named volume of a desired app.
type Volume struct {
	App  string
	Name string
}

// Backup is one backup attempt, reduced to what planning needs.
type Backup struct {
	ID         string
	Kind       string
	Database   string
	App        string
	Volume     string
	Succeeded  bool
	FinishedAt time.Time
	SizeBytes  int64
}

// Inventory reads what the restored control plane believes should exist.
type Inventory interface {
	Databases(ctx context.Context) ([]Database, error)
	Volumes(ctx context.Context) ([]Volume, error)
	Backups(ctx context.Context) ([]Backup, error)
}

// Restorer applies one backup through the existing restore runners and
// blocks until it finishes. It returns the restore history ID.
type Restorer interface {
	RestoreDatabase(ctx context.Context, d Database, backupID string) (string, error)
	RestoreVolume(ctx context.Context, v Volume, backupID string) (string, error)
}

// BackupRef is the backup an item restores from.
type BackupRef struct {
	ID         string    `json:"id"`
	FinishedAt time.Time `json:"finished_at"`
	SizeBytes  int64     `json:"size_bytes"`
	AgeSeconds int64     `json:"age_seconds"`
}

// Item is one resource in the plan and, after a run, its outcome.
type Item struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	App        string     `json:"app,omitempty"`
	Volume     string     `json:"volume,omitempty"`
	Database   string     `json:"database,omitempty"`
	Engine     string     `json:"engine,omitempty"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	Backup     *BackupRef `json:"backup,omitempty"`
	RestoreID  string     `json:"restore_id,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Summary counts items by outcome.
type Summary struct {
	Total      int `json:"total"`
	Restorable int `json:"restorable"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
	Blocked    int `json:"blocked"`
	// WorstBackupAgeSeconds is the data-loss window: the oldest backup used or planned.
	WorstBackupAgeSeconds int64 `json:"worst_backup_age_seconds"`
}

// Report is the plan and the DR report of a run; the same shape serves both.
type Report struct {
	ID                  string     `json:"id"`
	Mode                string     `json:"mode"`
	State               string     `json:"state"`
	GeneratedAt         time.Time  `json:"generated_at"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	MaxBackupAgeSeconds int64      `json:"max_backup_age_seconds"`
	Concurrency         int        `json:"concurrency"`
	AllowStale          bool       `json:"allow_stale"`
	Items               []Item     `json:"items"`
	Summary             Summary    `json:"summary"`
}

// PlanOptions tunes planning.
type PlanOptions struct {
	// AllowStale restores from backups older than MaxBackupAge instead of blocking them.
	AllowStale bool
}

// BuildPlan matches every database and volume to its newest usable backup.
func BuildPlan(ctx context.Context, inv Inventory, cfg Config, opts PlanOptions, now time.Time) (Report, error) {
	dbs, err := inv.Databases(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("rehydrate: list databases: %w", err)
	}
	vols, err := inv.Volumes(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("rehydrate: list volumes: %w", err)
	}
	backups, err := inv.Backups(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("rehydrate: list backups: %w", err)
	}

	newest := map[string]Backup{}
	for _, b := range backups {
		if !b.Succeeded {
			continue
		}
		k := backupKey(b)
		if cur, ok := newest[k]; !ok || b.FinishedAt.After(cur.FinishedAt) {
			newest[k] = b
		}
	}

	rep := Report{
		ID:                  "rh_" + strconv.FormatInt(now.UnixNano(), 36),
		Mode:                ModePlan,
		State:               StatePlanned,
		GeneratedAt:         now.UTC(),
		MaxBackupAgeSeconds: int64(cfg.MaxBackupAge / time.Second),
		Concurrency:         cfg.Concurrency,
		AllowStale:          opts.AllowStale,
		Items:               []Item{},
	}
	for _, d := range dbs {
		it := Item{ID: "database/" + d.Name, Kind: KindDatabase, Database: d.Name, Engine: d.Engine}
		if d.Stateless {
			it.Status, it.Reason = StatusSkipped, ReasonNoBackupsEngine
		} else {
			classify(&it, newest[it.ID], cfg, opts, now)
		}
		rep.Items = append(rep.Items, it)
	}
	for _, v := range vols {
		it := Item{ID: "volume/" + v.App + "/" + v.Name, Kind: KindVolume, App: v.App, Volume: v.Name}
		classify(&it, newest[it.ID], cfg, opts, now)
		rep.Items = append(rep.Items, it)
	}
	rep.Summary = summarize(rep.Items)
	return rep, nil
}

func backupKey(b Backup) string {
	if b.Kind == KindVolume {
		return "volume/" + b.App + "/" + b.Volume
	}
	return "database/" + b.Database
}

func classify(it *Item, b Backup, cfg Config, opts PlanOptions, now time.Time) {
	if b.ID == "" {
		it.Status, it.Reason = StatusBlocked, ReasonNoBackup
		return
	}
	age := now.Sub(b.FinishedAt)
	if age < 0 {
		age = 0
	}
	it.Backup = &BackupRef{ID: b.ID, FinishedAt: b.FinishedAt.UTC(), SizeBytes: b.SizeBytes, AgeSeconds: int64(age / time.Second)}
	if cfg.MaxBackupAge > 0 && age > cfg.MaxBackupAge && !opts.AllowStale {
		it.Status, it.Reason = StatusBlocked, ReasonBackupTooOld
		return
	}
	it.Status = StatusPlanned
}

func summarize(items []Item) Summary {
	s := Summary{Total: len(items)}
	for _, it := range items {
		switch it.Status {
		case StatusPlanned, StatusRunning:
			s.Restorable++
		case StatusSucceeded:
			s.Restorable++
			s.Succeeded++
		case StatusFailed:
			s.Restorable++
			s.Failed++
		case StatusSkipped:
			s.Skipped++
		case StatusBlocked:
			s.Blocked++
		}
		if it.Backup != nil && it.Status != StatusBlocked && it.Backup.AgeSeconds > s.WorstBackupAgeSeconds {
			s.WorstBackupAgeSeconds = it.Backup.AgeSeconds
		}
	}
	return s
}
