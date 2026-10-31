-- A non-empty hold keeps the reconciler from starting, creating or removing
-- the service's containers while an operation such as a mail restore runs.
ALTER TABLE desired_services ADD COLUMN maintenance_hold TEXT NOT NULL DEFAULT '';
