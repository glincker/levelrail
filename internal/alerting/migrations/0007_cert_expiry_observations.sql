-- Tracks, per (rule_id, domain), the certificate expiry state a
-- kind='cert_expiry' rule last observed: lets EvaluateCertExpiry
-- (internal/alerting/cert_expiry.go) tell "still stuck in the same
-- unrenewed episode" apart from "just entered a fresh expiry warning,"
-- the stronger "renewal appears stalled" signal.
CREATE TABLE cert_expiry_observations (
    rule_id             TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    domain              TEXT NOT NULL,
    status              TEXT NOT NULL,
    not_after           TEXT NOT NULL,
    episode_not_after   TEXT NOT NULL,
    episode_started_at  TEXT NOT NULL,
    observed_at         TEXT NOT NULL,
    PRIMARY KEY (rule_id, domain)
);
