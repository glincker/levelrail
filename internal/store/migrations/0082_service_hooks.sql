-- Pre/post-deploy hook commands (internal/spec.Hooks), stored as JSON the
-- same way desired_services.health/resources already are (0002): a nil
-- store.DesiredService.Hooks marshals to the JSON literal null, so an
-- existing row picks that up for free as "no hooks configured" after this
-- migration runs.
ALTER TABLE desired_services ADD COLUMN hooks TEXT NOT NULL DEFAULT 'null';
