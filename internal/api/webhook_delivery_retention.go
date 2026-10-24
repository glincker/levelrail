package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// defaultWebhookDeliveryRetention is how long a webhook_deliveries row
// survives before PurgeOldWebhookDeliveries removes it. Overridable via
// APP_WEBHOOK_DELIVERY_RETENTION_DAYS (cmd/levelrail/main.go's
// webhookDeliveryRetention, WithWebhookDeliveryRetention), the project's
// "no hardcoded thresholds" rule. 30 days, not audit_log's 90: a delivery
// row is debug/replay data for a webhook an operator is actively
// troubleshooting, not a compliance trail.
const defaultWebhookDeliveryRetention = 30 * 24 * time.Hour

// effectiveWebhookDeliveryRetention is zero when the operator keeps deliveries forever.
func (rt *Router) effectiveWebhookDeliveryRetention(ctx context.Context) time.Duration {
	if rt.webhookDeliveryRetention > 0 {
		return rt.webhookDeliveryRetention
	}
	return time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyWebhookDeliveryDays)) * 24 * time.Hour
}

// PurgeOldWebhookDeliveries deletes every webhook_deliveries row older
// than effectiveWebhookDeliveryRetention, returning how many rows were
// removed.
func (rt *Router) PurgeOldWebhookDeliveries(ctx context.Context) (int64, error) {
	window := rt.effectiveWebhookDeliveryRetention(ctx)
	if window == 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-window)
	return rt.webhookDeliveries.DeleteWebhookDeliveriesOlderThan(ctx, cutoff)
}

// RunWebhookDeliverySweeper calls PurgeOldWebhookDeliveries on interval
// until ctx is done, the same ticker shape RunAuditLogSweeper already
// establishes: without it, webhook_deliveries would be the one inbound-
// request history table in this codebase that only ever grows.
func (rt *Router) RunWebhookDeliverySweeper(ctx context.Context, interval time.Duration) error {
	return footprint.Every(ctx, "webhook delivery sweep", interval, rt.logger, func(ctx context.Context) error {
		n, err := rt.PurgeOldWebhookDeliveries(ctx)
		if err == nil && n > 0 {
			rt.logger.Info("api: swept old webhook deliveries", slog.Int64("deleted", n))
		}
		return err
	})
}
