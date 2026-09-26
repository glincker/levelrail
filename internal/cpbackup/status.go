package cpbackup

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Warning is one actionable finding about the DR setup.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Checklist is the guided setup progress.
type Checklist struct {
	DestinationChosen  bool `json:"destination_chosen"`
	RecipientSet       bool `json:"recipient_set"`
	EscrowAcknowledged bool `json:"escrow_acknowledged"`
	DrillPassed        bool `json:"drill_passed"`
}

// DrillStatus is the last drill outcome.
type DrillStatus struct {
	At         *time.Time `json:"at,omitempty"`
	OK         bool       `json:"ok"`
	Partial    bool       `json:"partial"`
	Detail     string     `json:"detail"`
	DurationMs int64      `json:"duration_ms"`
}

// Status is the disaster recovery state shown in the UI, CLI and doctor. It
// carries public recipients only, never key material.
type Status struct {
	Enabled                 bool        `json:"enabled"`
	Configured              bool        `json:"configured"`
	TargetID                string      `json:"target_id"`
	TargetName              string      `json:"target_name,omitempty"`
	InstallID               string      `json:"install_id"`
	Recipients              []string    `json:"recipients"`
	Schedule                string      `json:"schedule"`
	DrillSchedule           string      `json:"drill_schedule"`
	RetainDaily             int         `json:"retain_daily"`
	RetainWeekly            int         `json:"retain_weekly"`
	RetainMonthly           int         `json:"retain_monthly"`
	EscrowTargetID          string      `json:"escrow_target_id"`
	EscrowGeneratedAt       *time.Time  `json:"escrow_generated_at,omitempty"`
	EscrowAckedAt           *time.Time  `json:"escrow_acked_at,omitempty"`
	NextBackupAt            *time.Time  `json:"next_backup_at,omitempty"`
	NextDrillAt             *time.Time  `json:"next_drill_at,omitempty"`
	LastBackupAt            *time.Time  `json:"last_backup_at,omitempty"`
	LastBackupKey           string      `json:"last_backup_key,omitempty"`
	LastBackupError         string      `json:"last_backup_error,omitempty"`
	LastAttemptAt           *time.Time  `json:"last_attempt_at,omitempty"`
	LastDrill               DrillStatus `json:"last_drill"`
	DrillIdentityConfigured bool        `json:"drill_identity_configured"`
	BackupRunning           bool        `json:"backup_running"`
	DrillRunning            bool        `json:"drill_running"`
	Checklist               Checklist   `json:"checklist"`
	Warnings                []Warning   `json:"warnings"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nextAfter(expr string, last, now time.Time) *time.Time {
	sched, err := cronexpr.Parse(expr)
	if err != nil {
		return nil
	}
	from := last
	if from.IsZero() || from.Before(now.Add(-366*24*time.Hour)) {
		from = now
	}
	n := sched.Next(from)
	return &n
}

func overdue(expr string, last, now time.Time, grace time.Duration) bool {
	if last.IsZero() {
		return false
	}
	sched, err := cronexpr.Parse(expr)
	if err != nil {
		return false
	}
	return now.After(sched.Next(last).Add(grace))
}

// Status assembles the current disaster recovery state.
func (s *Service) Status(ctx context.Context) (Status, error) {
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("load settings: %w", err)
	}
	now := s.now().UTC()
	eff := s.effective(cfg)
	st := Status{
		Enabled: cfg.Enabled, Configured: cfg.TargetID != "" && len(cfg.Recipients) > 0,
		TargetID: cfg.TargetID, InstallID: cfg.InstallID, Recipients: nonNil(cfg.Recipients),
		Schedule: eff.Schedule, DrillSchedule: eff.DrillSchedule,
		RetainDaily: eff.Retention.Daily, RetainWeekly: eff.Retention.Weekly, RetainMonthly: eff.Retention.Monthly,
		EscrowTargetID: cfg.EscrowTargetID, EscrowGeneratedAt: timePtr(cfg.EscrowGeneratedAt), EscrowAckedAt: timePtr(cfg.EscrowAckedAt),
		LastBackupAt: timePtr(cfg.LastBackupAt), LastBackupKey: cfg.LastBackupKey, LastBackupError: cfg.LastBackupError,
		LastAttemptAt: timePtr(cfg.LastAttemptAt), DrillIdentityConfigured: len(s.DrillIdentities) > 0,
		BackupRunning: s.BackupRunning(), DrillRunning: s.DrillRunning(),
		LastDrill: DrillStatus{At: timePtr(cfg.LastDrillAt), OK: cfg.LastDrillOK, Partial: cfg.LastDrillPartial, Detail: cfg.LastDrillDetail, DurationMs: cfg.LastDrillMs},
		Warnings:  []Warning{},
	}
	if cfg.Enabled && st.Configured {
		st.NextBackupAt = nextAfter(eff.Schedule, cfg.LastBackupAt, now)
		st.NextDrillAt = nextAfter(eff.DrillSchedule, cfg.LastDrillAt, now)
	}
	st.Checklist = Checklist{
		DestinationChosen: cfg.TargetID != "", RecipientSet: len(cfg.Recipients) > 0,
		EscrowAcknowledged: !cfg.EscrowAckedAt.IsZero(),
		DrillPassed:        !cfg.LastDrillAt.IsZero() && cfg.LastDrillOK,
	}
	if cfg.TargetID != "" {
		if _, loc, err := s.Dest.Open(ctx, cfg.TargetID); err == nil {
			st.TargetName = loc.Name
		}
	}
	st.Warnings = s.warnings(ctx, cfg, eff, now)
	return st, nil
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (s *Service) warnings(ctx context.Context, cfg store.CPDRSettings, eff Effective, now time.Time) []Warning {
	out := []Warning{}
	add := func(code, msg string) { out = append(out, Warning{Code: code, Message: msg}) }
	if cfg.TargetID == "" {
		add("no_destination", "Choose a storage destination for off-box backups.")
	}
	if len(cfg.Recipients) == 0 {
		add("no_recipients", "Add at least one age public key (recipient). Backups are encrypted to it, so only its private key can read them.")
	}
	if cfg.EscrowTargetID != "" && cfg.TargetID != "" {
		if same, err := s.sameBucket(ctx, cfg.TargetID, cfg.EscrowTargetID); err == nil && same {
			add("escrow_same_bucket", "The escrow destination is the same bucket as the backups. Anyone who can read that bucket gets both the backup and the key to it. Use a separate destination, or keep escrow offline.")
		}
	}
	if !cfg.Enabled || cfg.TargetID == "" || len(cfg.Recipients) == 0 {
		return out
	}
	if cfg.LastBackupError != "" {
		add("backup_failed", "The last off-box backup failed: "+cfg.LastBackupError)
	}
	if overdue(eff.Schedule, cfg.LastBackupAt, now, s.Opts.Grace) {
		add("backup_overdue", "The newest off-box backup is older than its schedule allows.")
	}
	switch {
	case cfg.EscrowGeneratedAt.IsZero():
		add("escrow_missing", "No escrow bundle has been generated. Without the master key a restored database cannot decrypt its secrets.")
	case cfg.EscrowAckedAt.IsZero():
		add("escrow_unacknowledged", "The escrow bundle was generated but not confirmed as stored offline.")
	}
	switch {
	case cfg.LastDrillAt.IsZero():
		add("no_drill", "No restore drill has run yet. An untested backup is a hope, not a backup.")
	case !cfg.LastDrillOK:
		add("drill_failed", "The last restore drill failed: "+cfg.LastDrillDetail)
	case cfg.LastDrillPartial:
		add("drill_partial", "Drills only verify checksums because no drill identity is configured, so decryption is untested.")
	}
	if overdue(eff.DrillSchedule, cfg.LastDrillAt, now, s.Opts.Grace) {
		add("drill_overdue", "The restore drill is overdue.")
	}
	return out
}

func (s *Service) sameBucket(ctx context.Context, a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	_, la, err := s.Dest.Open(ctx, a)
	if err != nil {
		return false, err
	}
	_, lb, err := s.Dest.Open(ctx, b)
	if err != nil {
		return false, err
	}
	return la.SameBucket(lb), nil
}

// Problem returns a one-line description of what is wrong with the running
// off-box backups or drills, or "" when healthy or not enabled.
func (s *Service) Problem(now time.Time) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil || !cfg.Enabled || cfg.TargetID == "" || len(cfg.Recipients) == 0 {
		return ""
	}
	eff := s.effective(cfg)
	switch {
	case cfg.LastBackupError != "":
		return "control plane off-box backup failed: " + cfg.LastBackupError
	case overdue(eff.Schedule, cfg.LastBackupAt, now, s.Opts.Grace):
		return "control plane off-box backup is overdue"
	case !cfg.LastDrillAt.IsZero() && !cfg.LastDrillOK:
		return "control plane restore drill failed: " + cfg.LastDrillDetail
	case overdue(eff.DrillSchedule, cfg.LastDrillAt, now, s.Opts.Grace):
		return "control plane restore drill is overdue"
	}
	return ""
}

// DRProblem lets the stale-backup alert rule fire on off-box failures and failed drills.
func (s *Service) DRProblem(now time.Time) string { return s.Problem(now) }

// AlertSource adapts the local snapshot manager and the off-box service to the
// stale-backup alert rule: when off-box backups are enabled their age counts.
type AlertSource struct {
	Local *Manager
	Svc   *Service
	// LocalScheduled is false when scheduled local snapshots are switched off,
	// so their absence is not staleness.
	LocalScheduled bool
}

// Newest reports the newest off-box backup when enabled, else the newest local snapshot.
func (a AlertSource) Newest() (time.Time, bool, error) {
	if a.Svc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if cfg, err := a.Svc.Store.GetCPDRSettings(ctx); err == nil && cfg.Enabled && cfg.TargetID != "" {
			return cfg.LastBackupAt, !cfg.LastBackupAt.IsZero(), nil
		}
	}
	if !a.LocalScheduled {
		return time.Time{}, false, nil
	}
	return a.Local.Newest()
}

// DRProblem forwards to the service.
func (a AlertSource) DRProblem(now time.Time) string {
	if a.Svc == nil {
		return ""
	}
	return a.Svc.Problem(now)
}

// ResolverDestinations adapts an objectstore.Resolver to Destinations.
type ResolverDestinations struct{ Resolver *objectstore.Resolver }

// Open resolves a stored destination to a client and its location.
func (d ResolverDestinations) Open(ctx context.Context, targetID string) (Bucket, Location, error) {
	c, t, err := d.Resolver.Client(ctx, targetID)
	if err != nil {
		return nil, Location{}, err
	}
	return c, Location{TargetID: t.ID, Name: t.Name, Endpoint: t.Endpoint, Bucket: t.Bucket}, nil
}
