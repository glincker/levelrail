package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// recordLegacyVaultRefusal stores one security event per app per hour, so a retry loop cannot flood the timeline.
func (c *Controller) recordLegacyVaultRefusal(envVar string) {
	slog.Warn("security: refused an unvetted legacy vault path outside the allowed prefixes", slog.String("service", c.serviceName), slog.String("event", store.AppEventLegacyVaultPathRefused))
	adder, ok := c.store.(appEventAdder)
	if !ok {
		return
	}
	now := time.Now().UnixNano()
	last := c.legacyVaultEventAt.Load()
	if last != 0 && now-last < int64(reservedOwnerEventEvery) {
		return
	}
	if !c.legacyVaultEventAt.CompareAndSwap(last, now) {
		return
	}
	ev := store.AppEvent{
		AppName: c.serviceName,
		Kind:    store.AppEventLegacyVaultPathRefused,
		Actor:   "reconciler",
		Title:   "Vault path refused: outside the allowed prefixes",
		Detail:  "Add the path's prefix to vault.legacy_allowed_prefixes or move the variable to a secret connection.",
		Keys:    []string{envVar},
	}
	if err := adder.AddAppEvent(context.Background(), ev); err != nil {
		slog.Warn("reconcile: record legacy vault refusal event failed", slog.String("service", c.serviceName), slog.String("error", err.Error()))
	}
}
