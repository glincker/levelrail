ALTER TABLE webhook_deliveries ADD COLUMN delivery_id TEXT NOT NULL DEFAULT '';
ALTER TABLE webhook_deliveries ADD COLUMN attempt_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_delivery_id ON webhook_deliveries (service_name, delivery_id) WHERE delivery_id != '';
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_attempt_id ON webhook_deliveries (attempt_id) WHERE attempt_id != '';
