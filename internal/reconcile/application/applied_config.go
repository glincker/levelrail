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

// recordApplied stores the config a replica-0 container was just created
// with. Best effort: it must never fail a reconcile.
func (c *Controller) recordApplied(ctx context.Context, containerName string, desired *store.DesiredService) {
	if c.applied == nil || releaseOf(containerName) != containerName {
		return
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
	_ = c.applied.SaveAppliedConfig(ctx, store.AppliedConfig{
		ServiceName: c.serviceName,
		Release:     containerName,
		EnvHashes:   store.AppEnvHashes(c.serviceName, literal, secretValues),
		Fields:      fields,
		AppliedAt:   time.Now(),
	})
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
