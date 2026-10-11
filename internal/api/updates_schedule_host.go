package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/dbupgrade"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradesched"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// UpgradeScheduleStore is the persistence behind scheduled and automatic
// upgrades. *store.DB satisfies it.
type UpgradeScheduleStore interface {
	upgradesched.Store
	UpdateUpgradePolicy(ctx context.Context, p store.UpgradePolicy) error
}

type upgradeScheduleConfig struct {
	store  UpgradeScheduleStore
	sender dbupgrade.NoticeSender
	cfg    upgradesched.Config
}

// WithUpgradeScheduling enables scheduled and automatic upgrades. sender
// delivers the alerts (the deploy dispatcher), nil keeps them in the log.
func WithUpgradeScheduling(s UpgradeScheduleStore, sender dbupgrade.NoticeSender, cfg upgradesched.Config) Option {
	return func(rt *Router) { rt.upgradeSchedule = &upgradeScheduleConfig{store: s, sender: sender, cfg: cfg} }
}

// UpgradeScheduler returns the scheduler to run, or nil when scheduling is
// not configured. It is built once and shared with the HTTP handlers so a
// cancel and a fire serialise on the same lock.
func (rt *Router) UpgradeScheduler() *upgradesched.Scheduler {
	if rt.upgradeSchedule == nil || rt.selfUpgrade == nil {
		return nil
	}
	rt.upgradeSchedOnce.Do(func() {
		c := rt.upgradeSchedule
		s := &upgradesched.Scheduler{
			Store: c.store, Host: upgradeHost{rt: rt}, Config: c.cfg, Logger: rt.logger,
			Current: func() string { return version.Version },
			NewID:   upgradesched.NewScheduleID,
			Audit:   rt.auditSystemUpgradeEvent,
		}
		if rt.alertNoise != nil {
			s.Windows = rt.alertNoise.ListMaintenanceWindows
		}
		if c.sender != nil && rt.notificationChannels != nil {
			s.Notifier = channelFanout{store: rt.notificationChannels, sender: c.sender}
		}
		rt.upgradeSched = s
	})
	return rt.upgradeSched
}

type notificationChannelLister interface {
	ListNotificationChannels(ctx context.Context) ([]alerting.NotificationChannel, error)
}

// channelFanout delivers a notice to the chosen channels, or to every
// enabled channel when none is chosen so an unconfigured policy still alerts.
type channelFanout struct {
	store  notificationChannelLister
	sender dbupgrade.NoticeSender
}

func (f channelFanout) Notify(ctx context.Context, ids []string, text string) error {
	all, err := f.store.ListNotificationChannels(ctx)
	if err != nil {
		return fmt.Errorf("list notification channels: %w", err)
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var errs []error
	for _, ch := range all {
		if !ch.Enabled || (len(want) > 0 && !want[ch.ID]) {
			continue
		}
		if err := f.sender.SendNotice(ctx, ch.Kind, ch.NotifyURL, text); err != nil {
			errs = append(errs, fmt.Errorf("channel %q: %w", ch.ID, err))
		}
	}
	return errors.Join(errs...)
}

// upgradeHost lets the scheduler use the same release lookup, preflight,
// backup and launcher that the dashboard's own buttons use.
type upgradeHost struct{ rt *Router }

func (h upgradeHost) Latest(ctx context.Context) (upgradesched.Candidate, bool, error) {
	rel, ok := h.rt.latestReleaseForCurrentChannel(ctx)
	if !ok || rel == nil {
		return upgradesched.Candidate{}, false, nil
	}
	c := upgradesched.Candidate{Tag: rel.Tag}
	if t, err := time.Parse(time.RFC3339, rel.PublishedAt); err == nil {
		c.PublishedAt = t
	}
	return c, true, nil
}

func (h upgradeHost) Plan(ctx context.Context, target string) (upgradesched.Plan, error) {
	rt := h.rt
	plan, status, msg := rt.buildSelfUpgradePlan(ctx, target)
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		return upgradesched.Plan{}, upgradesched.ErrReleaseNotFound
	default:
		return upgradesched.Plan{}, fmt.Errorf("%w: %s", upgradesched.ErrInvalid, msg)
	}
	out := upgradesched.Plan{
		Current: plan.CurrentVersion, Target: plan.TargetVersion, NotesAvailable: plan.NotesAvailable,
		CanApply: plan.CanApply, CannotApplyWhy: plan.CannotApplyWhy,
	}
	for _, b := range plan.Breaking {
		out.Breaking = append(out.Breaking, upgradesched.Breaking{ID: b.ID, Version: b.Version, Summary: b.Summary, RequiresAck: b.RequiresAck})
	}
	rel, found, err := rt.releaseOnChannel(ctx, target)
	if err != nil {
		return upgradesched.Plan{}, err
	}
	if !found {
		return upgradesched.Plan{}, upgradesched.ErrReleaseNotFound
	}
	if t, err := time.Parse(time.RFC3339, rel.PublishedAt); err == nil {
		out.PublishedAt = t
	}
	return out, nil
}

// releaseOnChannel finds tag among the releases the configured channel may
// install: stable takes only final releases, beta takes any, edge also takes
// the channel's own newest build.
func (rt *Router) releaseOnChannel(ctx context.Context, tag string) (upgrade.HistoryRelease, bool, error) {
	if rt.selfUpgrade == nil {
		return upgrade.HistoryRelease{}, false, errors.New("self-upgrade is not configured")
	}
	list, err := rt.selfUpgrade.releases(ctx)
	if err != nil {
		return upgrade.HistoryRelease{}, false, fmt.Errorf("list releases: %w", err)
	}
	channel := rt.currentUpdateChannel(ctx)
	for _, r := range list {
		if r.Tag != tag {
			continue
		}
		if channel == upgrade.ChannelStable && r.Prerelease {
			return upgrade.HistoryRelease{}, false, nil
		}
		return r, true, nil
	}
	if channel == upgrade.ChannelEdge {
		if rel, ok := rt.latestReleaseForCurrentChannel(ctx); ok && rel != nil && rel.Tag == tag {
			return upgrade.HistoryRelease{Tag: rel.Tag, PublishedAt: rel.PublishedAt, Body: rel.Body}, true, nil
		}
	}
	return upgrade.HistoryRelease{}, false, nil
}

func (h upgradeHost) Preflight(ctx context.Context, target string) (upgradesched.PreflightResult, error) {
	rt := h.rt
	rt.expireReleaseCaches()
	rel, ok := rt.latestReleaseForCurrentChannel(ctx)
	if target != "" && (!ok || rel == nil || rel.Tag != target) {
		found, hit, err := rt.releaseOnChannel(ctx, target)
		if err != nil {
			return upgradesched.PreflightResult{}, err
		}
		if !hit {
			return upgradesched.PreflightResult{}, upgradesched.ErrReleaseNotFound
		}
		names := make([]string, 0, len(found.Assets))
		for _, a := range found.Assets {
			names = append(names, a.Name)
		}
		rel, ok = &upgrade.Release{Tag: found.Tag, URL: found.URL, PublishedAt: found.PublishedAt, Body: found.Body, AssetNames: names}, true
	}
	res := rt.evaluatePreflight(ctx, rel, ok && rel != nil)
	return upgradesched.PreflightResult{Checks: res.Checks, Blocked: res.Blocked}, nil
}

func (h upgradeHost) BackupNow(ctx context.Context) (string, error) {
	if h.rt.cpBackups == nil {
		return "", upgradesched.ErrBackupNotAvail
	}
	info, err := h.rt.cpBackups.Create(ctx)
	if err != nil {
		return "", fmt.Errorf("create control plane backup: %w", err)
	}
	return info.Name, nil
}

func (h upgradeHost) Apply(ctx context.Context, target string, ack []string, initiator string) error {
	sc := h.rt.selfUpgrade
	if sc == nil {
		return errors.New("self-upgrade is not configured")
	}
	if ok, why := sc.canLaunch(); !ok {
		return errors.New(why)
	}
	args := []string{"--to", target, "--yes", "--initiator", initiator, "--json"}
	if len(ack) > 0 {
		args = append(args, "--ack", strings.Join(ack, ","))
	}
	return sc.launch(ctx, args)
}

func (h upgradeHost) Running(context.Context) bool {
	if h.rt.dataDir == "" {
		return false
	}
	running, _ := selfupgrade.NewJournal(h.rt.dataDir).Running()
	return len(running) > 0
}

func (h upgradeHost) Attempt(ctx context.Context, initiator string) (upgradesched.AttemptInfo, bool, error) {
	rt := h.rt
	if rt.selfUpgrade == nil {
		return upgradesched.AttemptInfo{}, false, nil
	}
	if rt.dataDir != "" {
		if _, err := selfupgrade.ImportJournal(ctx, selfupgrade.NewJournal(rt.dataDir), rt.selfUpgrade.store); err != nil {
			rt.logger.Warn("api: import self-upgrade journal failed", "error", err.Error())
		}
	}
	rows, err := rt.selfUpgrade.store.ListSelfUpgradeAttempts(ctx, selfUpgradeListLimit)
	if err != nil {
		return upgradesched.AttemptInfo{}, false, fmt.Errorf("list self-upgrade attempts: %w", err)
	}
	for _, row := range rows {
		if row.Initiator == initiator {
			return upgradesched.AttemptInfo{ID: row.ID, Outcome: row.Outcome, Error: row.Error}, true, nil
		}
	}
	return upgradesched.AttemptInfo{}, false, nil
}
