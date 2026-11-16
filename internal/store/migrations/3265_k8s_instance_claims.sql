-- A stable per-install id for ownership labels, and short-lived claims that serialise deploys.
CREATE TABLE IF NOT EXISTS k8s_instance (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    id        TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS k8s_deploy_claims (
    cluster_name TEXT NOT NULL,
    app          TEXT NOT NULL,
    claimed_at   TEXT NOT NULL,
    PRIMARY KEY (cluster_name, app)
);
