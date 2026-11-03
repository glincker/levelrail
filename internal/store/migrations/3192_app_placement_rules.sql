-- Per-app placement rules: tolerations of node taints and affinity terms,
-- stored as JSON. Empty means no rules.
ALTER TABLE desired_services ADD COLUMN tolerations TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN affinity TEXT NOT NULL DEFAULT '';
