-- Per-service load balancer config (internal/loadbalancer.Config), stored as
-- JSON. Presence of a row means the service's route balances across all
-- replica upstreams; cascade drops it with the service.
CREATE TABLE service_load_balancers (
    service_name TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    config       TEXT NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
