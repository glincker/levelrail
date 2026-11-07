package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// SaveDesiredService creates or fully replaces a service's desired state in
// one transaction, reconciling svc.Domains against service_domains. NodeID is
// never written here (see UpdateServiceNode) so redeploys keep placement.
// Returns *ErrDomainTaken (via errors.As) and writes nothing if a domain is
// claimed by a different service.
func (db *DB) SaveDesiredService(ctx context.Context, svc DesiredService) error {
	return db.saveDesiredService(ctx, svc, nil, false)
}

// SaveSuspendedDesiredService is SaveDesiredService that also marks the row
// suspended in the same transaction, so no reconcile pass can see it running.
func (db *DB) SaveSuspendedDesiredService(ctx context.Context, svc DesiredService) error {
	return db.saveDesiredService(ctx, svc, nil, true)
}

// saveDesiredService is SaveDesiredService with an optional hook run inside
// the same transaction before the write; a hook error aborts the save.
func (db *DB) saveDesiredService(ctx context.Context, svc DesiredService, inTx func(*sql.Tx) error, suspended bool) error {
	domainsJSON, err := json.Marshal(nonNilSlice(svc.Domains))
	if err != nil {
		return fmt.Errorf("store: marshal domains for service %q: %w", svc.Name, err)
	}
	envJSON, err := json.Marshal(nonNilMap(svc.Env))
	if err != nil {
		return fmt.Errorf("store: marshal env for service %q: %w", svc.Name, err)
	}
	commandJSON, err := json.Marshal(nonNilSlice(svc.Command))
	if err != nil {
		return fmt.Errorf("store: marshal command for service %q: %w", svc.Name, err)
	}
	dependsOnJSON, err := json.Marshal(nonNilSlice(svc.DependsOn))
	if err != nil {
		return fmt.Errorf("store: marshal depends_on for service %q: %w", svc.Name, err)
	}
	dependsOnHealthyJSON, err := json.Marshal(nonNilSlice(svc.DependsOnHealthy))
	if err != nil {
		return fmt.Errorf("store: marshal depends_on_healthy for service %q: %w", svc.Name, err)
	}
	entrypointJSON, err := json.Marshal(nonNilSlice(svc.Entrypoint))
	if err != nil {
		return fmt.Errorf("store: marshal entrypoint for service %q: %w", svc.Name, err)
	}
	secretEnvJSON, err := json.Marshal(nonNilSecretEnv(svc.SecretEnv))
	if err != nil {
		return fmt.Errorf("store: marshal secret_env for service %q: %w", svc.Name, err)
	}
	databaseEnv := svc.DatabaseEnv
	if databaseEnv == nil {
		databaseEnv = map[string]DatabaseEnvRef{}
	}
	databaseEnvJSON, err := json.Marshal(databaseEnv)
	if err != nil {
		return fmt.Errorf("store: marshal database_env for service %q: %w", svc.Name, err)
	}
	vaultEnv := svc.VaultEnv
	if vaultEnv == nil {
		vaultEnv = map[string]VaultEnvRef{}
	}
	vaultEnvJSON, err := json.Marshal(vaultEnv)
	if err != nil {
		return fmt.Errorf("store: marshal vault_env for service %q: %w", svc.Name, err)
	}
	resourcesJSON, err := json.Marshal(svc.Resources)
	if err != nil {
		return fmt.Errorf("store: marshal resources for service %q: %w", svc.Name, err)
	}
	healthJSON, err := json.Marshal(svc.Health)
	if err != nil {
		return fmt.Errorf("store: marshal health for service %q: %w", svc.Name, err)
	}
	hooksJSON, err := json.Marshal(svc.Hooks)
	if err != nil {
		return fmt.Errorf("store: marshal hooks for service %q: %w", svc.Name, err)
	}
	egressJSON, err := json.Marshal(svc.Egress)
	if err != nil {
		return fmt.Errorf("store: marshal egress_policy for service %q: %w", svc.Name, err)
	}
	tolerationsJSON, affinityJSON := svc.Tolerations, svc.Affinity
	labelsJSON, err := json.Marshal(nonNilMap(svc.Labels))
	if err != nil {
		return fmt.Errorf("store: marshal labels for service %q: %w", svc.Name, err)
	}
	volumes := svc.Volumes
	if volumes == nil {
		volumes = []ServiceVolume{}
	}
	volumesJSON, err := json.Marshal(volumes)
	if err != nil {
		return fmt.Errorf("store: marshal volumes for service %q: %w", svc.Name, err)
	}
	bindMounts := svc.BindMounts
	if bindMounts == nil {
		bindMounts = []ServiceBindMount{}
	}
	bindMountsJSON, err := json.Marshal(bindMounts)
	if err != nil {
		return fmt.Errorf("store: marshal bind_mounts for service %q: %w", svc.Name, err)
	}
	filesJSON, err := json.Marshal(nonNilFiles(svc.Files))
	if err != nil {
		return fmt.Errorf("store: marshal files for service %q: %w", svc.Name, err)
	}
	publishedPorts := make([]PublishedPort, 0, len(svc.PublishedPorts))
	for _, p := range svc.PublishedPorts {
		p.Protocol = p.EffectiveProtocol()
		publishedPorts = append(publishedPorts, p)
	}
	publishedPortsJSON, err := json.Marshal(publishedPorts)
	if err != nil {
		return fmt.Errorf("store: marshal published_ports for service %q: %w", svc.Name, err)
	}

	strategy := svc.Strategy
	if strategy == "" {
		strategy = DefaultDeployStrategy
	}
	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = DefaultReplicas
	}
	bindAddress := svc.BindAddress
	if bindAddress == "" {
		bindAddress = DefaultBindAddress
	}

	incarnation, err := newIncarnation()
	if err != nil {
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save desired service %q: begin transaction: %w", svc.Name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	if inTx != nil {
		if err := inTx(tx); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO desired_services (name, image, port, host_port, bind_address, domains, env, command, entrypoint, secret_env, env_dirty, database_env, vault_env, resources, health, hooks, egress_policy, node_id, strategy, replicas, labels, volumes, bind_mounts, registry_credential_id, project_id, environment_id, app_id, is_trial, template_id, pull_policy, image_id, image_id_ref, depends_on, depends_on_healthy, published_ports, files, incarnation, tolerations, affinity, next_cache, next_image_cache, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			image = excluded.image,
			port = excluded.port,
			host_port = excluded.host_port,
			bind_address = excluded.bind_address,
			domains = excluded.domains,
			env = excluded.env,
			command = excluded.command,
			entrypoint = excluded.entrypoint,
			secret_env = excluded.secret_env,
			env_dirty = excluded.env_dirty,
			database_env = excluded.database_env,
			vault_env = excluded.vault_env,
			resources = excluded.resources,
			health = excluded.health,
			hooks = excluded.hooks,
			egress_policy = excluded.egress_policy,
			strategy = excluded.strategy,
			replicas = excluded.replicas,
			labels = excluded.labels,
			volumes = excluded.volumes,
			bind_mounts = excluded.bind_mounts,
			registry_credential_id = excluded.registry_credential_id,
			pull_policy = excluded.pull_policy,
			image_id = excluded.image_id,
			image_id_ref = excluded.image_id_ref,
			depends_on = excluded.depends_on,
			depends_on_healthy = excluded.depends_on_healthy,
			published_ports = excluded.published_ports,
			tolerations = excluded.tolerations,
			affinity = excluded.affinity,
			next_cache = CASE WHEN excluded.next_cache <> '' THEN excluded.next_cache ELSE desired_services.next_cache END,
			next_image_cache = CASE WHEN excluded.next_image_cache <> '' THEN excluded.next_image_cache ELSE desired_services.next_image_cache END,
			files = CASE WHEN ? THEN excluded.files ELSE desired_services.files END,
			updated_at = excluded.updated_at
	`, svc.Name, svc.Image, svc.Port, hostPortToNull(svc.HostPort), bindAddress, string(domainsJSON), string(envJSON), string(commandJSON), string(entrypointJSON), string(secretEnvJSON), svc.EnvDirty, string(databaseEnvJSON), string(vaultEnvJSON), string(resourcesJSON), string(healthJSON), string(hooksJSON), string(egressJSON), strategy, replicas, string(labelsJSON), string(volumesJSON), string(bindMountsJSON), svc.RegistryCredentialID, sql.NullString{String: svc.AppID, Valid: svc.AppID != ""}, svc.IsTrial, svc.TemplateID, svc.PullPolicy, svc.ImageID, svc.ImageIDRef, string(dependsOnJSON), string(dependsOnHealthyJSON), string(publishedPortsJSON), string(filesJSON), incarnation, string(tolerationsJSON), string(affinityJSON), svc.NextCache, svc.NextImageCache, svc.Files != nil)
	if err != nil {
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}

	if err := claimServiceDomains(ctx, tx, svc.Name, svc.Domains); err != nil {
		// Wrapped with %w, not returned bare: still findable via
		// errors.As for a *ErrDomainTaken caller that wants the
		// specific domain/owner, per this method's error-wrapping
		// convention and the house rule that errors are wrapped with
		// context at every layer boundary.
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}

	if suspended {
		if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET suspended = 1 WHERE name = ?`, svc.Name); err != nil {
			return fmt.Errorf("store: save desired service %q: suspend: %w", svc.Name, err)
		}
	}
	if err := recordAppChange(ctx, tx, svc.Name); err != nil {
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save desired service %q: commit: %w", svc.Name, err)
	}
	return nil
}

// newIncarnation returns a random 128 bit id for one life of an app.
func newIncarnation() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate incarnation: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// EnsureServiceIncarnation gives a legacy row its id and returns the stored one. The conditional UPDATE keeps two racing passes on one value.
func (db *DB) EnsureServiceIncarnation(ctx context.Context, name string) (string, error) {
	id, err := newIncarnation()
	if err != nil {
		return "", fmt.Errorf("store: ensure incarnation for %q: %w", name, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE desired_services SET incarnation = ? WHERE name = ? AND incarnation = ''`, id, name); err != nil {
		return "", fmt.Errorf("store: ensure incarnation for %q: %w", name, err)
	}
	var got string
	if err := db.QueryRowContext(ctx, `SELECT incarnation FROM desired_services WHERE name = ?`, name).Scan(&got); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrServiceNotFound
		}
		return "", fmt.Errorf("store: ensure incarnation for %q: %w", name, err)
	}
	return got, nil
}

// claimServiceDomains makes serviceName's service_domains rows match
// domains, returning *ErrDomainTaken when another service or static site
// already claims one. Kept domains keep their row: per-domain settings
// (redirect, basic auth, TLS cert, WAF, maintenance, error pages) cascade
// from it, so re-inserting would drop them on every save.
func claimServiceDomains(ctx context.Context, tx *sql.Tx, serviceName string, domains []string) error {
	envDomains, err := heldEnvDomains(ctx, tx, serviceName)
	if err != nil {
		return err
	}
	domains = append(append([]string(nil), domains...), envDomains...)
	held, err := serviceDomainClaims(ctx, tx, serviceName)
	if err != nil {
		return err
	}
	claimed := make(map[string]bool, len(domains))
	for _, domain := range domains {
		if domain != "" {
			claimed[domain] = true
		}
	}
	for domain := range held {
		if claimed[domain] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM service_domains WHERE domain = ? AND service_name = ?`, domain, serviceName); err != nil {
			return fmt.Errorf("release domain %q: %w", domain, err)
		}
	}

	for _, domain := range domains {
		if domain == "" || held[domain] {
			continue
		}
		held[domain] = true

		owner, found, err := domainOwner(ctx, tx, domain)
		if err != nil {
			return fmt.Errorf("check domain %q availability: %w", domain, err)
		}
		if found {
			return &ErrDomainTaken{Domain: domain, Owner: owner}
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO service_domains (domain, service_name) VALUES (?, ?)
		`, domain, serviceName); err != nil {
			return fmt.Errorf("claim domain %q: %w", domain, err)
		}
	}
	return nil
}

func serviceDomainClaims(ctx context.Context, tx *sql.Tx, serviceName string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT domain FROM service_domains WHERE service_name = ?`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("list existing domain claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	held := make(map[string]bool)
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("scan existing domain claim: %w", err)
		}
		held[domain] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list existing domain claims: %w", err)
	}
	return held, nil
}
