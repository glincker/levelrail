package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Held request kinds, each replayed by its own ReleaseHandler.
const (
	HeldKindImage = "image"
	HeldKindGit   = "git"
)

// HeldRequest is what a held deploy needs to be replayed after its freeze.
type HeldRequest struct {
	Kind        string    `json:"kind"`
	Image       string    `json:"image,omitempty"`
	CheckoutRef string    `json:"checkout_ref,omitempty"`
	CommitLabel string    `json:"commit_label,omitempty"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Before      string    `json:"before,omitempty"`
	CommitAt    time.Time `json:"commit_at,omitempty"`
}

// HeldStore is the store surface holding and releasing needs.
type HeldStore interface {
	SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error
	ListHeldDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
	TransitionDeployAttempt(ctx context.Context, id, from, to, reason string) (bool, error)
}

// HoldDeploy records an automatic deploy parked by a freeze window.
func HoldDeploy(ctx context.Context, st HeldStore, serviceName, image, source string, req HeldRequest, status FreezeStatus) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("deploy: hold %q: marshal: %w", serviceName, err)
	}
	id, err := store.NewDeployAttemptID()
	if err != nil {
		return "", fmt.Errorf("deploy: hold %q: %w", serviceName, err)
	}
	reason := store.DeployReasonFrozen
	if !status.Until.IsZero() {
		reason += " until " + status.Until.UTC().Format(time.RFC3339)
	}
	if err := st.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: serviceName, Image: image, CommitSHA: req.CommitSHA,
		Source: source, Status: store.DeployAttemptStatusHeld, StartedAt: time.Now(),
		Reason: reason, HeldRequest: string(payload),
	}); err != nil {
		return "", fmt.Errorf("deploy: hold %q: %w", serviceName, err)
	}
	return id, nil
}

// ReleaseHandler replays one held request of its kind.
type ReleaseHandler func(ctx context.Context, attempt store.DeployAttempt, req HeldRequest) error

// Releaser replays held deploys once their service is no longer frozen.
// Only the newest held deploy per service runs; older ones are superseded.
type Releaser struct {
	Store    HeldStore
	Freeze   FreezeStore
	Handlers map[string]ReleaseHandler
	Logger   *slog.Logger
	Now      func() time.Time
}

// ReleaseDue releases every service whose freeze has ended, returning how
// many deploys were replayed.
func (r Releaser) ReleaseDue(ctx context.Context) (int, error) {
	held, err := r.Store.ListHeldDeployAttempts(ctx)
	if err != nil {
		return 0, fmt.Errorf("deploy: release held deploys: %w", err)
	}
	newest := map[string]store.DeployAttempt{}
	var order []string
	for _, a := range held {
		if prev, ok := newest[a.ServiceName]; ok {
			r.transition(ctx, prev.ID, store.DeployAttemptStatusSuperseded, store.DeployReasonSuperseded)
		} else {
			order = append(order, a.ServiceName)
		}
		newest[a.ServiceName] = a
	}
	released := 0
	for _, svc := range order {
		a := newest[svc]
		status, err := CheckFreeze(ctx, r.Freeze, svc, r.now())
		if err != nil {
			r.logger().Warn("deploy: release held: check freeze failed", slog.String("service", svc), slog.String("error", err.Error()))
			continue
		}
		if status.Frozen {
			continue
		}
		if r.release(ctx, a) {
			released++
		}
	}
	return released, nil
}

func (r Releaser) release(ctx context.Context, a store.DeployAttempt) bool {
	var req HeldRequest
	if err := json.Unmarshal([]byte(a.HeldRequest), &req); err != nil {
		r.transition(ctx, a.ID, store.DeployAttemptStatusFailed, "held request unreadable: "+err.Error())
		return false
	}
	handler, ok := r.Handlers[req.Kind]
	if !ok {
		r.transition(ctx, a.ID, store.DeployAttemptStatusFailed, "no release handler for "+req.Kind)
		return false
	}
	moved, err := r.Store.TransitionDeployAttempt(ctx, a.ID, store.DeployAttemptStatusHeld, store.DeployAttemptStatusRunning, "Released")
	if err != nil || !moved {
		return false
	}
	if err := handler(ctx, a, req); err != nil {
		r.logger().Warn("deploy: release held deploy failed", slog.String("attempt_id", a.ID), slog.String("service", a.ServiceName), slog.String("error", err.Error()))
		r.finish(ctx, a.ID, store.DeployAttemptStatusFailed, "Released: "+err.Error())
		return true
	}
	r.finish(ctx, a.ID, store.DeployAttemptStatusSucceeded, "Released")
	return true
}

func (r Releaser) transition(ctx context.Context, id, to, reason string) {
	if _, err := r.Store.TransitionDeployAttempt(ctx, id, store.DeployAttemptStatusHeld, to, reason); err != nil {
		r.logger().Warn("deploy: transition held deploy failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
	}
}

func (r Releaser) finish(ctx context.Context, id, to, reason string) {
	if _, err := r.Store.TransitionDeployAttempt(ctx, id, store.DeployAttemptStatusRunning, to, reason); err != nil {
		r.logger().Warn("deploy: finish released deploy failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
	}
}

// Run calls ReleaseDue every interval until ctx ends.
func (r Releaser) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := r.ReleaseDue(ctx); err != nil {
			r.logger().Warn("deploy: release held deploys failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r Releaser) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r Releaser) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
