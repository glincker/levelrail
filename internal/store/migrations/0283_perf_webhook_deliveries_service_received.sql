-- ListWebhookDeliveries filters service_name and orders by received_at
-- DESC; the single-column service_name index can't cover that sort, so
-- it filesorts as this unbounded, attacker-reachable table grows.
CREATE INDEX idx_webhook_deliveries_service_received ON webhook_deliveries(service_name, received_at DESC);
