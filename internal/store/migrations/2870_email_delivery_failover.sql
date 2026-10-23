-- Which provider handled a send, and which one it failed over from (empty when it did not).
ALTER TABLE email_deliveries ADD COLUMN provider_id TEXT NOT NULL DEFAULT '';
ALTER TABLE email_deliveries ADD COLUMN failover_from TEXT NOT NULL DEFAULT '';
