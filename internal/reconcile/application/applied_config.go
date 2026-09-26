package application

import (
	"context"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AppliedConfigRecorder persists what a container was created with and when
// the held previous release is due. *store.DB satisfies this structurally.
type AppliedConfigRecorder interface {
	SaveAppliedConfig(ctx context.Context, c store.AppliedConfig) error
	SetPreviousReleaseHeldUntil(ctx context.Context, name string, until time.Time) error
}

// WithAppliedConfigRecorder records each new container's creation-time
// config, so the API can report pending changes. nil records nothing.
func WithAppliedConfigRecorder(r AppliedConfigRecorder) Option {
	return func(ctrl *Controller) { ctrl.applied = r }
}

// appliedSnapshot is the config a replica-0 container was just created with,
// nil for other replicas or when nothing records it.
func (c *Controller) appliedSnapshot(ctx context.Context, containerName string, desired *store.DesiredService) *store.AppliedConfig {
	if c.applied == nil || releaseOf(containerName) != containerName {
		return nil
	}
	literal := desired.Env
	secretValues := map[string]string{}
	var secretKeys []string
	if c.secretResolver != nil {
		for _, ref := range desired.SecretEnv {
			exists, err := c.secretResolver.Exists(ctx, c.serviceName, ref.Name)
			if err != nil || !exists {
				continue
			}
			value, err := c.secretResolver.Resolve(ctx, c.serviceName, ref.Name)
			if err != nil {
				continue
			}
			secretValues[ref.Name] = value
			secretKeys = append(secretKeys, ref.Name)
		}
	}
	fields := store.AppliedFields(*desired)
	fields[store.AppliedFieldSecretKeys] = strings.Join(secretKeys, ",")
	return &store.AppliedConfig{
		ServiceName: c.serviceName,
		Release:     containerName,
		EnvHashes:   store.AppEnvHashes(c.serviceName, literal, secretValues),
		Fields:      fields,
	}
}

// holdApplied keeps snap until its container passes readiness, so a release
// that never went live is never reported as applied.
func (c *Controller) holdApplied(containerName string, snap *store.AppliedConfig) {
	if snap == nil {
		return
	}
	c.unconfirmedMu.Lock()
	defer c.unconfirmedMu.Unlock()
	if c.unconfirmed == nil {
		c.unconfirmed = map[string]*store.AppliedConfig{}
	}
	c.unconfirmed[containerName] = snap
}

// confirmApplied records containerName's held snapshot now that it is live.
// Best effort: it must never fail a reconcile.
func (c *Controller) confirmApplied(ctx context.Context, containerName string) {
	c.unconfirmedMu.Lock()
	snap := c.unconfirmed[containerName]
	delete(c.unconfirmed, containerName)
	c.unconfirmedMu.Unlock()
	if c.applied == nil || snap == nil {
		return
	}
	snap.AppliedAt = time.Now()
	_ = c.applied.SaveAppliedConfig(ctx, *snap)
}

// recordHold publishes when the held previous release is due, writing only
// on change.
func (c *Controller) recordHold(ctx context.Context, until time.Time) {
	if c.applied == nil || until.Equal(c.lastHeldUntil) {
		return
	}
	if err := c.applied.SetPreviousReleaseHeldUntil(ctx, c.serviceName, until); err == nil {
		c.lastHeldUntil = until
	}
}
