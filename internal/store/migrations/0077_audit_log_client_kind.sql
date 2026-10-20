-- client_kind normalizes the audit_log row's caller into one of "cli",
-- "dashboard", "mcp", or "api" (internal/api's clientKindFromUserAgent),
-- derived from the request's User-Agent header, closing the CloudTrail-parity
-- gap of distinguishing HOW an action was performed, not just who did it.
-- Existing rows predate this column and had no User-Agent recorded, so they
-- default to 'api', the same "unrecognized/unknown caller" bucket a future
-- unparseable User-Agent falls into.
ALTER TABLE audit_log ADD COLUMN client_kind TEXT NOT NULL DEFAULT 'api';
