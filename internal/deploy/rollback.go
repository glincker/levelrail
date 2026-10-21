package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageDeployStore is the narrow store surface an image deploy trigger
// needs. *store.DB satisfies this structurally.
type ImageDeployStore interface {
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
	SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error
	FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error
}

// Sequencer hands out per-service trigger order.
type Sequencer interface {
	NextDeploySequence(ctx context.Context, serviceName string) (int64, error)
}

// ReconcileNudger wakes the reconcile loop after a desired-state write.
type ReconcileNudger interface {
	Nudge()
}

// ImageTrigger points a service at an image with no build step: manual
// redeploys and rollbacks, pipeline deploy steps and crashloop rollback.
// Every optional field degrades to the pre-digest behavior when nil.
type ImageTrigger struct {
	Store    ImageDeployStore
	Nudger   ReconcileNudger
	Resolver docker.ImageResolver
	// Ordered and Sequencer together enable the stale-deploy guard.
	Ordered   OrderedStore
	Sequencer Sequencer
	Logger    *slog.Logger
	Source    string
	// RequireFresh fails instead of deploying a cached image (deploy --pull).
	RequireFresh bool
	// Automatic subjects the deploy to the stale guard; manual ones are exempt.
	Automatic bool
	// Auth authenticates digest resolution against a private registry.
	Auth *docker.RegistryAuth
	// Reason is recorded on the attempt, e.g. a freeze override.
	Reason string
	// Locks, when set, holds the deploy instead of making it live while the
	// app's publish lock is on. Set it on automatic triggers only.
	Locks PublishLocks
}

// Deploy resolves image, writes it as existing's desired image, records a
// deploy attempt carrying the digest, and nudges the reconciler.
func (t ImageTrigger) Deploy(ctx context.Context, existing store.DesiredService, image string) (store.DesiredService, error) {
	logger := t.Logger
	if logger == nil {
		logger = slog.Default()
	}
	tr := deploytrace.NewBuffered()
	resolution, err := ResolveImageTraced(ctx, tr, t.Resolver, image, t.Auth, t.RequireFresh, existing.Image)
	if err != nil {
		return store.DesiredService{}, fmt.Errorf("deploy: trigger image deploy for %q: %w", existing.Name, err)
	}

	updated := existing
	resolution.apply(&updated)
	updated.EnvDirty = false

	heldID, err := t.holdIfLocked(ctx, updated)
	if err != nil {
		return store.DesiredService{}, fmt.Errorf("deploy: trigger image deploy for %q: %w", existing.Name, err)
	}

	var seq int64
	if t.Sequencer != nil && t.Ordered != nil {
		if seq, err = t.Sequencer.NextDeploySequence(ctx, existing.Name); err != nil {
			return store.DesiredService{}, fmt.Errorf("deploy: trigger image deploy for %q: %w", existing.Name, err)
		}
	}
	switch {
	case heldID != "":
	case t.Sequencer != nil && t.Ordered != nil:
		err = t.Ordered.SaveDesiredServiceOrdered(ctx, updated, store.DeployOrder{Sequence: seq, Automatic: t.Automatic}, CheckOrder)
	default:
		err = t.Store.SaveDesiredService(ctx, updated)
	}
	if err != nil {
		return store.DesiredService{}, fmt.Errorf("deploy: trigger image deploy for %q: %w", existing.Name, err)
	}

	source := t.Source
	if source == "" {
		source = store.DeployAttemptSourceImage
	}
	deploytrace.ConfigStep(ctx, tr, updated)
	if id := recordImageDeployAttempt(ctx, t.Store, updated, resolution, seq, source, t.Reason, heldID, logger); id != "" {
		if sink, ok := t.Store.(deploytrace.Sink); ok {
			tr.Flush(ctx, sink, id, existing.Name)
		}
	}

	if t.Nudger != nil && heldID == "" {
		t.Nudger.Nudge()
	}
	return updated, nil
}

// TriggerImageDeploy is ImageTrigger without digest resolution or ordering,
// for callers deploying an image already taken from deploy history.
func TriggerImageDeploy(ctx context.Context, st ImageDeployStore, nudger ReconcileNudger, existing store.DesiredService, image, source string, logger *slog.Logger) (store.DesiredService, error) {
	return ImageTrigger{Store: st, Nudger: nudger, Logger: logger, Source: source}.Deploy(ctx, existing, image)
}

// recordImageDeployAttempt saves and immediately finishes an attempt for a
// trigger with no build. Failures are logged: desired state already landed.
func recordImageDeployAttempt(ctx context.Context, st ImageDeployStore, svc store.DesiredService, res ImageResolution, seq int64, source, reason, id string, logger *slog.Logger) string {
	if id == "" {
		var err error
		if id, err = store.NewDeployAttemptID(); err != nil {
			logger.Error("deploy: record deploy attempt: mint id failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
			return ""
		}
	}
	now := time.Now()
	if err := st.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: svc.Name, Image: svc.Image,
		Source: source,
		Status: store.DeployAttemptStatusRunning, StartedAt: now,
		Snapshot:    store.NewDeployAttemptSnapshot(svc),
		ImageDigest: res.Digest, DigestReason: res.Reason, Sequence: seq, Reason: reason,
	}); err != nil {
		logger.Error("deploy: record deploy attempt: save failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return ""
	}
	if res.Note != "" {
		logger.Warn("deploy: image resolved without the registry", slog.String("attempt_id", id), slog.String("reason", res.Reason), slog.String("registry_error", res.Note))
	}
	if err := st.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		logger.Error("deploy: record deploy attempt: finish failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
	}
	return id
}

// PreviousKnownGoodImage returns the newest succeeded attempt's image that
// differs from currentImage, scanning attempts newest first. ok is false
// when there is nothing to roll back to.
func PreviousKnownGoodImage(attempts []store.DeployAttempt, currentImage string) (image string, ok bool) {
	for _, a := range attempts {
		if a.Status != store.DeployAttemptStatusSucceeded {
			continue
		}
		if a.Image == "" || a.Image == currentImage {
			continue
		}
		return a.Image, true
	}
	return "", false
}

// holdIfLocked saves updated as a held build and returns its id when the
// app is locked. A lock lookup failure fails closed.
func (t ImageTrigger) holdIfLocked(ctx context.Context, updated store.DesiredService) (string, error) {
	if t.Locks == nil {
		return "", nil
	}
	lock, err := t.Locks.GetPublishLock(ctx, updated.Name)
	if err != nil {
		return "", fmt.Errorf("check publish lock: %w", err)
	}
	if lock == nil {
		return "", nil
	}
	id, err := store.NewDeployAttemptID()
	if err != nil {
		return "", fmt.Errorf("mint held build id: %w", err)
	}
	if err := t.Locks.SaveHeldBuild(ctx, id, updated, time.Now()); err != nil {
		return "", fmt.Errorf("save held build: %w", err)
	}
	return id, nil
}
