-- Singleton row holding this control-plane instance's own persistent,
-- random identity, same "one authoritative row" shape as onboarding_state
-- (0067). No seed row here, unlike onboarding_state: the value is a random
-- ID minted in Go (store.GetOrCreateInstanceID), not a static default a
-- migration can express in SQL.
CREATE TABLE instance_identity (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    instance_id TEXT NOT NULL
);
