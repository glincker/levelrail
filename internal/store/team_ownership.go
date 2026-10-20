package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ResourceOwner is the team and project an app or database belongs to. A
// resource with no project belongs to the default team.
type ResourceOwner struct {
	TeamID    string
	ProjectID string
}

const ownerColumns = `COALESCE(p.org_id, '` + DefaultTeamID + `'), COALESCE(r.project_id, '')`

func (db *DB) ownerOf(ctx context.Context, table, name string) (*ResourceOwner, error) {
	var o ResourceOwner
	err := db.QueryRowContext(ctx, `SELECT `+ownerColumns+` FROM `+table+` r LEFT JOIN projects p ON p.id = r.project_id WHERE r.name = ?`, name).
		Scan(&o.TeamID, &o.ProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: owner of %q: %w", name, err)
	}
	return &o, nil
}

func (db *DB) ownerMap(ctx context.Context, table string) (map[string]*ResourceOwner, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.name, `+ownerColumns+` FROM `+table+` r LEFT JOIN projects p ON p.id = r.project_id`)
	if err != nil {
		return nil, fmt.Errorf("store: owners of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]*ResourceOwner{}
	for rows.Next() {
		var name string
		var o ResourceOwner
		if err := rows.Scan(&name, &o.TeamID, &o.ProjectID); err != nil {
			return nil, fmt.Errorf("store: scan owner: %w", err)
		}
		out[name] = &o
	}
	return out, rows.Err()
}

// OwnerOfApp returns the app's team and project, or nil when it does not exist.
func (db *DB) OwnerOfApp(ctx context.Context, name string) (*ResourceOwner, error) {
	return db.ownerOf(ctx, "desired_services", name)
}

// OwnerOfDatabase returns the database's team and project, or nil.
func (db *DB) OwnerOfDatabase(ctx context.Context, name string) (*ResourceOwner, error) {
	return db.ownerOf(ctx, "desired_databases", name)
}

// OwnersOfApps maps every app to its owner in one query.
func (db *DB) OwnersOfApps(ctx context.Context) (map[string]*ResourceOwner, error) {
	return db.ownerMap(ctx, "desired_services")
}

// OwnersOfDatabases maps every database to its owner in one query.
func (db *DB) OwnersOfDatabases(ctx context.Context) (map[string]*ResourceOwner, error) {
	return db.ownerMap(ctx, "desired_databases")
}

// TeamOfProject returns the team owning a project, or ErrProjectNotFound.
func (db *DB) TeamOfProject(ctx context.Context, projectID string) (string, error) {
	var team string
	err := db.QueryRowContext(ctx, `SELECT COALESCE(org_id, '`+DefaultTeamID+`') FROM projects WHERE id = ?`, projectID).Scan(&team)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrProjectNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: team of project %q: %w", projectID, err)
	}
	return team, nil
}

// TeamOfEnvironment returns the team and project owning an environment.
func (db *DB) TeamOfEnvironment(ctx context.Context, environmentID string) (ResourceOwner, error) {
	var o ResourceOwner
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(p.org_id, '`+DefaultTeamID+`'), e.project_id
		FROM environments e JOIN projects p ON p.id = e.project_id WHERE e.id = ?`, environmentID).Scan(&o.TeamID, &o.ProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceOwner{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return ResourceOwner{}, fmt.Errorf("store: team of environment %q: %w", environmentID, err)
	}
	return o, nil
}

// ProjectTeams maps every project to its owning team.
func (db *DB) ProjectTeams(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(org_id, '`+DefaultTeamID+`') FROM projects`)
	if err != nil {
		return nil, fmt.Errorf("store: project teams: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, team string
		if err := rows.Scan(&id, &team); err != nil {
			return nil, fmt.Errorf("store: scan project team: %w", err)
		}
		out[id] = team
	}
	return out, rows.Err()
}

// CountTeamProjects returns how many projects a team owns.
func (db *DB) CountTeamProjects(ctx context.Context, teamID string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE org_id = ?`, teamID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count team projects: %w", err)
	}
	return n, nil
}
