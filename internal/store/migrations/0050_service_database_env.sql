-- Real app-to-database attachment (parity note: this is the baseline
-- capability an operator expects from any self-hosted deploy platform).
--
-- database_env: JSON object, envVarName -> {"database":"...","field":"..."},
-- the persisted form of app.yaml's own { from: "<database>.<field>" } env
-- var syntax (internal/spec.EnvVar.From). Same storage shape env/
-- secret_env/resources already use on this table, and the same "ordinary
-- desired state, SaveDesiredService writes it on every save" treatment
-- secret_env gets: it's a declaration derived fresh from app.yaml each
-- deploy, not an operator-set attachment (see database_attachment_*
-- below for that).
ALTER TABLE desired_services ADD COLUMN database_env TEXT NOT NULL DEFAULT '{}';

-- database_attachment_*: the UI/CLI-facing counterpart to database_env
-- above, for an app not hand-editing app.yaml (build.type: image created
-- directly through the API). Three columns, not a JSON object, mirroring
-- registry_credential_id's plain-column shape (migrations/0046) rather
-- than resources/health's JSON shape: exactly one attachment per app,
-- not an open-ended collection. Empty string on database_attachment_name
-- means "no attachment", the same node_id/registry_credential_id
-- empty-string-means-none convention this table already uses. Like
-- node_id/project_id/storage_target_id, SaveDesiredService never writes
-- these columns: only a dedicated setter does (store.DB's
-- UpdateServiceDatabaseAttachment), so an ordinary app.yaml redeploy can
-- never silently clear an attachment made through the UI/CLI.
ALTER TABLE desired_services ADD COLUMN database_attachment_name TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN database_attachment_env_var TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN database_attachment_field TEXT NOT NULL DEFAULT '';
