-- Deploy strategy and replica count. internal/spec's Service.Strategy/
-- Replicas fields, and their EffectiveStrategy()/EffectiveReplicas()
-- defaulting helpers, have existed since app.yaml's schema first
-- documented them; nothing in internal/store or the application
-- controller has ever had anywhere to put the resolved values until
-- now, so every deploy has been an implicit single-container image
-- swap regardless of what a service's app.yaml actually declared.
--
-- Both columns default to the same values EffectiveStrategy()/
-- EffectiveReplicas() already return for an unset field ('blue-green',
-- 1), so every pre-existing row reads exactly as it already behaves
-- today on upgrade: one container, create-alongside-then-remove-old.
-- store.SaveDesiredService additionally defends against an explicit
-- empty-string/zero write (not just an omitted column, which is the
-- only case a SQL DEFAULT actually covers) coercing to the same two
-- values in Go, so no caller of this package, present or future, can
-- persist an ambiguous or invalid state by omission.
ALTER TABLE desired_services ADD COLUMN strategy TEXT NOT NULL DEFAULT 'blue-green';
ALTER TABLE desired_services ADD COLUMN replicas INTEGER NOT NULL DEFAULT 1;
