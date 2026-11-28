package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/spec"
)

// NameKind says which kind of resource holds a name in the shared secret namespace.
type NameKind string

// Name kinds that can hold a name.
const (
	NameKindReserved NameKind = "reserved"
	NameKindApp      NameKind = "app"
	NameKindDatabase NameKind = "database"
	NameKindStack    NameKind = "stack"
	NameKindParked   NameKind = "parked"
)

// ErrNameTaken is returned when a name is held by a different kind of resource.
var ErrNameTaken = errors.New("store: name is not available")

// NameHolder returns the kind holding name in the shared namespace of apps,
// databases, compose stacks, parked deleted names and reserved names, or "" when free.
func (db *DB) NameHolder(ctx context.Context, name string) (NameKind, error) {
	if spec.IsReservedName(name) {
		return NameKindReserved, nil
	}
	checks := []struct {
		kind  NameKind
		query string
		args  []any
	}{
		{NameKindApp, `SELECT EXISTS(SELECT 1 FROM desired_services WHERE name = ?)`, []any{name}},
		{NameKindDatabase, `SELECT EXISTS(SELECT 1 FROM desired_databases WHERE name = ?)`, []any{name}},
		{NameKindStack, `SELECT EXISTS(SELECT 1 FROM desired_services WHERE app_id = ? AND name <> app_id)`, []any{name}},
		{NameKindParked, `SELECT EXISTS(SELECT 1 FROM recently_deleted WHERE name = ?)`, []any{name}},
	}
	for _, c := range checks {
		var found bool
		if err := db.QueryRowContext(ctx, c.query, c.args...).Scan(&found); err != nil {
			return "", fmt.Errorf("store: check name %q: %w", name, err)
		}
		if found {
			return c.kind, nil
		}
	}
	return "", nil
}

// refuseCrossKindName fails when a name is held by a kind other than own, ignoring parked rows
// because a restore reuses its parked name.
func (db *DB) refuseCrossKindName(ctx context.Context, name string, own NameKind) error {
	for _, other := range []NameKind{NameKindApp, NameKindDatabase, NameKindStack} {
		if other == own {
			continue
		}
		var found bool
		var query string
		switch other {
		case NameKindApp:
			query = `SELECT EXISTS(SELECT 1 FROM desired_services WHERE name = ?)`
		case NameKindDatabase:
			query = `SELECT EXISTS(SELECT 1 FROM desired_databases WHERE name = ?)`
		default:
			query = `SELECT EXISTS(SELECT 1 FROM desired_services WHERE app_id = ? AND name <> app_id)`
		}
		if err := db.QueryRowContext(ctx, query, name).Scan(&found); err != nil {
			return fmt.Errorf("store: check name %q: %w", name, err)
		}
		if found {
			return fmt.Errorf("%w: %q", ErrNameTaken, name)
		}
	}
	return nil
}

// checkNewNameKind applies the cross-kind rule to a name that has no row of its own kind yet,
// so a collision already in the data never blocks saving the existing row.
func (db *DB) checkNewNameKind(ctx context.Context, name string, own NameKind) error {
	table := "desired_services"
	if own == NameKindDatabase {
		table = "desired_databases"
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE name = ?)`, name).Scan(&exists); err != nil {
		return fmt.Errorf("store: check name %q: %w", name, err)
	}
	if exists {
		return nil
	}
	return db.refuseCrossKindName(ctx, name, own)
}

// NameCollision is one name held by more than one kind.
type NameCollision struct {
	Name  string
	Kinds []NameKind
}

// NameCollisions lists names already held by two kinds (app and database, or app and stack, or database and stack).
func (db *DB) NameCollisions(ctx context.Context) ([]NameCollision, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.name, 'app' FROM desired_databases d JOIN desired_services s ON s.name = d.name
		UNION ALL
		SELECT s.app_id, 'stack_database' FROM desired_services s JOIN desired_databases d ON d.name = s.app_id WHERE s.name <> s.app_id
		UNION ALL
		SELECT s.app_id, 'stack_app' FROM desired_services s JOIN desired_services o ON o.name = s.app_id WHERE s.name <> s.app_id
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list name collisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byName := map[string][]NameKind{}
	for rows.Next() {
		var name, tag string
		if err := rows.Scan(&name, &tag); err != nil {
			return nil, fmt.Errorf("store: scan name collision: %w", err)
		}
		kinds := byName[name]
		switch tag {
		case "app":
			kinds = append(kinds, NameKindApp, NameKindDatabase)
		case "stack_database":
			kinds = append(kinds, NameKindStack, NameKindDatabase)
		default:
			kinds = append(kinds, NameKindStack, NameKindApp)
		}
		byName[name] = kinds
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]NameCollision, 0, len(byName))
	for name, kinds := range byName {
		out = append(out, NameCollision{Name: name, Kinds: uniqueKinds(kinds)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func uniqueKinds(in []NameKind) []NameKind {
	seen := map[NameKind]bool{}
	var out []NameKind
	for _, k := range in {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// String joins kinds for display.
func (c NameCollision) String() string {
	parts := make([]string, len(c.Kinds))
	for i, k := range c.Kinds {
		parts[i] = string(k)
	}
	return c.Name + " (" + strings.Join(parts, ", ") + ")"
}

// checkStackNameNotDatabase refuses a new stack member whose stack name is a database name,
// since the stack's generated values would land in the database's credential namespace.
func (db *DB) checkStackNameNotDatabase(ctx context.Context, svc DesiredService) error {
	if svc.AppID == "" || svc.AppID == svc.Name {
		return nil
	}
	var exists, member bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM desired_services WHERE name = ?)`, svc.Name).Scan(&member); err != nil {
		return fmt.Errorf("store: check name %q: %w", svc.Name, err)
	}
	if member {
		return nil
	}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM desired_databases WHERE name = ?)`, svc.AppID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check name %q: %w", svc.AppID, err)
	}
	if exists {
		return fmt.Errorf("%w: %q", ErrNameTaken, svc.AppID)
	}
	return nil
}
