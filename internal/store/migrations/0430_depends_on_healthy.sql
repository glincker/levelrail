-- Dependencies that wait for the sibling's Ready condition, not just its
-- container start (compose depends_on condition: service_healthy).
ALTER TABLE desired_services ADD COLUMN depends_on_healthy TEXT NOT NULL DEFAULT '[]';
