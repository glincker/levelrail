-- Env var names an app keeps out of its preview environments.
CREATE TABLE preview_env_excludes (
    service_name TEXT NOT NULL,
    key TEXT NOT NULL,
    PRIMARY KEY (service_name, key)
);
