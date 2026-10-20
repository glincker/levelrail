-- An app.yaml-style databases: map, mirroring services_spec
-- (0063_git_source_services_spec.sql): persisted so a pull request
-- webhook can see which databases the app declares, in particular which
-- ones opted into ephemeralInPreviews, without re-fetching and parsing
-- app.yaml from the repo on every push.
ALTER TABLE service_git_sources ADD COLUMN databases_spec TEXT NOT NULL DEFAULT '{}';
