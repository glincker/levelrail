package deploywatch

import (
	"context"
	"errors"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/changes"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// WatchStore is the store surface the Watcher needs. *store.DB satisfies it.
type WatchStore interface {
	deploy.ImageDeployStore
	ListGateEventsSince(ctx context.Context, phase, status string, since time.Time) ([]store.DeployGateEvent, error)
	ListGateEvents(ctx context.Context, attemptID string) ([]store.DeployGateEvent, error)
	RecordGateEvent(ctx context.Context, e store.DeployGateEvent) error
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	GetDeployAttempt(ctx context.Context, id string) (*store.DeployAttempt, error)
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
	GetDeploySafetySettings(ctx context.Context, serviceName string) (store.DeploySafetySettings, error)
	FailDeployAttemptWithReason(ctx context.Context, id, reason, message string) (bool, error)
	SetDeployAttemptReason(ctx context.Context, id, reason string) error
	ListRollbackPins(ctx context.Context, serviceName string) ([]store.RollbackPin, error)
}

// RestartCounter counts restarts of the named containers since a time.
// alerting.RestartTracker satisfies it.
type RestartCounter interface {
	CountContainersSince(names []string, since time.Time) int
}

// Notifier tells operators a live release was rolled back or needs one.
type Notifier func(ctx context.Context, app, image, message string)

// Watcher watches each release for its post-deploy window.
type Watcher struct {
	Store    WatchStore
	Defaults deploysafety.Effective
	// Inspectors returns the exit-state inspector for an app's node.
	Inspectors func(nodeID string) (docker.ExitStateInspector, error)
	Restarts   RestartCounter
	Metrics    telemetry.MetricsQuerier
	Resolver   docker.ImageResolver
	Nudger     deploy.ReconcileNudger
	Notify     Notifier
	// SampleInterval is the request sampler's tick; samples within one tick
	// of cutover are left out of the 5xx rate.
	SampleInterval time.Duration
	// Audit, when set, records automatic rollbacks in the audit log.
	Audit  AuditSink
	Logger *slog.Logger
	now    func() time.Time
	soft   softState
}

// Run ticks every interval until ctx is done.
func (w *Watcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		w.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Watcher) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *Watcher) logger() *slog.Logger {
	if w.Logger == nil {
		return slog.Default()
	}
	return w.Logger
}

// Tick evaluates every release whose watch window may still be open.
func (w *Watcher) Tick(ctx context.Context) {
	now := w.clock()
	horizon := deploysafety.MaxOverrideWindow
	if w.Defaults.WatchWindow > horizon {
		horizon = w.Defaults.WatchWindow
	}
	cutovers, err := w.Store.ListGateEventsSince(ctx, deploysafety.PhaseCutover, deploysafety.StatusPassed, now.Add(-horizon))
	if err != nil {
		w.logger().Error("deploywatch: list cutovers failed", slog.String("error", err.Error()))
		return
	}
	latest := map[string]string{}
	for _, e := range cutovers {
		latest[e.ServiceName] = e.DeployAttemptID
	}
	for service, attemptID := range latest {
		if err := w.watchOne(ctx, service, attemptID, now); err != nil {
			w.logger().Error("deploywatch: watch failed", slog.String("service", service), slog.String("attempt_id", attemptID), slog.String("error", err.Error()))
		}
	}
}

type watchState struct {
	cutoverAt    time.Time
	started      bool
	done         bool
	softNotified bool
}

func readWatchState(events []store.DeployGateEvent) watchState {
	var st watchState
	for _, e := range events {
		switch {
		case e.Phase == deploysafety.PhaseCutover && e.Status == deploysafety.StatusPassed && st.cutoverAt.IsZero():
			st.cutoverAt = e.At
		case e.Phase == deploysafety.PhaseWatch && e.Status == deploysafety.StatusStarted:
			st.started = true
		case e.Phase == deploysafety.PhaseWatch && (e.Status == deploysafety.StatusPassed || e.Status == deploysafety.StatusFailed || e.Status == deploysafety.StatusStopped):
			st.done = true
		case e.Phase == deploysafety.PhaseWatch && e.Status == deploysafety.StatusInfo && e.Reason == deploysafety.ReasonWatchLatency:
			st.softNotified = true
		}
	}
	return st
}

func (w *Watcher) watchOne(ctx context.Context, service, attemptID string, now time.Time) error {
	events, err := w.Store.ListGateEvents(ctx, attemptID)
	if err != nil {
		return err
	}
	st := readWatchState(events)
	if st.done || st.cutoverAt.IsZero() {
		return nil
	}
	svc, err := w.Store.GetDesiredService(ctx, service)
	if errors.Is(err, store.ErrServiceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	attempt, err := w.Store.GetDeployAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	if !docker.SameImageRef(attempt.Image, svc.Image) || svc.Suspended {
		return nil
	}
	overrides, err := w.Store.GetDeploySafetySettings(ctx, service)
	if err != nil {
		return err
	}
	cfg := deploysafety.Resolve(w.Defaults, overrides)
	if cfg.WatchWindow <= 0 || cfg.WatchMode == deploysafety.WatchModeOff {
		return nil
	}
	if !st.started {
		w.record(ctx, attempt, deploysafety.PhaseWatch, deploysafety.StatusStarted, "",
			fmt.Sprintf("watching for crashloops, OOM kills and a 5xx rate over %.0f%% for %s", cfg.WatchErrorRate*100, cfg.WatchWindow))
	}

	obs, err := w.observe(ctx, svc, attempt.ID, cfg, st.cutoverAt, now)
	if err != nil {
		return err
	}
	verdict, reason, message := deploysafety.EvaluateWatch(obs, cfg)
	switch verdict {
	case deploysafety.VerdictPass:
		w.record(ctx, attempt, deploysafety.PhaseWatch, deploysafety.StatusPassed, "",
			fmt.Sprintf("no crashloop, OOM kill or 5xx spike in the %s after cutover", cfg.WatchWindow))
	case deploysafety.VerdictFail:
		w.trip(ctx, svc, attempt, cfg, reason, message)
	default:
		w.softSignal(ctx, svc, attempt, cfg, obs, st)
	}
	if verdict != deploysafety.VerdictPending {
		w.soft.forget(attempt.ID)
	}
	return nil
}

func (w *Watcher) observe(ctx context.Context, svc *store.DesiredService, attemptID string, cfg deploysafety.Effective, since, now time.Time) (deploysafety.WatchObservation, error) {
	obs := deploysafety.WatchObservation{SinceCutover: now.Sub(since)}
	resourceID := "service:" + svc.Name
	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = store.DefaultReplicas
	}
	names := make([]string, replicas)
	for i := range names {
		names[i] = application.ReplicaContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce, i)
	}
	// Restarts are counted per container of this release: a failed release
	// still held beside it restarts under the same service name.
	if w.Restarts != nil {
		obs.Restarts = w.Restarts.CountContainersSince(names, since)
	}
	if w.Inspectors != nil {
		inspector, err := w.Inspectors(svc.NodeID)
		if err == nil && inspector != nil {
			for _, name := range names {
				es, err := inspector.InspectExitState(ctx, name)
				if err != nil || es == nil {
					continue
				}
				obs.OOMKilled = obs.OOMKilled || es.OOMKilled
				obs.Exited = obs.Exited || !es.Running
			}
		}
	}
	// A request sample covers the interval before its timestamp, so the
	// first one after cutover can still hold the previous release's answers.
	from := since.Add(w.SampleInterval)
	if w.Metrics != nil && now.After(from) {
		var err error
		if obs.Requests, err = sumMetric(ctx, w.Metrics, resourceID, telemetry.MetricHTTPRequests, from, now); err != nil {
			return obs, err
		}
		if obs.Errors5xx, err = sumMetric(ctx, w.Metrics, resourceID, telemetry.MetricHTTPResponses5xx, from, now); err != nil {
			return obs, err
		}
		if cfg.WatchLatencyRatio > 0 && obs.Requests >= float64(cfg.WatchMinRequests) {
			w.observeLatency(ctx, svc.Name, attemptID, cfg, since, from, now, &obs)
		}
	}
	return obs, nil
}

func sumMetric(ctx context.Context, q telemetry.MetricsQuerier, resourceID, metric string, from, to time.Time) (float64, error) {
	samples, err := q.QueryMetrics(ctx, resourceID, metric, from, to)
	if err != nil {
		return 0, fmt.Errorf("query %s: %w", metric, err)
	}
	var total float64
	for _, s := range samples {
		total += s.Value
	}
	return total, nil
}

func (w *Watcher) record(ctx context.Context, attempt *store.DeployAttempt, phase, status, reason, message string) {
	if err := w.Store.RecordGateEvent(ctx, store.DeployGateEvent{
		DeployAttemptID: attempt.ID, ServiceName: attempt.ServiceName, Phase: phase, Status: status,
		Reason: reason, Message: message, At: w.clock(),
	}); err != nil {
		w.logger().Error("deploywatch: record gate event failed", slog.String("attempt_id", attempt.ID), slog.String("error", err.Error()))
	}
}

func (w *Watcher) trip(ctx context.Context, svc *store.DesiredService, attempt *store.DeployAttempt, cfg deploysafety.Effective, reason, message string) {
	w.record(ctx, attempt, deploysafety.PhaseWatch, deploysafety.StatusFailed, reason, message)
	// An operator who rolled back chose this release: record the crash and
	// keep the row as it is, so the choice stays visible whatever the
	// auto-rollback setting is.
	if w.operatorChose(ctx, attempt) {
		msg := "this release was chosen by an operator rollback, so it was not rolled back automatically"
		w.record(ctx, attempt, deploysafety.PhaseRollback, deploysafety.StatusInfo, deploysafety.ReasonOperatorRollback, msg)
		w.logger().Warn("deploywatch: operator rollback release tripped the watch", slog.String("service", svc.Name), slog.String("attempt_id", attempt.ID), slog.String("reason", reason))
		w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; %s", reason, message, msg))
		return
	}
	changed, err := w.Store.FailDeployAttemptWithReason(ctx, attempt.ID, reason, message)
	if err == nil && !changed {
		err = w.Store.SetDeployAttemptReason(ctx, attempt.ID, reason)
	}
	if err != nil {
		w.logger().Error("deploywatch: mark attempt failed", slog.String("attempt_id", attempt.ID), slog.String("error", err.Error()))
	}
	w.logger().Warn("deploywatch: post-deploy watch tripped", slog.String("service", svc.Name), slog.String("attempt_id", attempt.ID), slog.String("reason", reason), slog.String("detail", message))

	if !cfg.AutoRollback {
		w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; automatic rollback is off for this app", reason, message))
		return
	}
	// Never chain rollbacks: a known-good release failing too points at
	// something outside the app, and walking further back would hide it.
	if attempt.Source == store.DeployAttemptSourceAutoRollback {
		w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; this release is already an automatic rollback, leaving it for an operator", reason, message))
		return
	}
	target, pin, err := w.rollbackTarget(ctx, svc, attempt)
	if err != nil {
		w.logger().Error("deploywatch: find rollback target failed", slog.String("service", svc.Name), slog.String("error", err.Error()))
	}
	if target == nil {
		w.record(ctx, attempt, deploysafety.PhaseRollback, deploysafety.StatusFailed, deploysafety.ReasonNoRollbackTarget, "no earlier release is available to roll back to")
		w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; no earlier release to roll back to", reason, message))
		return
	}
	restored, ref, safe := deploy.RollbackDesired(*svc, *target, pin)
	rollbackDetail := fmt.Sprintf("%s: %s", reason, message)
	if !safe && !w.tagStillMatches(ctx, *target) {
		msg := fmt.Sprintf("deploy %s has no rollback pin or digest and tag %s no longer matches its content", target.ID, target.Image)
		w.record(ctx, attempt, deploysafety.PhaseRollback, deploysafety.StatusFailed, deploysafety.ReasonNoRollbackTarget, msg)
		w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; %s", reason, message, msg))
		return
	}
	trigger := deploy.ImageTrigger{
		Store: w.Store, Nudger: w.Nudger, Resolver: w.Resolver, Logger: w.logger(),
		Source: store.DeployAttemptSourceAutoRollback, Reason: deploysafety.ReasonAutoRolledBack,
	}
	// Content is already verified locally: a registry lookup could only
	// swap in a tag that moved upstream since.
	if !safe || deploy.LocalOnlyRef(pin, ref) {
		trigger.Resolver = nil
		restored.PullPolicy = ""
	}
	if _, err := trigger.Deploy(ctx, restored, ref); err != nil {
		w.record(ctx, attempt, deploysafety.PhaseRollback, deploysafety.StatusFailed, reason, "automatic rollback could not start: "+err.Error())
		w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; automatic rollback failed: %v", reason, message, err))
		return
	}
	w.record(ctx, attempt, deploysafety.PhaseRollback, deploysafety.StatusPassed, deploysafety.ReasonAutoRolledBack,
		fmt.Sprintf("rolled back to deploy %s (%s) with its config; it goes through the same readiness gate", target.ID, ref))
	w.audit(ctx, svc.Name, attempt.ID, target.ID, rollbackDetail)
	w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; rolled back automatically to deploy %s", reason, message, target.ID))
}

// operatorChose reports whether attempt is an explicit operator rollback.
func (w *Watcher) operatorChose(ctx context.Context, attempt *store.DeployAttempt) bool {
	if attempt.Source == store.DeployAttemptSourceAutoRollback {
		return false
	}
	attempts, err := w.Store.ListDeployAttempts(ctx, attempt.ServiceName)
	if err != nil {
		return false
	}
	for i := range attempts {
		if attempts[i].ID == attempt.ID {
			return changes.IsRollbackAttempt(*attempt, attempts[i+1:])
		}
	}
	return false
}

// rollbackTarget is the newest still-succeeded pinned release other than
// attempt, else the newest earlier succeeded deploy of a different image.
// Tripped releases are failed, so rollbacks never ping-pong.
func (w *Watcher) rollbackTarget(ctx context.Context, svc *store.DesiredService, attempt *store.DeployAttempt) (*store.DeployAttempt, *store.RollbackPin, error) {
	pins, err := w.Store.ListRollbackPins(ctx, svc.Name)
	if err != nil {
		return nil, nil, err
	}
	for i := range pins {
		pin := pins[i]
		if pin.DeployAttemptID == attempt.ID || (pin.Image == attempt.Image && pin.ImageID != "" && pin.ImageID == attempt.RunningImageID) {
			continue
		}
		target, err := w.Store.GetDeployAttempt(ctx, pin.DeployAttemptID)
		if err != nil || target.Status != store.DeployAttemptStatusSucceeded {
			continue
		}
		return target, &pin, nil
	}
	attempts, err := w.Store.ListDeployAttempts(ctx, svc.Name)
	if err != nil {
		return nil, nil, err
	}
	for i := range attempts {
		a := attempts[i]
		if a.ID == attempt.ID || a.Status != store.DeployAttemptStatusSucceeded || a.Image == "" || docker.SameImageRef(a.Image, attempt.Image) {
			continue
		}
		return &a, nil, nil
	}
	return nil, nil, nil
}

// tagStillMatches reports whether target's bare tag still resolves locally
// to the content target recorded.
func (w *Watcher) tagStillMatches(ctx context.Context, target store.DeployAttempt) bool {
	inspector, ok := w.Resolver.(docker.ImageInspector)
	if !ok || target.ImageDigest == "" {
		return false
	}
	id, err := inspector.InspectImageID(ctx, target.Image)
	return err == nil && id == target.ImageDigest
}

func (w *Watcher) notify(ctx context.Context, app, image, message string) {
	if w.Notify != nil {
		w.Notify(ctx, app, image, message)
	}
}
