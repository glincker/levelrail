package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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

	// Labels are arbitrary operator-supplied Docker labels applied to the
	// service's container at create time (internal/spec.Service.Labels'
	// storage home, migrations/0027_service_labels.sql). Already
	// validated by the time a DesiredService exists: internal/spec.
	// ValidateLabels runs at every input boundary (app.yaml parsing,
	// the HTTP API's validateAppResource), so this package and
	// internal/docker both trust it's collision-free with this
	// platform's own reserved label namespace by the time it gets here,
	// the same "resolve/validate once, store the resolved form" shape
	// Resources/Health already follow.
	Labels map[string]string

	// NodeID is which node (internal/store's own nodes table, TASKS.md
	// 3.1) this service should run on. Empty string is the explicit
	// "this control plane's own local node" value (TASKS.md 3.3's own
	// migration comment explains why that's not NULL or a foreign key),
	// the only value that existed before this field did, so an existing
	// single-node deployment's services keep running exactly where they
	// already were on upgrade.
	NodeID string

	// Strategy and Replicas are always the *resolved* (never empty/zero)
	// values: internal/spec.Service.EffectiveStrategy()/
	// EffectiveReplicas() already define what "unset in app.yaml" means,
	// and this package stores that already-resolved decision rather than
	// re-deriving it on every read, the same "resolve once, store the
	// resolved form" shape Resources/Health already follow (app.yaml's
	// human units are converted before a DesiredService exists at all).
	// SaveDesiredService independently defends against an empty
	// Strategy/zero Replicas from any caller, so this field is never
	// ambiguous regardless of who constructs the struct.
	Strategy string
	Replicas int

	// RestartNonce is opaque and only ever compared for equality, never
	// interpreted: internal/reconcile/application.ContainerName folds it
	// into the container name hash (only when non-empty, so a service
	// that has never been restarted keeps the exact container name it
	// always has). Changing it is therefore indistinguishable, from the
	// reconciler's level-triggered point of view, from an image change:
	// the existing blue-green/recreate cutover logic already handles it
	// correctly with no new branch. Like NodeID, this is deliberately
	// excluded from SaveDesiredService's full-record-replace semantics;
	// only RestartService writes it, see that method's own doc comment.
	RestartNonce string

	// ProjectID is which store.Project (migrations/0022_projects.sql)
	// this service is organizationally grouped under; empty string is
	// "no project", a real, permanent, equally-valid state, not merely
	// "unset" (see that migration's own comment on why the column is
	// genuinely nullable rather than reusing NodeID's NOT NULL DEFAULT
	// '' convention). Like NodeID and RestartNonce, SaveDesiredService
	// never writes this field, on either an INSERT or an UPDATE: only
	// UpdateServiceProject does, see that method's own doc comment for
	// why, and internal/api/apps.go's handleCreateApp for how a create-
	// time project choice still reaches it without this method's
	// full-replace semantics being allowed to silently move an
	// already-placed service between projects.
	ProjectID string
}

// DefaultDeployStrategy and DefaultReplicas mirror internal/spec's
// StrategyBlueGreen/DefaultReplicas exactly (same values), kept as this
// package's own constants rather than importing internal/spec: this
// package's own doc comment already establishes that DesiredService is
// deliberately independent of spec.Service, and every other cross-
// package enum in this file (NodeStatus, and so on) follows the same
// "define locally, don't import a higher-level package's vocabulary"
// convention.
const (
	DefaultDeployStrategy = "blue-green"
	DefaultReplicas       = 1
)

// ErrDomainTaken is returned by SaveDesiredService when one of svc's
// Domains is already claimed by a different service. Enforced here, not
// only by internal/spec's Validate() (which only ever sees one app.yaml
// at a time and so cannot see a domain already claimed by an earlier,
// separate deploy), because internal/reconcile/ingress's controller
// builds one Caddy route per domain from every desired service in a
// single reconcile pass (TASKS.md 3.6): two services claiming the same
// host would silently produce two routes matching the same Host header,
// with whichever sorted last winning inside Caddy's own matcher
// evaluation and silently shadowing the other. See that controller's
// package doc comment and migrations/0011_service_domains.sql.
type ErrDomainTaken struct {
	Domain string
	Owner  string
}

func (e *ErrDomainTaken) Error() string {
	return fmt.Sprintf("store: domain %q is already in use by service %q", e.Domain, e.Owner)
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
//
// The whole write, including reconciling svc.Domains against the
// service_domains table (0011), happens in one transaction: either the
// service's desired state and its domain claims both land, or neither
// does. Returns *ErrDomainTaken (via errors.As) if any domain in
// svc.Domains is already claimed by a different service; the caller's
// entire desired-state write is rejected in that case, not partially
// applied with some domains silently dropped.
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
	labelsJSON, err := json.Marshal(nonNilMap(svc.Labels))
	if err != nil {
		return fmt.Errorf("store: marshal labels for service %q: %w", svc.Name, err)
	}

	strategy := svc.Strategy
	if strategy == "" {
		strategy = DefaultDeployStrategy
	}
	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = DefaultReplicas
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save desired service %q: begin transaction: %w", svc.Name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO desired_services (name, image, port, domains, env, secret_env, resources, health, node_id, strategy, replicas, labels, project_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, NULL, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			image = excluded.image,
			port = excluded.port,
			domains = excluded.domains,
			env = excluded.env,
			secret_env = excluded.secret_env,
			resources = excluded.resources,
			health = excluded.health,
			strategy = excluded.strategy,
			replicas = excluded.replicas,
			labels = excluded.labels,
			updated_at = excluded.updated_at
	`, svc.Name, svc.Image, svc.Port, string(domainsJSON), string(envJSON), string(secretEnvJSON), string(resourcesJSON), string(healthJSON), strategy, replicas, string(labelsJSON))
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save desired service %q: commit: %w", svc.Name, err)
	}
	return nil
}

// claimServiceDomains replaces serviceName's rows in service_domains
// with exactly domains: every domain serviceName previously claimed but
// no longer declares is released, and every domain it now declares is
// claimed, unless a different service or static site already claims it
// (domainOwner, migrations/0015, checks both service_domains and
// static_site_domains), in which case this returns *ErrDomainTaken and
// the caller's whole transaction rolls back (claimServiceDomains never
// partially claims a service's domain list). Duplicate entries within
// domains are claimed once, not reported as a conflict against
// themselves.
func claimServiceDomains(ctx context.Context, tx *sql.Tx, serviceName string, domains []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_domains WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("clear existing domain claims: %w", err)
	}

	claimed := make(map[string]bool, len(domains))
	for _, domain := range domains {
		if domain == "" || claimed[domain] {
			continue
		}
		claimed[domain] = true

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

// UpdateServiceProject reassigns svc to project projectID ("" for "no
// project", the same real-and-permanent sentinel DesiredService.
// ProjectID's own doc comment describes), the only way project_id ever
// changes: SaveDesiredService's own doc comment explains why it's
// deliberately excluded from that method's full-record-replace
// semantics, the same reasoning UpdateServiceNode already establishes
// for NodeID. An empty projectID is written as SQL NULL, not the empty
// string node_id uses, because unlike node_id (NOT NULL DEFAULT ''),
// this column is genuinely nullable (migrations/0022_projects.sql's own
// comment on why).
func (db *DB) UpdateServiceProject(ctx context.Context, name, projectID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET project_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, sql.NullString{String: projectID, Valid: projectID != ""}, name)
	if err != nil {
		return fmt.Errorf("store: update project for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update project for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// RestartService is the only way restart_nonce ever changes: SaveDesiredService's
// own doc comment (and this field's own doc comment on DesiredService)
// explains why it's deliberately excluded from that method's
// full-record-replace semantics, the same reasoning UpdateServiceNode
// already establishes for NodeID.
//
// The generated value is opaque and only ever compared for equality by
// internal/reconcile/application.ContainerName, never interpreted, so a
// short random hex string is enough; it does not need to be
// cryptographically unpredictable the way a session token or API key
// does, only different from whatever was there before.
func (db *DB) RestartService(ctx context.Context, name string) error {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("store: restart service %q: generate nonce: %w", name, err)
	}
	nonce := hex.EncodeToString(buf)

	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET restart_nonce = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nonce, name)
	if err != nil {
		return fmt.Errorf("store: restart service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: restart service %q: rows affected: %w", name, err)
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
		SELECT name, image, port, domains, env, secret_env, resources, health, node_id, strategy, replicas, restart_nonce, project_id, labels
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
		SELECT name, image, port, domains, env, secret_env, resources, health, node_id, strategy, replicas, restart_nonce, project_id, labels
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

// ListDesiredServicesByNode returns every saved service currently
// placed on nodeID, ordered by name. TASKS.md 3.7's drain
// (internal/api's handleDrainNode) uses this to find what to move off a
// node before it's removed, and handleDeleteNode uses it as the guard
// that makes node deletion refuse to run while placements remain.
// nodeID="" (the local-node sentinel, see DesiredService.NodeID's own
// doc comment) is a valid argument, matching every other node_id
// comparison in this package.
func (db *DB) ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]DesiredService, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, image, port, domains, env, secret_env, resources, health, node_id, strategy, replicas, restart_nonce, project_id, labels
		FROM desired_services
		WHERE node_id = ?
		ORDER BY name
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services for node %q: %w", nodeID, err)
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

// scanDesiredService reads the column shape both GetDesiredService
// and ListDesiredServices query, via either row.Scan or rows.Scan (same
// signature), so the decode-JSON-columns logic exists exactly once.
func scanDesiredService(scan func(dest ...any) error) (*DesiredService, error) {
	var (
		svc                                                                 DesiredService
		domainsJSON, envJSON, secretEnvJSON, resourcesJSON, health, labels string
		projectID                                                          sql.NullString
	)
	if err := scan(&svc.Name, &svc.Image, &svc.Port, &domainsJSON, &envJSON, &secretEnvJSON, &resourcesJSON, &health, &svc.NodeID, &svc.Strategy, &svc.Replicas, &svc.RestartNonce, &projectID, &labels); err != nil {
		return nil, err
	}
	svc.ProjectID = projectID.String

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
	if err := json.Unmarshal([]byte(labels), &svc.Labels); err != nil {
		return nil, fmt.Errorf("unmarshal labels: %w", err)
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
