package api

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/store"
)

const (
	envCapRecheckPollSeconds     = "APP_PROVISION_CAPABILITIES_RECHECK_POLL_SECONDS"
	defaultCapRecheckPollSeconds = 60
)

// StartProviderCapabilityRecheck re-probes connected providers in the
// background so a rotated or narrowed token shows up without a click. It
// stops with ctx and a second call is a no-op.
func (rt *Router) StartProviderCapabilityRecheck(ctx context.Context) {
	if !rt.providerCaps.recheckActive.CompareAndSwap(false, true) {
		return
	}
	poll := time.Duration(envIntDefault(envCapRecheckPollSeconds, defaultCapRecheckPollSeconds)) * time.Second
	if poll <= 0 {
		poll = defaultCapRecheckPollSeconds * time.Second
	}
	go func() {
		defer rt.providerCaps.recheckActive.Store(false)
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rt.recheckProviderCapabilities(ctx)
			}
		}
	}()
}

// recheckProviderCapabilities probes each connected provider whose last
// result is older than the recheck interval. The interval is read every
// pass so changing the setting, or 0 to disable, applies live.
func (rt *Router) recheckProviderCapabilities(ctx context.Context) {
	iv := rt.capRecheckInterval(ctx)
	if iv <= 0 || rt.nodeProviderSecrets == nil {
		return
	}
	for _, name := range nodeProviderNames {
		if ctx.Err() != nil {
			return
		}
		exists, err := rt.nodeProviderSecrets.Exists(ctx, store.NodeProviderSecretsKey(name), store.NodeProviderTokenEnvKey)
		if err != nil || !exists {
			continue
		}
		rt.recheckOneProvider(ctx, name, iv)
	}
}

func (rt *Router) recheckOneProvider(ctx context.Context, provider string, iv time.Duration) {
	e := rt.providerCaps.entry(provider)
	// A probe already running (a user refresh) is the recheck, so skip.
	if !e.probeMu.TryLock() {
		return
	}
	defer e.probeMu.Unlock()
	if e.result != nil && time.Since(e.result.CheckedAt) < iv {
		return
	}
	p, ok, _, _ := rt.resolveNodeProvisioner(ctx, provider)
	if !ok {
		return
	}
	if _, can := p.(provision.CapabilityProber); !can {
		return
	}
	rt.providerCapabilitiesLocked(ctx, provider, p, e, capForceProbe)
}
