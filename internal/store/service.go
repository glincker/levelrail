package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ServiceResources caps a service's memory and CPU, in the same units
// internal/docker.Resources already uses (bytes, nano-CPUs), not
// app.yaml's human-friendly "512Mi"/0.5-cores strings. Translating those
// is the deploy pipeline's job (TASKS.md 1.4/1.5), not this package's;
// by the time a DesiredService exists, its units are already resolved.
type ServiceResources struct {
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`
}

// ServiceProbe is one readiness or liveness check.
type ServiceProbe struct {
	Path     string        `json:"path"`
	Interval time.Duration `json:"interval,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
	Failures int           `json:"failures,omitempty"`
}

// ServiceHealth holds a service's probe configuration.
type ServiceHealth struct {
	Readiness *ServiceProbe `json:"readiness,omitempty"`
	Liveness  *ServiceProbe `json:"liveness,omitempty"`
}

// DesiredService is what the application controller reconciles running
// containers against: an already-resolved image plus what it needs to
// run. See the migration comment in migrations/0002_desired_services.sql
// for why this is a distinct type from internal/spec.Service rather than
// reusing it directly.
type DesiredService struct {
	Name    string
	Image   string
	Port    int
	Domains []string
	Env     map[string]string
	// SecretEnv names env vars whose values live in secret storage
	// (internal/secrets, TASKS.md 1.7), resolved and decrypted by the
	// application controller immediately before container creation.
	// Never holds a value itself, only the key name, the same shape
	// app.yaml's { secret: true } already has: a name is not a secret,
	// only the value is.
	SecretEnv []string

	Resources *ServiceResources
	Health    *ServiceHealth

	// NodeID is which node (internal/store's own nodes table, TASKS.md
	// 3.1) this service should run on. Empty string is the explicit
	// "this control plane's own local node" value (TASKS.md 3.3's own
	// migration comment explains why that's not NULL or a foreign key),
	// the only value that existed before this field did, so an existing
	// single-node deployment's services keep running exactly where they
	// already were on upgrade.
	NodeID string
}

// SaveDesiredService creates or fully replaces the desired state for a
// named service. There's no partial update: a service's desired state is
// always written as a whole record, matching how it'll actually be
// produced (a deploy pipeline resolving a complete DesiredService from
// one app.yaml service block, not assembling one field at a time).
//
// One deliberate exception: NodeID is never written by this method,
// only ever by UpdateServiceNode below. internal/deploy.Pipeline calls
// this on every ordinary redeploy without ever setting NodeID (it has
// no opinion on placement), and svc.NodeID passed in here is always
// silently ignored, not just on an update but even on the very first
// INSERT: a new service starts on the local node ("") until an operator
// explicitly places it elsewhere via UpdateServiceNode, and a redeploy
// of an already-placed service must never un-assign it from wherever
// that placement decision put it.
func (db *DB) SaveDesiredService(ctx context.Context, svc DesiredService) error {
	domainsJSON, err := json.Marshal(nonNilSlice(svc.Domains))
	if err != nil {
		return fmt.Errorf("store: marshal domains for service %q: %w", svc.Name, err)
	}
	envJSON, err := json.Marshal(nonNilMap(svc.Env))
	if err != nil {
		return fmt.Errorf("store: marshal env for service %q: %w", svc.Name, err)
	}
	secretEnvJSON, err := json.Marshal(nonNilSlice(svc.SecretEnv))
	if err != nil {
		return fmt.Errorf("store: marshal secret_env for service %q: %w", svc.Name, err)
	}
	resourcesJSON, err := json.Marshal(svc.Resources)
	if err != nil {
		return fmt.Errorf("store: marshal resources for service %q: %w", svc.Name, err)
	}
	healthJSON, err := json.Marshal(svc.Health)
	if err != nil {
		return fmt.Errorf("store: marshal health for service %q: %w", svc.Name, err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO desired_services (name, image, port, domains, env, secret_env, resources, health, node_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			image = excluded.image,
			port = excluded.port,
			domains = excluded.domains,
			env = excluded.env,
			secret_env = excluded.secret_env,
			resources = excluded.resources,
			health = excluded.health,
			updated_at = excluded.updated_at
	`, svc.Name, svc.Image, svc.Port, string(domainsJSON), string(envJSON), string(secretEnvJSON), string(resourcesJSON), string(healthJSON))
	if err != nil {
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}
	return nil
}

// UpdateServiceNode reassigns svc to run on nodeID ("" for this control
// plane's own local node, TASKS.md 3.3's own migration comment), the
// only way node_id ever changes: SaveDesiredService's own doc comment
// explains why it's deliberately excluded from that method's
// full-record-replace semantics.
func (db *DB) UpdateServiceNode(ctx context.Context, name, nodeID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET node_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nodeID, name)
	if err != nil {
		return fmt.Errorf("store: update node for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update node for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// ErrServiceNotFound is returned by GetDesiredService when no service
// has that name.
var ErrServiceNotFound = errors.New("store: service not found")

// GetDesiredService returns the desired state for name, or
// ErrServiceNotFound if no such service has been saved.
func (db *DB) GetDesiredService(ctx context.Context, name string) (*DesiredService, error) {
	row := db.QueryRowContext(ctx, `
		SELECT name, image, port, domains, env, secret_env, resources, health, node_id
		FROM desired_services
		WHERE name = ?
	`, name)

	svc, err := scanDesiredService(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get desired service %q: %w", name, err)
	}
	return svc, nil
}

// ListDesiredServices returns every saved service, ordered by name.
func (db *DB) ListDesiredServices(ctx context.Context) ([]DesiredService, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, image, port, domains, env, secret_env, resources, health, node_id
		FROM desired_services
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredService
	for rows.Next() {
		svc, err := scanDesiredService(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired service row: %w", err)
		}
		out = append(out, *svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired service rows: %w", err)
	}
	return out, nil
}

// DeleteDesiredService removes a service's desired state, e.g. because
// the app was deleted through the HTTP API (TASKS.md 1.9). It returns
// ErrServiceNotFound if no such service exists, the same sentinel
// GetDesiredService uses, so callers handle "not found" one way
// regardless of which method produced it.
//
// Deleting desired state does not, by itself, stop or remove any
// container currently running for this service: as of this writing the
// application controller (internal/reconcile/application) treats a
// missing desired service as "nothing to do yet" (NoDesiredState), not
// "tear down what's running". Making delete actually converge to zero
// containers is a reconciler change, not a store one, and is a known gap
// left for that package.
func (db *DB) DeleteDesiredService(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM desired_services WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete desired service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// scanDesiredService reads the six-column shape both GetDesiredService
// and ListDesiredServices query, via either row.Scan or rows.Scan (same
// signature), so the decode-JSON-columns logic exists exactly once.
func scanDesiredService(scan func(dest ...any) error) (*DesiredService, error) {
	var (
		svc                                                        DesiredService
		domainsJSON, envJSON, secretEnvJSON, resourcesJSON, health string
	)
	if err := scan(&svc.Name, &svc.Image, &svc.Port, &domainsJSON, &envJSON, &secretEnvJSON, &resourcesJSON, &health, &svc.NodeID); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(domainsJSON), &svc.Domains); err != nil {
		return nil, fmt.Errorf("unmarshal domains: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &svc.Env); err != nil {
		return nil, fmt.Errorf("unmarshal env: %w", err)
	}
	if err := json.Unmarshal([]byte(secretEnvJSON), &svc.SecretEnv); err != nil {
		return nil, fmt.Errorf("unmarshal secret_env: %w", err)
	}
	if resourcesJSON != "null" {
		if err := json.Unmarshal([]byte(resourcesJSON), &svc.Resources); err != nil {
			return nil, fmt.Errorf("unmarshal resources: %w", err)
		}
	}
	if health != "null" {
		if err := json.Unmarshal([]byte(health), &svc.Health); err != nil {
			return nil, fmt.Errorf("unmarshal health: %w", err)
		}
	}

	return &svc, nil
}

// nonNilMap makes sure an unset Env marshals to "{}", not the JSON null
// a nil Go map produces, so GetDesiredService always round-trips into a
// non-nil (if possibly empty) map, one less nil-check for every caller.
func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// nonNilSlice is nonNilMap's counterpart for Domains: an unset slice
// marshals to "[]", not JSON null.
func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
