-- A push subscription is one browser's Web Push registration (endpoint
-- plus the two subscribe-time keys the Push API hands back), the
-- delivery target for the "webpush" notification-channel kind
-- (internal/alerting). p256dh/auth are the browser's own public half of
-- its subscription keys, not secrets this control plane generates, so
-- unlike the VAPID private key (internal/secrets, service "webpush")
-- they're ordinary columns.
CREATE TABLE push_subscriptions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint     TEXT NOT NULL,
    p256dh       TEXT NOT NULL,
    auth         TEXT NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    last_used_at TEXT NULL
);

CREATE INDEX idx_push_subscriptions_user_id ON push_subscriptions(user_id);
CREATE UNIQUE INDEX ux_push_subscriptions_endpoint ON push_subscriptions(endpoint);
