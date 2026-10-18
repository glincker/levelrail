package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Resource kinds the console explorer and tags understand.
const (
	ResourceKindApp      = "app"
	ResourceKindDatabase = "database"
	ResourceKindNode     = "node"
	ResourceKindDomain   = "domain"
	ResourceKindBucket   = "bucket"
	ResourceKindNetwork  = "network"
)

// ErrResourceViewNotFound is returned when a saved view does not exist for its owner.
var ErrResourceViewNotFound = errors.New("store: resource view not found")

// ResourceRef identifies one taggable resource. Name is the stable ID for nodes, buckets and
// networks (so a rename keeps its tags) and the natural name for apps, databases and domains.
type ResourceRef struct {
	Kind string
	Name string
}

// Key is the "kind:name" form used to index tags and cost rows.
func (r ResourceRef) Key() string { return r.Kind + ":" + r.Name }

// ResourceView is a saved explorer query owned by one principal.
type ResourceView struct {
	ID        string
	Owner     string
	Name      string
	Query     string
	CreatedAt time.Time
}

// AttachResourceTag tags a resource, idempotently. Apps keep using app_tags so
// every existing app tag, filter and bulk action stays valid.
func (db *DB) AttachResourceTag(ctx context.Context, tagID string, ref ResourceRef) error {
	if ref.Kind == ResourceKindApp {
		return db.AttachAppTag(ctx, tagID, ref.Name)
	}
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO resource_tags (tag_id, kind, name, created_at) VALUES (?, ?, ?, ?)`,
		tagID, ref.Kind, ref.Name, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: attach tag %q to %s: %w", tagID, ref.Key(), err)
	}
	return nil
}

// DetachResourceTag removes a tag from a resource, or returns ErrTagNotFound.
func (db *DB) DetachResourceTag(ctx context.Context, tagID string, ref ResourceRef) error {
	if ref.Kind == ResourceKindApp {
		return db.DetachAppTag(ctx, tagID, ref.Name)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM resource_tags WHERE tag_id = ? AND kind = ? AND name = ?`, tagID, ref.Kind, ref.Name)
	if err != nil {
		return fmt.Errorf("store: detach tag %q from %s: %w", tagID, ref.Key(), err)
	}
	return rowsAffectedOrNotFound(res, ErrTagNotFound, "detach tag from %s", ref.Key())
}

// ListAllResourceTags returns every tag per "kind:name" key, apps included.
func (db *DB) ListAllResourceTags(ctx context.Context) (map[string][]Tag, error) {
	out := map[string][]Tag{}
	rows, err := db.QueryContext(ctx, `
		SELECT 'app', at.app_name, t.id, t.name, t.created_at FROM app_tags at JOIN tags t ON t.id = at.tag_id
		UNION ALL
		SELECT rt.kind, rt.name, t.id, t.name, t.created_at FROM resource_tags rt JOIN tags t ON t.id = rt.tag_id
		ORDER BY 1, 2, 4`)
	if err != nil {
		return nil, fmt.Errorf("store: list resource tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, name string
		t, err := scanTag(func(dest ...any) error {
			return rows.Scan(&kind, &name, dest[0], dest[1], dest[2])
		})
		if err != nil {
			return nil, fmt.Errorf("store: scan resource tag: %w", err)
		}
		key := ResourceRef{Kind: kind, Name: name}.Key()
		out[key] = append(out[key], *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate resource tags: %w", err)
	}
	return out, nil
}

// SaveResourceView creates or replaces the owner's view with the same name.
func (db *DB) SaveResourceView(ctx context.Context, v ResourceView) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO resource_views (id, owner, name, query, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(owner, name) DO UPDATE SET query = excluded.query`,
		v.ID, v.Owner, v.Name, v.Query, formatTime(v.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: save resource view %q: %w", v.Name, err)
	}
	return nil
}

// ListResourceViews returns the owner's views, alphabetical.
func (db *DB) ListResourceViews(ctx context.Context, owner string) ([]ResourceView, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, owner, name, query, created_at FROM resource_views WHERE owner = ? ORDER BY name`, owner)
	if err != nil {
		return nil, fmt.Errorf("store: list resource views: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ResourceView
	for rows.Next() {
		var v ResourceView
		var created string
		if err := rows.Scan(&v.ID, &v.Owner, &v.Name, &v.Query, &created); err != nil {
			return nil, fmt.Errorf("store: scan resource view: %w", err)
		}
		v.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("store: parse resource view time: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate resource views: %w", err)
	}
	return out, nil
}

// DeleteResourceView removes one of the owner's views.
func (db *DB) DeleteResourceView(ctx context.Context, owner, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM resource_views WHERE owner = ? AND id = ?`, owner, id)
	if err != nil {
		return fmt.Errorf("store: delete resource view %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrResourceViewNotFound, "delete resource view %q", id)
}
