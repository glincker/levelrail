package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// AppListFilter narrows and pages the app list. Zero values mean no
// filter; Limit 0 means no paging.
type AppListFilter struct {
	Query         string
	ProjectID     string
	Environment   string
	Environments  []string
	AnyTag        bool
	Tags          []string
	Names         []string
	Limit, Offset int
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (f AppListFilter) where() (string, []any, error) {
	clauses := []string{`name NOT LIKE ? ESCAPE '\'`, `name NOT IN (SELECT service_name FROM deploy_urls)`}
	args := []any{"%" + likeEscaper.Replace(CanaryServiceSuffix)}
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" {
		clauses = append(clauses, `(lower(name) LIKE ? ESCAPE '\' OR lower(image) LIKE ? ESCAPE '\')`)
		like := "%" + likeEscaper.Replace(q) + "%"
		args = append(args, like, like)
	}
	if f.Names != nil {
		bNames, err := json.Marshal(f.Names)
		if err != nil {
			return "", nil, fmt.Errorf("marshal names: %w", err)
		}
		clauses = append(clauses, `name IN (SELECT value FROM json_each(?))`)
		args = append(args, string(bNames))
	}
	if f.ProjectID != "" {
		clauses = append(clauses, `project_id = ?`)
		args = append(args, f.ProjectID)
	}
	if f.Environment != "" {
		clauses = append(clauses, `environment_id IN (SELECT id FROM environments WHERE id = ? OR name = ?)`)
		args = append(args, f.Environment, f.Environment)
	}
	if len(f.Environments) > 0 {
		bEnvs, err := json.Marshal(f.Environments)
		if err != nil {
			return "", nil, fmt.Errorf("marshal environments: %w", err)
		}
		clauses = append(clauses, `environment_id IN (SELECT id FROM environments WHERE id IN (SELECT value FROM json_each(?)) OR name IN (SELECT value FROM json_each(?)))`)
		args = append(args, string(bEnvs), string(bEnvs))
	}
	if n := len(f.Tags); n > 0 {
		need := n
		if f.AnyTag {
			need = 1
		}
		bTags, err := json.Marshal(f.Tags)
		if err != nil {
			return "", nil, fmt.Errorf("marshal tags: %w", err)
		}
		clauses = append(clauses, `name IN (
			SELECT at.app_name FROM app_tags at JOIN tags t ON t.id = at.tag_id
			WHERE t.name IN (SELECT value FROM json_each(?))
			GROUP BY at.app_name HAVING COUNT(DISTINCT t.name) >= ?)`)
		args = append(args, string(bTags), need)
	}
	if len(clauses) == 0 {
		return "", nil, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

// ListDesiredServicesFiltered returns one page of services matching f,
// plus the total match count before paging.
func (db *DB) ListDesiredServicesFiltered(ctx context.Context, f AppListFilter) ([]DesiredService, int, error) {
	where, args, err := f.where()
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM desired_services`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count filtered services: %w", err)
	}
	query := `SELECT ` + desiredServiceColumns + ` FROM desired_services` + where + ` ORDER BY name`
	if f.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, f.Limit, f.Offset)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list filtered services: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DesiredService
	if total > 0 {
		out = make([]DesiredService, 0, total)
		if f.Limit > 0 {
			out = make([]DesiredService, 0, min(total, f.Limit))
		}
	}
	for rows.Next() {
		svc, err := scanDesiredService(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("store: scan filtered service row: %w", err)
		}
		out = append(out, *svc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate filtered service rows: %w", err)
	}
	return out, total, nil
}

// ListAppNamesFiltered returns every matching app name, unpaged and
// without loading service graphs, for summary counts and bulk targeting.
func (db *DB) ListAppNamesFiltered(ctx context.Context, f AppListFilter) ([]string, error) {
	where, args, err := f.where()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM desired_services`+where+` ORDER BY name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list filtered app names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan filtered app name: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate filtered app names: %w", err)
	}
	return out, nil
}

// ListServiceHeads returns every service matching f ordered by name with only
// Name, NodeID, EnvironmentID, Domains and Volumes set. Status, summary and
// coverage callers need no more, and skipping the other JSON columns keeps
// those requests cheap at high app counts.
func (db *DB) ListServiceHeads(ctx context.Context, f AppListFilter) ([]DesiredService, error) {
	where, args, err := f.where()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT name, node_id, environment_id, domains, volumes FROM desired_services`+where+` ORDER BY name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list service heads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DesiredService
	for rows.Next() {
		var (
			svc                  DesiredService
			envID                sql.NullString
			domainsJSON, volumes string
		)
		if err := rows.Scan(&svc.Name, &svc.NodeID, &envID, &domainsJSON, &volumes); err != nil {
			return nil, fmt.Errorf("store: scan service head row: %w", err)
		}
		svc.EnvironmentID = envID.String
		if err := json.Unmarshal([]byte(domainsJSON), &svc.Domains); err != nil {
			return nil, fmt.Errorf("store: unmarshal service head domains: %w", err)
		}
		if err := json.Unmarshal([]byte(volumes), &svc.Volumes); err != nil {
			return nil, fmt.Errorf("store: unmarshal service head volumes: %w", err)
		}
		out = append(out, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate service head rows: %w", err)
	}
	return out, nil
}
