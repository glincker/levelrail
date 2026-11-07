-- One row per superseded Next.js release still inside its old asset grace window, so a restart keeps the window.
CREATE TABLE next_asset_grace (
    service_name  TEXT NOT NULL,
    deployment_id TEXT NOT NULL,
    image         TEXT NOT NULL DEFAULT '',
    until         TEXT NOT NULL,
    PRIMARY KEY (service_name, deployment_id)
);
