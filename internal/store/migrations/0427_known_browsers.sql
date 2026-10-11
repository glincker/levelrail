-- Browsers an account has signed in from, for the new-browser sign-in alert.
-- fingerprint is a SHA-256 over the browser label and the network prefix;
-- the raw user agent and address are not stored here.
CREATE TABLE IF NOT EXISTS known_browsers (
    user_id       TEXT NOT NULL,
    fingerprint   TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_known_browsers_seen ON known_browsers (last_seen_at);
