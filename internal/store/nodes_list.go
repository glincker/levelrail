package store

import (
	"context"
	"fmt"
	"strings"
)

// NodeListFilter narrows and pages the node list, mirroring
// AppListFilter (apps_list.go). Zero values mean no filter; Limit 0
// means no paging.
type NodeListFilter struct {
	Query         string
	Limit, Offset int
}

func (f NodeListFilter) where() (string, []any) {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	if q == "" {
		return "", nil
	}
	like := "%" + likeEscaper.Replace(q) + "%"
	return ` WHERE (lower(name) LIKE ? ESCAPE '\' OR lower(address) LIKE ? ESCAPE '\')`, []any{like, like}
}

// ListNodesFiltered returns one page of nodes matching f, ordered by
// name, plus the total match count before paging. ListNodes stays the
// simple unpaged primitive every existing caller already uses; this is
// the new surface for the API/CLI's paginated list, the same "add a
// filtered sibling rather than change the existing signature" shape
// apps_list.go's ListDesiredServicesFiltered already uses next to
// ListApps.
func (db *DB) ListNodesFiltered(ctx context.Context, f NodeListFilter) ([]Node, int, error) {
	where, args := f.where()
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count filtered nodes: %w", err)
	}

	query := nodeSelectColumns + ` FROM nodes` + where + ` ORDER BY name`
	if f.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, f.Limit, f.Offset)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list filtered nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Node
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("store: scan filtered node row: %w", err)
		}
		out = append(out, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate filtered node rows: %w", err)
	}
	return out, total, nil
}
