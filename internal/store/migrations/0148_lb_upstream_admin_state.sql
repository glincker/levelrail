-- Operator-set admin state (draining or disabled) per load balancer upstream,
-- keyed by service and replica index (the same identity as the upstream ID).
-- No row means active; cascade drops rows with the service.
CREATE TABLE lb_upstream_admin_state (
    service_name TEXT    NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    replica      INTEGER NOT NULL CHECK (replica >= 0),
    admin_state  TEXT    NOT NULL CHECK (admin_state IN ('draining', 'disabled')),
    updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (service_name, replica)
);
