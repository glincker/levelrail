package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrServiceNotFound is returned by GetDesiredService when no service
// has that name.
var ErrServiceNotFound = errors.New("store: service not found")

// ErrServiceVolumeNotFound is returned by ResolveServiceVolumeDockerName
// when serviceName has no volume by that logical name.
var ErrServiceVolumeNotFound = errors.New("store: service volume not found")

// ServiceVolumeDockerName finds svc's Docker volume for the logical name
// volumeName by stripping the fixed "app-<svc>-" prefix deploy applies. Exact
// prefix stripping, so logical names containing "-" are never misparsed.
func ServiceVolumeDockerName(svc DesiredService, volumeName string) (string, bool) {
	for _, v := range svc.Volumes {
		if v.Name == ServiceVolumeName(svc.Name, volumeName) {
			return v.Name, true
		}
	}
	return "", false
}

// ServiceVolumeName is the Docker volume name for a service's logical volume.
// Two services can map to one name ("a" with "b-c" and "a-b" with "c"), so
// callers that attach or touch data check ServiceVolumeOwnerConflict too.
func ServiceVolumeName(serviceName, logicalName string) string {
	return "app-" + serviceName + "-" + logicalName
}

// ServiceVolumeOwnerConflict returns another service in all that already holds
// the Docker volume dockerName, so a volume is never shared by name collision.
func ServiceVolumeOwnerConflict(all []DesiredService, owner, dockerName string) (string, bool) {
	for _, other := range all {
		if other.Name == owner {
			continue
		}
		for _, v := range other.Volumes {
			if v.Name == dockerName {
				return other.Name, true
			}
		}
	}
	return "", false
}

// ResolveServiceVolumeDockerName looks up serviceName and resolves
// volumeName to its real Docker volume name, the single DB-round-trip
// convenience ServiceVolumeDockerName's own callers that don't already
// have a DesiredService in hand need (internal/backup.Scheduler's own
// per-tick volume evaluation).
func (db *DB) ResolveServiceVolumeDockerName(ctx context.Context, serviceName, volumeName string) (string, error) {
	svc, err := db.GetDesiredService(ctx, serviceName)
	if err != nil {
		return "", err
	}
	dockerName, ok := ServiceVolumeDockerName(*svc, volumeName)
	if !ok {
		return "", ErrServiceVolumeNotFound
	}
	return dockerName, nil
}

// GetDesiredService returns the desired state for name, or
// ErrServiceNotFound if no such service has been saved.
func (db *DB) GetDesiredService(ctx context.Context, name string) (*DesiredService, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+desiredServiceColumns+`
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
	rows, err := db.reader().QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services: %w", err)
	}
	return collectDesiredServices(rows)
}

// ListDesiredServicesByNode returns every saved service currently
// placed on nodeID, ordered by name. Node drain
// (internal/api's handleDrainNode) uses this to find what to move off a
// node before it's removed, and handleDeleteNode uses it as the guard
// that makes node deletion refuse to run while placements remain.
// nodeID="" (the local-node sentinel, see DesiredService.NodeID's own
// doc comment) is a valid argument, matching every other node_id
// comparison in this package.
func (db *DB) ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]DesiredService, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE node_id = ?
		ORDER BY name
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services for node %q: %w", nodeID, err)
	}
	return collectDesiredServices(rows)
}

// ListDesiredServicesByProject returns every saved service filed under
// projectID, ordered by name, the project-kind counterpart to
// ListDesiredServicesByNode. Used by handleStopProject/handleStartProject
// (internal/api/project_stop_start.go) and internal/api's bulk-restart
// endpoint to find every app in a project without listing every service.
func (db *DB) ListDesiredServicesByProject(ctx context.Context, projectID string) ([]DesiredService, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE project_id = ?
		ORDER BY name
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services for project %q: %w", projectID, err)
	}
	return collectDesiredServices(rows)
}

// ListDesiredServicesByEnvironment returns every saved service tagged
// with environmentID, ordered by name, the environment-kind counterpart
// to ListDesiredServicesByProject above. Used by
// internal/api/environment_clone.go to find what a "clone environment"
// operation copies, without listing every service and filtering
// in-memory the way promote.go's own findPromotionCandidates still does
// for its narrower single-app lookup.
func (db *DB) ListDesiredServicesByEnvironment(ctx context.Context, environmentID string) ([]DesiredService, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE environment_id = ?
		ORDER BY name
	`, environmentID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services for environment %q: %w", environmentID, err)
	}
	return collectDesiredServices(rows)
}

// collectDesiredServices drains and closes rows of desiredServiceColumns.
func collectDesiredServices(rows *sql.Rows) ([]DesiredService, error) {
	defer func() { _ = rows.Close() }()
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

// DeleteDesiredService removes a service's desired state and returns
// ErrServiceNotFound if it does not exist. It does not stop running
// containers; converging to zero is the reconciler's job.
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
	// A recreated app of the same name must not inherit the old commit order.
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_cursors WHERE service_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete desired service %q: clear deploy cursor: %w", name, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_freeze_windows WHERE scope = ?`, DeployFreezeScopeApp(name)); err != nil {
		return fmt.Errorf("store: delete desired service %q: clear freeze windows: %w", name, err)
	}
	if err := db.DeleteAppTimelineData(ctx, name); err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	if err := db.DeleteDeployExits(ctx, name); err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	if err := db.DeleteAppSleep(ctx, name); err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	if err := db.DeleteRolloutProgress(ctx, name); err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	if err := db.DeleteAppForms(ctx, name); err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	return db.noteAppChange(ctx, name)
}

// LocalImageID is ImageID when it still belongs to the current Image.
func (s DesiredService) LocalImageID() string {
	if s.ImageIDRef != "" && s.ImageIDRef == s.Image {
		return s.ImageID
	}
	return ""
}
