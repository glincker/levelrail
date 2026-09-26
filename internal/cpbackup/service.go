package cpbackup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env vars tuning off-box backups; unset or invalid values use the defaults.
const (
	EnvOffboxSchedule    = "APP_CONTROL_PLANE_OFFBOX_SCHEDULE"
	EnvRetainDaily       = "APP_CONTROL_PLANE_OFFBOX_RETAIN_DAILY"
	EnvRetainWeekly      = "APP_CONTROL_PLANE_OFFBOX_RETAIN_WEEKLY"
	EnvRetainMonthly     = "APP_CONTROL_PLANE_OFFBOX_RETAIN_MONTHLY"
	EnvMaxPrune          = "APP_CONTROL_PLANE_OFFBOX_MAX_PRUNE_PER_RUN"
	EnvTick              = "APP_CONTROL_PLANE_OFFBOX_TICK"
	EnvRetryAfter        = "APP_CONTROL_PLANE_OFFBOX_RETRY_AFTER"
	EnvDrillSchedule     = "APP_CONTROL_PLANE_DRILL_SCHEDULE"
	EnvDrillIdentityFile = "APP_CONTROL_PLANE_DRILL_IDENTITY_FILE"
	EnvGrace             = "APP_CONTROL_PLANE_DR_GRACE"
)

// Options tunes off-box backups and drills.
type Options struct {
	Schedule      string
	DrillSchedule string
	Retention     Retention
	MaxPrune      int
	Tick          time.Duration
	RetryAfter    time.Duration
	// Grace is how late a scheduled run may be before it counts as overdue.
	Grace time.Duration
	// OrphanAge is how old an object without its partner must be before pruning removes it.
	OrphanAge time.Duration
}

// DefaultOptions returns the built-in tuning: daily backups at 02:00, weekly drills.
func DefaultOptions() Options {
	return Options{
		Schedule: "0 2 * * *", DrillSchedule: "0 5 * * 0",
		Retention: Retention{Daily: 7, Weekly: 4, Monthly: 6},
		MaxPrune:  50, Tick: time.Minute, RetryAfter: 30 * time.Minute, Grace: 6 * time.Hour, OrphanAge: 24 * time.Hour,
	}
}

// OptionsFromEnv applies env overrides to DefaultOptions.
func OptionsFromEnv(lookup func(string) (string, bool)) Options {
	o := DefaultOptions()
	if v, ok := lookup(EnvOffboxSchedule); ok && validCron(v) {
		o.Schedule = v
	}
	if v, ok := lookup(EnvDrillSchedule); ok && validCron(v) {
		o.DrillSchedule = v
	}
	for env, dst := range map[string]*int{EnvRetainDaily: &o.Retention.Daily, EnvRetainWeekly: &o.Retention.Weekly, EnvRetainMonthly: &o.Retention.Monthly, EnvMaxPrune: &o.MaxPrune} {
		if v, ok := lookup(env); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	for env, dst := range map[string]*time.Duration{EnvTick: &o.Tick, EnvRetryAfter: &o.RetryAfter, EnvGrace: &o.Grace} {
		if v, ok := lookup(env); ok {
			if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
				*dst = d
			}
		}
	}
	return o
}

func validCron(expr string) bool {
	_, err := cronexpr.Parse(expr)
	return err == nil
}

// Bucket is the object store surface off-box backups use; *objectstore.Client satisfies it.
type Bucket interface {
	Put(ctx context.Context, key string, body io.ReadSeeker, contentType, contentEncoding string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix, token string, limit int32) (objectstore.ListPage, error)
}

// Location identifies a bucket well enough to tell whether two destinations are the same one.
type Location struct {
	TargetID string `json:"target_id"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Bucket   string `json:"bucket"`
}

// SameBucket reports whether two locations point at one bucket.
func (l Location) SameBucket(o Location) bool {
	if l.TargetID != "" && l.TargetID == o.TargetID {
		return true
	}
	return l.Bucket != "" && l.Bucket == o.Bucket && strings.EqualFold(strings.TrimRight(l.Endpoint, "/"), strings.TrimRight(o.Endpoint, "/"))
}

// Destinations resolves a stored storage destination to a bucket client.
type Destinations interface {
	Open(ctx context.Context, targetID string) (Bucket, Location, error)
}

// SettingsStore is the persistence surface the service needs.
type SettingsStore interface {
	GetCPDRSettings(ctx context.Context) (store.CPDRSettings, error)
	UpdateCPDRConfig(ctx context.Context, s store.CPDRSettings, now time.Time) error
	SetCPDRInstallID(ctx context.Context, id string) error
	RecordCPDRBackup(ctx context.Context, at time.Time, key, runErr string) error
	RecordCPDRDrill(ctx context.Context, at time.Time, ok, partial bool, detail string, ms int64) error
	RecordCPDREscrow(ctx context.Context, at time.Time) error
	AckCPDREscrow(ctx context.Context, at time.Time) error
}

// Errors callers map to responses.
var (
	ErrBusy          = errors.New("a run is already in progress")
	ErrNotConfigured = errors.New("off-box backups are not configured: choose a destination and at least one recipient")
	ErrNoBackups     = errors.New("no complete backup exists at the destination yet")
)

// Service runs encrypted off-box control plane backups, restore drills and escrow.
type Service struct {
	DB            Snapshotter
	Store         SettingsStore
	Dest          Destinations
	DataDir       string
	BinaryVersion string
	Opts          Options
	Logger        *slog.Logger
	Now           func() time.Time
	// DrillIdentities, when set, decrypt backups during drills. Their public
	// keys are added as recipients of every backup.
	DrillIdentities []age.Identity

	backupMu sync.Mutex
	drillMu  sync.Mutex
}

// NewService builds a Service with the wall clock.
func NewService(db Snapshotter, st SettingsStore, dest Destinations, dataDir, binaryVersion string, opts Options, logger *slog.Logger) *Service {
	return &Service{DB: db, Store: st, Dest: dest, DataDir: dataDir, BinaryVersion: binaryVersion, Opts: opts, Logger: logger, Now: time.Now}
}

// Effective is the configuration in force after applying env defaults.
type Effective struct {
	Schedule      string    `json:"schedule"`
	DrillSchedule string    `json:"drill_schedule"`
	Retention     Retention `json:"-"`
}

func (s *Service) effective(cfg store.CPDRSettings) Effective {
	e := Effective{Schedule: s.Opts.Schedule, DrillSchedule: s.Opts.DrillSchedule, Retention: s.Opts.Retention}
	if cfg.Schedule != "" {
		e.Schedule = cfg.Schedule
	}
	if cfg.DrillSchedule != "" {
		e.DrillSchedule = cfg.DrillSchedule
	}
	if cfg.RetainDaily > 0 {
		e.Retention.Daily = cfg.RetainDaily
	}
	if cfg.RetainWeekly > 0 {
		e.Retention.Weekly = cfg.RetainWeekly
	}
	if cfg.RetainMonthly > 0 {
		e.Retention.Monthly = cfg.RetainMonthly
	}
	return e
}

func (s *Service) recipients(cfg store.CPDRSettings) []string {
	out := append([]string(nil), cfg.Recipients...)
	for _, id := range s.DrillIdentities {
		if r, ok := RecipientOf(id); ok && !containsString(out, r) {
			out = append(out, r)
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// installID returns the stored install id, minting one on first use.
func (s *Service) installID(ctx context.Context, cfg store.CPDRSettings) (string, error) {
	if cfg.InstallID != "" {
		return cfg.InstallID, nil
	}
	id, err := newInstallID()
	if err != nil {
		return "", err
	}
	if err := s.Store.SetCPDRInstallID(ctx, id); err != nil {
		return "", fmt.Errorf("record install id: %w", err)
	}
	fresh, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("reload install id: %w", err)
	}
	return fresh.InstallID, nil
}
