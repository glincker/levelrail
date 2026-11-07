package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
)

// desiredServiceColumns is the column list every desired_services SELECT
// in this package shares, kept in one place so scanDesiredService's
// destination order and each query's column order can never drift apart.
const desiredServiceColumns = "name, image, port, host_port, bind_address, domains, env, secret_env, env_dirty, database_env, vault_env, resources, health, hooks, egress_policy, node_id, strategy, replicas, restart_nonce, project_id, labels, storage_target_id, suspended, app_id, volumes, registry_credential_id, database_attachment_name, database_attachment_env_var, database_attachment_field, log_drain, environment_id, command, bind_mounts, entrypoint, pull_policy, preview_env_overrides, auto_rollback_on_crashloop, exec_enabled, image_id, image_id_ref, depends_on, auto_rollback_on_slo_burn, is_trial, badge_enabled, depends_on_healthy, published_ports, template_id, files, maintenance_hold, incarnation, tolerations, affinity, next_cache"

// scanDesiredService reads the column shape both GetDesiredService
// and ListDesiredServices query, via either row.Scan or rows.Scan (same
// signature), so the decode-JSON-columns logic exists exactly once.
func scanDesiredService(scan func(dest ...any) error) (*DesiredService, error) {
	var (
		svc                                                                                                                                                                                 DesiredService
		domainsJSON, envJSON, secretEnvJSON, databaseEnvJSON, vaultEnvJSON, resourcesJSON, health, hooks, egress, labels, volumes, command, bindMounts, entrypoint, previewEnvOverridesJSON string
		projectID, storageTargetID, appID, logDrainJSON, environmentID                                                                                                                      sql.NullString
		hostPort                                                                                                                                                                            sql.NullInt64
		dbAttachmentName, dbAttachmentEnvVar, dbAttachmentField                                                                                                                             string
		dependsOnJSON, dependsOnHealthyJSON, publishedPortsJSON, filesJSON, tolerationsJSON, affinityJSON                                                                                   string
	)
	if err := scan(&svc.Name, &svc.Image, &svc.Port, &hostPort, &svc.BindAddress, &domainsJSON, &envJSON, &secretEnvJSON, &svc.EnvDirty, &databaseEnvJSON, &vaultEnvJSON, &resourcesJSON, &health, &hooks, &egress, &svc.NodeID, &svc.Strategy, &svc.Replicas, &svc.RestartNonce, &projectID, &labels, &storageTargetID, &svc.Suspended, &appID, &volumes, &svc.RegistryCredentialID, &dbAttachmentName, &dbAttachmentEnvVar, &dbAttachmentField, &logDrainJSON, &environmentID, &command, &bindMounts, &entrypoint, &svc.PullPolicy, &previewEnvOverridesJSON, &svc.AutoRollbackOnCrashloop, &svc.ExecEnabled, &svc.ImageID, &svc.ImageIDRef, &dependsOnJSON, &svc.AutoRollbackOnSLOBurn, &svc.IsTrial, &svc.BadgeEnabled, &dependsOnHealthyJSON, &publishedPortsJSON, &svc.TemplateID, &filesJSON, &svc.MaintenanceHold, &svc.Incarnation, &tolerationsJSON, &affinityJSON, &svc.NextCache); err != nil {
		return nil, err
	}
	svc.ProjectID = projectID.String
	svc.StorageTargetID = storageTargetID.String
	svc.AppID = appID.String
	svc.EnvironmentID = environmentID.String
	if hostPort.Valid {
		v := int(hostPort.Int64)
		svc.HostPort = &v
	}
	if dbAttachmentName != "" {
		svc.DatabaseAttachment = &DatabaseAttachment{DatabaseName: dbAttachmentName, EnvVar: dbAttachmentEnvVar, Field: dbAttachmentField}
	}

	if err := decodeJSONCol(domainsJSON, &svc.Domains); err != nil {
		return nil, fmt.Errorf("unmarshal domains: %w", err)
	}
	if err := decodeJSONCol(envJSON, &svc.Env); err != nil {
		return nil, fmt.Errorf("unmarshal env: %w", err)
	}
	secretEnv, err := unmarshalSecretEnv(secretEnvJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal secret_env: %w", err)
	}
	svc.SecretEnv = secretEnv
	if err := decodeJSONCol(databaseEnvJSON, &svc.DatabaseEnv); err != nil {
		return nil, fmt.Errorf("unmarshal database_env: %w", err)
	}
	if err := decodeJSONCol(vaultEnvJSON, &svc.VaultEnv); err != nil {
		return nil, fmt.Errorf("unmarshal vault_env: %w", err)
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
	if hooks != "null" {
		if err := json.Unmarshal([]byte(hooks), &svc.Hooks); err != nil {
			return nil, fmt.Errorf("unmarshal hooks: %w", err)
		}
	}
	if egress != "null" {
		if err := json.Unmarshal([]byte(egress), &svc.Egress); err != nil {
			return nil, fmt.Errorf("unmarshal egress_policy: %w", err)
		}
	}
	if err := decodeJSONCol(labels, &svc.Labels); err != nil {
		return nil, fmt.Errorf("unmarshal labels: %w", err)
	}
	if err := decodeJSONCol(volumes, &svc.Volumes); err != nil {
		return nil, fmt.Errorf("unmarshal volumes: %w", err)
	}
	if err := decodeJSONCol(command, &svc.Command); err != nil {
		return nil, fmt.Errorf("unmarshal command: %w", err)
	}
	if err := decodeJSONCol(bindMounts, &svc.BindMounts); err != nil {
		return nil, fmt.Errorf("unmarshal bind_mounts: %w", err)
	}
	if err := decodeJSONCol(entrypoint, &svc.Entrypoint); err != nil {
		return nil, fmt.Errorf("unmarshal entrypoint: %w", err)
	}
	if err := decodeJSONCol(dependsOnJSON, &svc.DependsOn); err != nil {
		return nil, fmt.Errorf("unmarshal depends_on: %w", err)
	}
	if err := decodeJSONCol(dependsOnHealthyJSON, &svc.DependsOnHealthy); err != nil {
		return nil, fmt.Errorf("unmarshal depends_on_healthy: %w", err)
	}
	if err := decodeJSONCol(filesJSON, &svc.Files); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	if len(svc.Files) == 0 {
		svc.Files = nil
	}
	if err := decodeJSONCol(publishedPortsJSON, &svc.PublishedPorts); err != nil {
		return nil, fmt.Errorf("unmarshal published_ports: %w", err)
	}
	if len(svc.PublishedPorts) == 0 {
		svc.PublishedPorts = nil
	}
	if err := decodeJSONCol(previewEnvOverridesJSON, &svc.PreviewEnvOverrides); err != nil {
		return nil, fmt.Errorf("unmarshal preview_env_overrides: %w", err)
	}
	if tolerationsJSON != "" && tolerationsJSON != "null" {
		svc.Tolerations = json.RawMessage(tolerationsJSON)
	}
	if affinityJSON != "" && affinityJSON != "null" {
		svc.Affinity = json.RawMessage(affinityJSON)
	}
	if logDrainJSON.Valid {
		if err := json.Unmarshal([]byte(logDrainJSON.String), &svc.LogDrain); err != nil {
			return nil, fmt.Errorf("unmarshal log_drain: %w", err)
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

// nonNilSecretEnv is nonNilSlice's counterpart for SecretEnv, which
// can't share that generic-less helper since it holds SecretEnvRef, not
// string.
func nonNilSecretEnv(s []SecretEnvRef) []SecretEnvRef {
	if s == nil {
		return []SecretEnvRef{}
	}
	return s
}

// unmarshalSecretEnv decodes secret_env's JSON, accepting both the
// current { name, required } object shape and the plain string-name
// array shape every row written before Required existed still has on
// disk (secret_env is a JSON blob column, migrations/0006: reshaping it
// is a decode-time concern, not a SQL migration). An old row decodes
// with Required defaulting to false for each name, the same permissive
// direction a declared-but-unresolvable secret already falls back to
// elsewhere in this codebase.
func unmarshalSecretEnv(raw string) ([]SecretEnvRef, error) {
	var refs []SecretEnvRef
	if err := json.Unmarshal([]byte(raw), &refs); err == nil {
		return refs, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	refs = make([]SecretEnvRef, len(names))
	for i, n := range names {
		refs[i] = SecretEnvRef{Name: n}
	}
	return refs, nil
}

// SecretEnvRefsFromNames builds []SecretEnvRef for names with Required
// false for each: the shape a caller that only ever tracks secret-backed
// env var names by name needs (e.g. the direct app API and Docker
// Compose imports, neither of which has an app.yaml { required: true }
// flag to carry over).
func SecretEnvRefsFromNames(names []string) []SecretEnvRef {
	if len(names) == 0 {
		return nil
	}
	refs := make([]SecretEnvRef, len(names))
	for i, n := range names {
		refs[i] = SecretEnvRef{Name: n}
	}
	return refs
}

// SecretEnvNames extracts just the names from refs, the shape a wire
// type like appResource.SecretEnv ([]string) still uses.
func SecretEnvNames(refs []SecretEnvRef) []string {
	if len(refs) == 0 {
		return nil
	}
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return names
}

// hostPortToNull converts DesiredService.HostPort to the database/sql
// type ExecContext needs to write either a real value or a genuine SQL
// NULL, the same *int-to-sql.NullInt64 idiom github_app.go's
// nullableInt64 already establishes for *int64.
func hostPortToNull(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

// decodeJSONCol is json.Unmarshal with the empty-literal cases answered
// without reflection: "[]" and "{}" still yield non-nil empty values, so the
// result is identical, but the common empty column costs no decoder allocs.
func decodeJSONCol[T any](raw string, dst *T) error {
	switch raw {
	case "null":
		return nil
	case "[]":
		if v := reflect.ValueOf(dst).Elem(); v.Kind() == reflect.Slice {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			return nil
		}
	case "{}":
		if v := reflect.ValueOf(dst).Elem(); v.Kind() == reflect.Map {
			v.Set(reflect.MakeMap(v.Type()))
			return nil
		}
	}
	return json.Unmarshal([]byte(raw), dst)
}
