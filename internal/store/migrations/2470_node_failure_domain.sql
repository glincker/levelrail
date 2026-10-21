-- Provider and zone labels for a node, next to region (0252), so placement can spread replicas across failure domains.
ALTER TABLE nodes ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN zone TEXT NOT NULL DEFAULT '';
