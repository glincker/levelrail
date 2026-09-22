-- Single-row Gitea OAuth Application connection, the Gitea counterpart
-- of gitlab_app_connections (migrations/0044): instance_url/client_id
-- are not secret and live here as plain columns; client_secret and the
-- OAuth access_token/refresh_token/token_expires_at go through
-- internal/secrets.Manager instead, keyed by store.GiteaAppSecretsKey()
-- (internal/store/gitea_app.go).
CREATE TABLE gitea_app_connections (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    instance_url TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
