package alerting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// LogsSource is the narrow surface Engine needs to attach log lines to
// a firing crashloop event. *telemetry.Federator satisfies this
// structurally, the same as MetricsSource in evaluate.go.
type LogsSource interface {
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error)
}

// RuleStore is the narrow store surface Engine needs: list what to
// evaluate, persist the result. *DB satisfies this structurally. The
// two CertExpiry methods are what a kind=cert_expiry rule uses to
// remember its per-domain state across ticks (EvaluateCertExpiry,
// cert_expiry.go); every other rule kind never calls them.
type RuleStore interface {
	ListEnabledRules(ctx context.Context) ([]Rule, error)
	UpdateState(ctx context.Context, id string, pendingSince, firingSince *time.Time, firing bool, evaluatedAt time.Time, value *float64) error
	RecordNotificationDelivery(ctx context.Context, d NotificationDelivery) error
	UpsertCertExpiryObservation(ctx context.Context, o CertExpiryObservation) error
	ListCertExpiryObservations(ctx context.Context, ruleID string) ([]CertExpiryObservation, error)
}

// crashloopLogLines surfaces the last 200 lines of a crashlooping
// container's logs.
const crashloopLogLines = 200

// crashloopLogLookback bounds how far back Engine searches for those
// 200 lines. A crashlooping container's logs from the last 15 minutes
// are what's actually useful to see; going back further risks pulling
// in an earlier, unrelated incident's output instead.
const crashloopLogLookback = 15 * time.Minute

// Engine evaluates every enabled rule on an interval, persists each
// rule's updated state, and notifies only on a firing/resolved
// transition, never on every tick a rule stays in the same state
// (the same reasoning already documented on Event.Resolved:
// repeated identical notifications train an operator to ignore the
// channel).
type Engine struct {
	rules          RuleStore
	metrics        MetricsSource
	logs           LogsSource
	tracker        *RestartTracker
	certs          CertSource
	nodes          NodeSource
	nodeServices   NodeServiceSource
	scheduledTasks ScheduledTaskSource
	domainApps     AppDomainSource
	domainChecker  DomainCheckSource
	backups        BackupSource
	newNotifier    func(Rule) Notifier
	logger         *slog.Logger

	// autoRollback/autoRollbackNudger back MaybeAutoRollback
	// (crashloop.go), set via SetAutoRollback. Both nil (the default)
	// means the feature is off platform-wide regardless of any
	// individual app's own opt-in, the same "absence degrades, never
	// errors" shape every other optional Engine dependency above follows.
	autoRollback        AutoRollbackStore
	autoRollbackNudger  deploy.ReconcileNudger
	autoRollbackTracker *AutoRollbackTracker

	certExpiryWarningWindow     time.Duration
	certRenewalStalledThreshold time.Duration
	patchStatusThreshold        float64
	nodeDiskSpaceThreshold      float64
	nodeCPUThreshold            float64
	nodeMemoryThreshold         float64
	domainHealthCheckInterval   time.Duration
	domainHealthThrottle        *domainHealthThrottle
	backupMissingGracePeriod    time.Duration

	cpBackups      ControlPlaneBackupSource
	cpBackupMaxAge time.Duration

	logArchive LogArchiveSource

	nodeCertWarning, nodeCertCritical time.Duration

	noise *NoiseControl
}

// NewEngine builds an Engine. newNotifier defaults to a Notifier with no
// email capability configured if nil; a real caller passes a closure
// capturing an email.Sender instead. certs may be nil if no cert
// storage is configured; a kind=cert_expiry rule then logs a warning and
// is skipped each tick rather than evaluated. nodes and scheduledTasks
// may likewise be nil, in which case a kind=patch_status or
// kind=scheduled_task_failure rule is skipped the same way.
// certExpiryWarningWindow and certRenewalStalledThreshold fall back to
// DefaultCertExpiryWarningWindow/DefaultCertRenewalStalledThreshold when
// passed as 0; patchStatusThreshold falls back to
// DefaultPatchStatusThreshold, nodeDiskSpaceThreshold falls back to
// DefaultNodeDiskSpaceThresholdPercent, and nodeCPUThreshold/
// nodeMemoryThreshold fall back to DefaultNodeCPUThresholdPercent/
// DefaultNodeMemoryThresholdBytes, all the same way. nodeServices may be
// nil, in which case a kind=node_resource_usage rule is skipped the same
// way a kind=patch_status rule is when nodes is nil. domainApps and
// domainChecker may likewise be nil, in which case a kind=domain_health
// rule is skipped the same way; domainHealthCheckInterval falls back to
// DefaultDomainHealthCheckInterval when passed as 0. backups may likewise
// be nil, in which case a kind=backup_missing rule is skipped the same
// way; backupMissingGracePeriod falls back to
// DefaultBackupMissingGracePeriod when passed as 0.
func NewEngine(rules RuleStore, metrics MetricsSource, logs LogsSource, tracker *RestartTracker, certs CertSource, scheduledTasks ScheduledTaskSource, certExpiryWarningWindow, certRenewalStalledThreshold time.Duration, nodes NodeSource, patchStatusThreshold, nodeDiskSpaceThreshold float64, nodeServices NodeServiceSource, nodeCPUThreshold, nodeMemoryThreshold float64, domainApps AppDomainSource, domainChecker DomainCheckSource, domainHealthCheckInterval time.Duration, backups BackupSource, backupMissingGracePeriod time.Duration, newNotifier func(Rule) Notifier, logger *slog.Logger) *Engine {
	if newNotifier == nil {
		newNotifier = func(r Rule) Notifier { return NewNotifier(nil, nil, r) }
	}
	if logger == nil {
		logger = slog.Default()
	}
	if certExpiryWarningWindow <= 0 {
		certExpiryWarningWindow = DefaultCertExpiryWarningWindow
	}
	if certRenewalStalledThreshold <= 0 {
		certRenewalStalledThreshold = DefaultCertRenewalStalledThreshold
	}
	if patchStatusThreshold <= 0 {
		patchStatusThreshold = DefaultPatchStatusThreshold
	}
	if nodeDiskSpaceThreshold <= 0 {
		nodeDiskSpaceThreshold = DefaultNodeDiskSpaceThresholdPercent
	}
	if nodeCPUThreshold <= 0 {
		nodeCPUThreshold = DefaultNodeCPUThresholdPercent
	}
	if nodeMemoryThreshold <= 0 {
		nodeMemoryThreshold = DefaultNodeMemoryThresholdBytes
	}
	if domainHealthCheckInterval <= 0 {
		domainHealthCheckInterval = DefaultDomainHealthCheckInterval
	}
	if backupMissingGracePeriod <= 0 {
		backupMissingGracePeriod = DefaultBackupMissingGracePeriod
	}
	return &Engine{
		rules: rules, metrics: metrics, logs: logs, tracker: tracker, certs: certs, nodes: nodes, nodeServices: nodeServices, scheduledTasks: scheduledTasks,
		domainApps: domainApps, domainChecker: domainChecker, backups: backups,
		autoRollbackTracker: NewAutoRollbackTracker(),
		newNotifier:         newNotifier, logger: logger,
		certExpiryWarningWindow: certExpiryWarningWindow, certRenewalStalledThreshold: certRenewalStalledThreshold,
		patchStatusThreshold: patchStatusThreshold, nodeDiskSpaceThreshold: nodeDiskSpaceThreshold,
		nodeCPUThreshold: nodeCPUThreshold, nodeMemoryThreshold: nodeMemoryThreshold,
		domainHealthCheckInterval: domainHealthCheckInterval, domainHealthThrottle: newDomainHealthThrottle(),
		backupMissingGracePeriod: backupMissingGracePeriod,
	}
}

// SetAutoRollback wires crashloop auto-rollback into e: once set, a
// KindCrashloop rule that transitions to firing, or stays firing across a
// desired-image change, checks its app's own AutoRollbackOnCrashloop
// opt-in and, if set, rolls back automatically (MaybeAutoRollback,
// crashloop.go). A setter rather than a NewEngine parameter deliberately:
// this keeps every existing call site and test unaffected by an optional
// dependency most callers don't need, the same "nil until explicitly
// wired" shape st/nudger already have inside MaybeAutoRollback itself.
func (e *Engine) SetAutoRollback(st AutoRollbackStore, nudger deploy.ReconcileNudger) {
	e.autoRollback = st
	e.autoRollbackNudger = nudger
}

// SetControlPlaneBackups enables kind=control_plane_backup_stale rules. It is
// left unset when scheduled control plane backups are disabled, so those
// rules stay quiet. maxAge <= 0 falls back to DefaultControlPlaneBackupMaxAge.
func (e *Engine) SetControlPlaneBackups(src ControlPlaneBackupSource, maxAge time.Duration) {
	e.cpBackups = src
	e.cpBackupMaxAge = maxAge
}

// SetLogArchive enables kind=log_archive_stale rules.
func (e *Engine) SetLogArchive(src LogArchiveSource) { e.logArchive = src }

// SetNodeCertThresholds sets the default warning window and critical
// threshold for kind=node_cert_expiring rules. Zero keeps the defaults.
func (e *Engine) SetNodeCertThresholds(warning, critical time.Duration) {
	e.nodeCertWarning, e.nodeCertCritical = warning, critical
}

// SetNoiseControl enables silences, grouping, flapping control and alert
// history. Unset, every transition notifies directly.
func (e *Engine) SetNoiseControl(n *NoiseControl) { e.noise = n }

// Tick evaluates every enabled rule once. Errors from individual rules
// (a metrics query failing, a notification failing to send) are
// collected and joined, never stopping evaluation of the remaining
// rules: the same "one broken resource must not block the rest"
// principle reconcile.Engine.ReconcileAll already applies to
// controllers, applied here to alert rules.
func (e *Engine) Tick(ctx context.Context) error {
	rules, err := e.rules.ListEnabledRules(ctx)
	if err != nil {
		return fmt.Errorf("alerting: list enabled rules: %w", err)
	}

	now := time.Now()
	var errs []error
	if e.noise != nil {
		defer e.noise.Sweep(ctx, now, e.sendEvent)
	}

	for _, r := range rules {
		var next Rule
		var certNotices, patchNotices, diskSpaceNotices, resourceUsageNotices, domainHealthNotices, nodeNotices []string
		var taskFailureNotice, backupMissingNoticeText string
		switch r.Kind {
		case KindThreshold:
			next, err = EvaluateThreshold(ctx, e.metrics, r, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindCrashloop:
			next = EvaluateCrashloop(e.tracker, r, now)
		case KindCertExpiry:
			if e.certs == nil {
				e.logger.Warn("alerting: cert_expiry rule found but no cert source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, certNotices, err = EvaluateCertExpiry(ctx, e.certs, e.rules, r, e.certExpiryWarningWindow, e.certRenewalStalledThreshold, now, e.logger)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindPatchStatus:
			if e.nodes == nil {
				e.logger.Warn("alerting: patch_status rule found but no node source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, patchNotices, err = EvaluatePatchStatus(ctx, e.nodes, e.metrics, r, e.patchStatusThreshold, now, e.logger)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindScheduledTaskFailure:
			if e.scheduledTasks == nil {
				e.logger.Warn("alerting: scheduled_task_failure rule found but no scheduled task source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, taskFailureNotice, err = EvaluateScheduledTaskFailure(ctx, e.scheduledTasks, r, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindNodeDiskSpace:
			if e.nodes == nil {
				e.logger.Warn("alerting: node_disk_space rule found but no node source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, diskSpaceNotices, err = EvaluateNodeDiskSpace(ctx, e.nodes, e.metrics, r, e.nodeDiskSpaceThreshold, now, e.logger)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindNodeOffline:
			if e.nodes == nil {
				e.logger.Warn("alerting: node_offline rule found but no node source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, nodeNotices, err = EvaluateNodeOffline(ctx, e.nodes, r, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindNodeCertExpiring:
			if e.nodes == nil {
				e.logger.Warn("alerting: node_cert_expiring rule found but no node source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			warning, critical := e.nodeCertWarning, e.nodeCertCritical
			if warning <= 0 {
				warning = DefaultNodeCertWarning
			}
			if critical <= 0 {
				critical = DefaultNodeCertCritical
			}
			next, nodeNotices, err = EvaluateNodeCertExpiring(ctx, e.nodes, r, warning, critical, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindNodeResourceUsage:
			if e.nodes == nil || e.nodeServices == nil {
				e.logger.Warn("alerting: node_resource_usage rule found but no node/service source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, resourceUsageNotices, err = EvaluateNodeResourceUsage(ctx, e.nodes, e.nodeServices, e.metrics, r, e.nodeCPUThreshold, e.nodeMemoryThreshold, now, e.logger)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindDomainHealth:
			if e.domainApps == nil || e.domainChecker == nil {
				e.logger.Warn("alerting: domain_health rule found but no app/domain check source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			if !e.domainHealthThrottle.ready(r.ID, e.domainHealthCheckInterval, now) {
				continue
			}
			next, domainHealthNotices, err = EvaluateDomainHealth(ctx, e.domainApps, e.domainChecker, r, now, e.logger)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindBackupMissing:
			if e.backups == nil {
				e.logger.Warn("alerting: backup_missing rule found but no backup source configured, skipping", slog.String("rule_id", r.ID))
				continue
			}
			next, backupMissingNoticeText, err = EvaluateBackupMissing(ctx, e.backups, r, e.backupMissingGracePeriod, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindControlPlaneBackupStale:
			if e.cpBackups == nil {
				// Scheduled backups are disabled: nothing to be stale.
				continue
			}
			next, backupMissingNoticeText, err = EvaluateControlPlaneBackupStale(e.cpBackups, r, e.cpBackupMaxAge, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindLogArchiveStale:
			if e.logArchive == nil {
				continue
			}
			next, backupMissingNoticeText, err = EvaluateLogArchiveStale(ctx, e.logArchive, r, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		default:
			e.logger.Warn("alerting: rule has unknown kind, skipping", slog.String("rule_id", r.ID), slog.String("kind", string(r.Kind)))
			continue
		}

		if e.noise != nil {
			next = e.noise.ApplyStreak(r, next, now)
		}
		becameFiring := next.Firing && !r.Firing
		becameResolved := !next.Firing && r.Firing
		stillFiring := next.Firing && r.Firing

		if err := e.rules.UpdateState(ctx, r.ID, next.PendingSince, next.FiringSince, next.Firing, now, next.LastValue); err != nil {
			errs = append(errs, fmt.Errorf("rule %q: persist state: %w", r.ID, err))
			continue
		}

		switch {
		case becameFiring:
			e.dispatch(ctx, next, false, certNotices, patchNotices, diskSpaceNotices, resourceUsageNotices, domainHealthNotices, nodeNotices, taskFailureNotice, backupMissingNoticeText)
			if r.Kind == KindCrashloop && e.autoRollback != nil {
				MaybeAutoRollback(ctx, e.autoRollback, e.autoRollbackNudger, e.autoRollbackTracker, next.ResourceID, e.logger)
			}
		case becameResolved:
			e.dispatch(ctx, next, true, nil, nil, nil, nil, nil, nil, "", "")
		case stillFiring:
			// No dispatch here: a rule that's still firing sends no repeat
			// notification (see dispatch's own doc comment on why). But a
			// second, genuinely different bad deploy inside the same
			// RestartWindow never produces its own becameFiring transition,
			// so auto-rollback still needs a chance to re-examine this rule
			// on every tick it stays firing. Gated on autoRollbackTracker
			// already having an entry for this resourceID (armed): a
			// service that has never auto-rolled-back has nothing new for
			// MaybeAutoRollback to detect here, and skipping the check
			// avoids a store round trip for every other still-firing rule.
			if r.Kind == KindCrashloop && e.autoRollback != nil && e.autoRollbackTracker.armed(next.ResourceID) {
				MaybeAutoRollback(ctx, e.autoRollback, e.autoRollbackNudger, e.autoRollbackTracker, next.ResourceID, e.logger)
			}
		}
	}

	return errors.Join(errs...)
}

// dispatch sends one notification for r's transition. Failures are
// logged, not returned: a notification failing to send must never stop
// evaluation of other rules, and the rule's own state was already
// persisted successfully before dispatch is called, so a lost
// notification doesn't leave the rule's stored state inconsistent with
// reality, only the operator momentarily uninformed.
func (e *Engine) dispatch(ctx context.Context, r Rule, resolved bool, certNotices, patchNotices, diskSpaceNotices, resourceUsageNotices, domainHealthNotices, nodeNotices []string, taskFailureNotice, backupMissingNotice string) {
	// r.Enabled is already resolved against its attached channel
	// (scanRule): a disabled channel silences the rule the same way
	// DeployDispatcher.Dispatch skips a target with a disabled channel.
	if !r.Enabled {
		if e.noise != nil {
			e.noise.RecordSkipped(ctx, r, resolved, time.Now())
		}
		return
	}

	ev := Event{Rule: r, Resolved: resolved}
	if r.Kind == KindCrashloop && !resolved {
		ev.LogLines = e.fetchRecentLogLines(ctx, r.ResourceID)
	}
	if r.Kind == KindCertExpiry && !resolved {
		ev.CertNotices = certNotices
	}
	if r.Kind == KindPatchStatus && !resolved {
		ev.PatchNotices = patchNotices
	}
	if r.Kind == KindNodeDiskSpace && !resolved {
		ev.DiskSpaceNotices = diskSpaceNotices
	}
	if r.Kind == KindNodeOffline && !resolved {
		ev.NodeOfflineNotices = nodeNotices
	}
	if r.Kind == KindNodeCertExpiring && !resolved {
		ev.NodeCertNotices = nodeNotices
	}
	if r.Kind == KindNodeResourceUsage && !resolved {
		ev.ResourceUsageNotices = resourceUsageNotices
	}
	if r.Kind == KindScheduledTaskFailure && !resolved {
		ev.TaskFailureNotice = taskFailureNotice
	}
	if r.Kind == KindDomainHealth && !resolved {
		ev.DomainHealthNotices = domainHealthNotices
	}
	if (r.Kind == KindBackupMissing || r.Kind == KindControlPlaneBackupStale || r.Kind == KindLogArchiveStale) && !resolved {
		ev.BackupMissingNotice = backupMissingNotice
	}

	if e.noise != nil {
		e.noise.Route(ctx, ev, time.Now(), e.sendEvent)
		return
	}
	_ = e.sendEvent(ctx, ev)
}

// sendEvent delivers ev and records the channel delivery. The error is
// returned so NoiseControl can record the outcome; it is already logged.
func (e *Engine) sendEvent(ctx context.Context, ev Event) error {
	r := ev.Rule
	sendErr := e.newNotifier(r).Notify(ctx, ev)
	if sendErr != nil {
		e.logger.Error("alerting: notification failed",
			slog.String("rule_id", r.ID), slog.String("resource_id", r.ResourceID),
			slog.Bool("resolved", ev.Resolved), slog.String("error", sendErr.Error()))
	}

	trigger := "alert-fired"
	if ev.Resolved {
		trigger = "alert-resolved"
	}
	recordDelivery(ctx, e.rules, e.logger, r.ChannelID, trigger, sendErr)
	return sendErr
}

// fetchRecentLogLines returns the most recent up to crashloopLogLines
// messages for resourceID, oldest of that tail first (matching how a
// log viewer reads top-to-bottom). A query failure returns nil, logged,
// not propagated: a firing crashloop notification without log lines
// attached is still far more useful than no notification at all.
func (e *Engine) fetchRecentLogLines(ctx context.Context, resourceID string) []string {
	now := time.Now()
	entries, err := e.logs.QueryLogs(ctx, resourceID, now.Add(-crashloopLogLookback), now, "")
	if err != nil {
		e.logger.Error("alerting: fetch crashloop log lines failed",
			slog.String("resource_id", resourceID), slog.String("error", err.Error()))
		return nil
	}

	if len(entries) > crashloopLogLines {
		entries = entries[len(entries)-crashloopLogLines:]
	}
	lines := make([]string, len(entries))
	for i, entry := range entries {
		lines[i] = entry.Message
	}
	return lines
}

// Run calls Tick on interval until ctx is done, matching the shape of
// every other periodic loop in this codebase
// (telemetry.Collector.Run, cmd/levelrail's retention sweeps).
func (e *Engine) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if e.noise != nil {
				flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				e.noise.FlushAll(flushCtx, time.Now(), e.sendEvent)
				cancel()
			}
			return ctx.Err()
		case <-ticker.C:
			if err := e.Tick(ctx); err != nil {
				e.logger.Warn("alerting: evaluation tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}
