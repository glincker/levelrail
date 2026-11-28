-- One monotonic counter per fence scope. A new desired_services row takes the next value, so a service
-- deleted and recreated never gets a token an agent has already seen. Restoring an older database can
-- still lower it: the agent then refuses until its fence-tokens.json is removed.
CREATE TABLE IF NOT EXISTS fence_epoch (
    key   TEXT PRIMARY KEY,
    value INTEGER NOT NULL
);

INSERT OR IGNORE INTO fence_epoch (key, value)
SELECT 'service', COALESCE(MAX(fence_token), 1) FROM desired_services;

CREATE TRIGGER IF NOT EXISTS desired_services_fence_seed
AFTER INSERT ON desired_services
BEGIN
    UPDATE fence_epoch SET value = value + 1 WHERE key = 'service';
    UPDATE desired_services SET fence_token = (SELECT value FROM fence_epoch WHERE key = 'service') WHERE name = NEW.name;
END;
