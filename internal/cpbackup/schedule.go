package cpbackup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ConfigUpdate is the operator-editable configuration.
type ConfigUpdate struct {
	Enabled        bool
	TargetID       string
	Recipients     []string
	Schedule       string
	DrillSchedule  string
	Retention      Retention
	EscrowTargetID string
}

const maxRetention = 3650

// ErrInvalid marks a configuration the operator must fix.
var ErrInvalid = errors.New("invalid configuration")

// UpdateConfig validates and stores the configuration.
func (s *Service) UpdateConfig(ctx context.Context, u ConfigUpdate) error {
	for _, expr := range []string{u.Schedule, u.DrillSchedule} {
		if expr == "" {
			continue
		}
		if _, err := cronexpr.Parse(expr); err != nil {
			return fmt.Errorf("%w: cron expression %q: %v", ErrInvalid, expr, err)
		}
	}
	for _, n := range []int{u.Retention.Daily, u.Retention.Weekly, u.Retention.Monthly} {
		if n < 0 || n > maxRetention {
			return fmt.Errorf("%w: retention counts must be between 0 and %d", ErrInvalid, maxRetention)
		}
	}
	recipients := make([]string, 0, len(u.Recipients))
	for _, r := range u.Recipients {
		if r = strings.TrimSpace(r); r != "" && !containsString(recipients, r) {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) > 0 {
		if _, err := ParseRecipients(recipients); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	for _, id := range []string{u.TargetID, u.EscrowTargetID} {
		if id == "" {
			continue
		}
		if _, _, err := s.Dest.Open(ctx, id); err != nil {
			return fmt.Errorf("%w: storage destination %q is not usable", ErrInvalid, id)
		}
	}
	if u.Enabled && (u.TargetID == "" || len(recipients) == 0) {
		return fmt.Errorf("%w: %v", ErrInvalid, ErrNotConfigured)
	}
	cfg := store.CPDRSettings{
		Enabled: u.Enabled, TargetID: u.TargetID, Recipients: recipients, Schedule: u.Schedule, DrillSchedule: u.DrillSchedule,
		RetainDaily: u.Retention.Daily, RetainWeekly: u.Retention.Weekly, RetainMonthly: u.Retention.Monthly, EscrowTargetID: u.EscrowTargetID,
	}
	if err := s.Store.UpdateCPDRConfig(ctx, cfg, s.now()); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

// Run checks for due backups and drills every Opts.Tick until ctx is done.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(s.Opts.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// Tick runs whatever is due right now: at most one backup, then at most one drill.
func (s *Service) Tick(ctx context.Context) {
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		s.logger().Error("read control plane dr settings", slog.String("error", err.Error()))
		return
	}
	if !cfg.Enabled || cfg.TargetID == "" || len(cfg.Recipients) == 0 {
		return
	}
	eff, now := s.effective(cfg), s.now().UTC()
	if due, slot := s.backupDue(cfg, eff, now); due {
		if m, err := s.RunBackup(ctx, slot); err != nil {
			if !errors.Is(err, ErrBusy) {
				s.logger().Error("off-box control plane backup failed", slog.String("error", err.Error()))
			}
		} else {
			s.logger().Info("off-box control plane backup uploaded", slog.String("key", m.Key), slog.Int64("size_bytes", m.SizeBytes))
		}
		if fresh, err := s.Store.GetCPDRSettings(ctx); err == nil {
			cfg = fresh
		}
	}
	if s.drillDue(cfg, eff, now) {
		if res, err := s.RunDrill(ctx); err != nil && !errors.Is(err, ErrBusy) {
			s.logger().Error("control plane restore drill failed", slog.String("error", err.Error()))
		} else if err == nil {
			s.logger().Info("control plane restore drill finished", slog.Bool("ok", res.OK), slog.Bool("partial", res.Partial))
		}
	}
}

func (s *Service) backupDue(cfg store.CPDRSettings, eff Effective, now time.Time) (bool, time.Time) {
	if cfg.LastBackupError != "" && now.Sub(cfg.LastAttemptAt) < s.Opts.RetryAfter {
		return false, time.Time{}
	}
	if cfg.LastBackupAt.IsZero() {
		return true, time.Time{}
	}
	sched, err := cronexpr.Parse(eff.Schedule)
	if err != nil {
		return false, time.Time{}
	}
	next := sched.Next(cfg.LastBackupAt)
	return !now.Before(next), next
}

func (s *Service) drillDue(cfg store.CPDRSettings, eff Effective, now time.Time) bool {
	if cfg.LastBackupKey == "" {
		return false
	}
	if cfg.LastDrillAt.IsZero() {
		return true
	}
	if !cfg.LastDrillOK && now.Sub(cfg.LastDrillAt) < s.Opts.RetryAfter {
		return false
	}
	sched, err := cronexpr.Parse(eff.DrillSchedule)
	if err != nil {
		return false
	}
	return !now.Before(sched.Next(cfg.LastDrillAt))
}
