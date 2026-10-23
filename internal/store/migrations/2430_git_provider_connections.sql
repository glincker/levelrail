-- Several GitLab, Bitbucket and Gitea connections per control plane: one row per authorized account, replacing the three single-row tables.
-- migrate: drops gitlab_app_connections.*
-- migrate: drops bitbucket_app_connections.*
-- migrate: drops gitea_app_connections.*
CREATE TABLE git_provider_connections (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL CHECK (provider IN ('gitlab', 'bitbucket', 'gitea')),
    label        TEXT NOT NULL,
    host         TEXT NOT NULL,
    instance_url TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    secrets_key  TEXT NOT NULL,
    account_id   TEXT NOT NULL DEFAULT '',
    account_name TEXT NOT NULL DEFAULT '',
    scopes       TEXT NOT NULL DEFAULT '',
    is_default   INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL
);
CREATE INDEX git_provider_connections_provider_host ON git_provider_connections (provider, host);

INSERT INTO git_provider_connections (id, provider, label, host, instance_url, client_id, secrets_key, scopes, is_default, created_at)
SELECT 'gc_' || lower(hex(randomblob(8))), 'gitlab', h, h, instance_url, client_id, 'gitlab-app/connection', 'api', 1, created_at
FROM (
    SELECT instance_url, client_id, created_at,
        lower(CASE WHEN instr(auth, ':') > 0 THEN substr(auth, 1, instr(auth, ':') - 1) ELSE auth END) AS h
    FROM (
        SELECT instance_url, client_id, created_at,
            CASE WHEN instr(rest, '/') > 0 THEN substr(rest, 1, instr(rest, '/') - 1) ELSE rest END AS auth
        FROM (SELECT instance_url, client_id, created_at, substr(instance_url, instr(instance_url, '://') + 3) AS rest FROM gitlab_app_connections)
    )
);

INSERT INTO git_provider_connections (id, provider, label, host, instance_url, client_id, secrets_key, scopes, is_default, created_at)
SELECT 'gc_' || lower(hex(randomblob(8))), 'gitea', h, h, instance_url, client_id, 'gitea-app/connection', '', 1, created_at
FROM (
    SELECT instance_url, client_id, created_at,
        lower(CASE WHEN instr(auth, ':') > 0 THEN substr(auth, 1, instr(auth, ':') - 1) ELSE auth END) AS h
    FROM (
        SELECT instance_url, client_id, created_at,
            CASE WHEN instr(rest, '/') > 0 THEN substr(rest, 1, instr(rest, '/') - 1) ELSE rest END AS auth
        FROM (SELECT instance_url, client_id, created_at, substr(instance_url, instr(instance_url, '://') + 3) AS rest FROM gitea_app_connections)
    )
);

INSERT INTO git_provider_connections (id, provider, label, host, instance_url, client_id, secrets_key, scopes, is_default, created_at)
SELECT 'gc_' || lower(hex(randomblob(8))), 'bitbucket', 'Bitbucket', 'bitbucket.org', 'https://bitbucket.org', oauth_key, 'bitbucket-app/connection', '', 1, created_at
FROM bitbucket_app_connections;

ALTER TABLE service_git_sources ADD COLUMN connection_id TEXT NOT NULL DEFAULT '';

WITH r AS (
    SELECT service_name, substr(repo_url, instr(repo_url, '://') + 3) AS rest FROM service_git_sources
), a AS (
    SELECT service_name, CASE WHEN instr(rest, '/') > 0 THEN substr(rest, 1, instr(rest, '/') - 1) ELSE rest END AS auth FROM r
), b AS (
    SELECT service_name, CASE WHEN instr(auth, '@') > 0 THEN substr(auth, instr(auth, '@') + 1) ELSE auth END AS hostport FROM a
), c AS (
    SELECT service_name, lower(CASE WHEN instr(hostport, ':') > 0 THEN substr(hostport, 1, instr(hostport, ':') - 1) ELSE hostport END) AS host FROM b
)
UPDATE service_git_sources SET connection_id = COALESCE((
    SELECT conn.id FROM git_provider_connections conn JOIN c ON c.host = conn.host
    WHERE c.service_name = service_git_sources.service_name LIMIT 1
), '');

UPDATE git_connection_status SET provider = (
    SELECT c.id FROM git_provider_connections c WHERE c.provider = git_connection_status.provider LIMIT 1
) WHERE provider IN ('gitlab', 'bitbucket', 'gitea')
  AND EXISTS (SELECT 1 FROM git_provider_connections c WHERE c.provider = git_connection_status.provider);
DELETE FROM git_connection_status WHERE provider IN ('gitlab', 'bitbucket', 'gitea');
DELETE FROM git_repo_list_cache WHERE provider IN ('gitlab', 'bitbucket', 'gitea');
-- Legacy gitlab, bitbucket and gitea connection tables are kept so the previous release can still read them; a later release drops them.
