-- Singleton row tracking whether the first-run onboarding flow has been
-- completed or dismissed, same "one authoritative row" shape as
-- ingress_settings (0024_ingress_settings.sql). Instance-level, not
-- per-user: this control plane has exactly one admin user today.
CREATE TABLE onboarding_state (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    completed INTEGER NOT NULL DEFAULT 0
);

INSERT INTO onboarding_state (id, completed) VALUES (1, 0);
