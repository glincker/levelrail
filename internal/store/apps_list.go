package store

import (
	"context"
	"fmt"
	"strings"
)

// AppListFilter narrows and pages the app list. Zero values mean no
// filter; Limit 0 means no paging.
type AppListFilter struct {
	Query         string
	ProjectID     string
	Environment   string
	Tags          []string
	Limit, Offset int
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (f AppListFilter) where() (string, []any) {
	var clauses []string
	var args []any
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" {
		clauses = append(clauses, `(lower(name) LIKE ? ESCAPE '\' OR lower(image) LIKE ? ESCAPE '\')`)
		like := "%" + likeEscaper.Replace(q) + "%"
		args = append(args, like, like)
	}
	if f.ProjectID != "" {
		clauses = append(clauses, `project_id = ?`)
		args = append(args, f.ProjectID)
	}
	if f.Environment != "" {
		clauses = append(clauses, `environment_id IN (SELECT id FROM environments WHERE id = ? OR name = ?)`)
		args = append(args, f.Environment, f.Environment)
	}
	if n := len(f.Tags); n > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", n), ",")
		clauses = append(clauses, `name IN (
			SELECT at.app_name FROM app_tags at JOIN tags t ON t.id = at.tag_id
			WHERE t.name IN (`+marks+`)
			GROUP BY at.app_name HAVING COUNT(DISTINCT t.name) = ?)`)
		for _, t := range f.Tags {
			args = append(args, t)
		}
		args = append(args, n)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// ListDesiredServicesFiltered returns one page of services matching f,
// plus the total match count before paging.
func (db *DB) ListDesiredServicesFiltered(ctx context.Context, f AppListFilter) ([]DesiredService, int, error) {
	where, args := f.where()
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
	where, args := f.where()
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
