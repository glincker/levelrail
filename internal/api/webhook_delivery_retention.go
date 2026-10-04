package api

import (
	"context"
	"log/slog"
	"time"
)

// defaultWebhookDeliveryRetention is how long a webhook_deliveries row
// survives before PurgeOldWebhookDeliveries removes it. Overridable via
// APP_WEBHOOK_DELIVERY_RETENTION_DAYS (cmd/levelrail/main.go's
// webhookDeliveryRetention, WithWebhookDeliveryRetention), the project's
// "no hardcoded thresholds" rule. 30 days, not audit_log's 90: a delivery
// row is debug/replay data for a webhook an operator is actively
// troubleshooting, not a compliance trail.
const defaultWebhookDeliveryRetention = 30 * 24 * time.Hour

func (rt *Router) effectiveWebhookDeliveryRetention() time.Duration {
	if rt.webhookDeliveryRetention > 0 {
		return rt.webhookDeliveryRetention
	}
	return defaultWebhookDeliveryRetention
}

// PurgeOldWebhookDeliveries deletes every webhook_deliveries row older
// than effectiveWebhookDeliveryRetention, returning how many rows were
// removed.
func (rt *Router) PurgeOldWebhookDeliveries(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-rt.effectiveWebhookDeliveryRetention())
	return rt.webhookDeliveries.DeleteWebhookDeliveriesOlderThan(ctx, cutoff)
}

// RunWebhookDeliverySweeper calls PurgeOldWebhookDeliveries on interval
// until ctx is done, the same ticker shape RunAuditLogSweeper already
// establishes: without it, webhook_deliveries would be the one inbound-
// request history table in this codebase that only ever grows.
func (rt *Router) RunWebhookDeliverySweeper(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if n, err := rt.PurgeOldWebhookDeliveries(ctx); err != nil {
				rt.logger.Warn("api: webhook delivery sweep tick failed", slog.String("error", err.Error()))
			} else if n > 0 {
				rt.logger.Info("api: swept old webhook deliveries", slog.Int64("deleted", n))
			}
		}
	}
}
