-- On-demand model residency: a model can stop its engine after an idle period and
-- wake on the first gateway request. residency_state is owned by the reconciler.
ALTER TABLE models ADD COLUMN residency TEXT NOT NULL DEFAULT 'always';
ALTER TABLE models ADD COLUMN idle_ttl_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN last_active_at TEXT NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN residency_state TEXT NOT NULL DEFAULT 'awake';
