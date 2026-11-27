-- Revision counter for compare-and-swap saves of the whole desired_services row.
-- The trigger bumps it on any write to a spec column, so a stale snapshot write is detectable.
ALTER TABLE desired_services ADD COLUMN rev INTEGER NOT NULL DEFAULT 0;

CREATE TRIGGER IF NOT EXISTS desired_services_rev_bump
AFTER UPDATE OF image, port, host_port, bind_address, domains, env, command, entrypoint, secret_env, env_dirty, database_env, vault_env, resources, health, hooks, egress_policy, strategy, replicas, labels, volumes, bind_mounts, registry_credential_id, pull_policy, image_id, image_id_ref, depends_on, depends_on_healthy, published_ports, tolerations, affinity, run_user, working_dir, runtime_options, granted_caps, internal_dns_names, files, next_cache, next_image_cache ON desired_services
BEGIN
    UPDATE desired_services SET rev = rev + 1 WHERE name = NEW.name;
END;
