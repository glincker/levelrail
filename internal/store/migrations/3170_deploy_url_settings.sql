-- Platform-wide deploy URL copies mode. NULL inherits the APP_DEPLOY_URL_COPIES
-- seed, then the built-in default (private).
CREATE TABLE deploy_url_settings (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    copies_mode  TEXT CHECK (copies_mode IS NULL OR copies_mode IN ('off', 'private', 'public'))
);

INSERT INTO deploy_url_settings (id, copies_mode) VALUES (1, NULL);
