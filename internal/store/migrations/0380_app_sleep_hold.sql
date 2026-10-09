-- Function mode: a request that wakes a sleeping app waits for it instead of
-- getting the waking-up page.
ALTER TABLE app_sleep ADD COLUMN hold_requests INTEGER NOT NULL DEFAULT 0;
