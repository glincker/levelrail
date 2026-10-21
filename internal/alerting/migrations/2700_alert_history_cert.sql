-- Structured cert expiry data (JSON) beside the human detail text; empty for older rows and other rule kinds.
ALTER TABLE alert_history ADD COLUMN cert_json TEXT NOT NULL DEFAULT '';
