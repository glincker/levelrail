package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// ErrInvalidServiceName is returned when a new service name could collide with a secret namespace.
var ErrInvalidServiceName = errors.New("store: invalid service name")

// ServiceNameNamespaceProblem names the rule a service name breaks, or "" when it is safe.
// Secrets live under the service name, so a name with a path separator can alias an internal namespace such as deleted-app/web.
func ServiceNameNamespaceProblem(name string) string {
	if name == "" {
		return "name is empty"
	}
	for _, r := range name {
		switch {
		case r == '/' || r == '\\':
			return "must not contain a slash or backslash (secret namespaces such as deleted-app/<name> use them)"
		case r == 0 || unicode.IsSpace(r) || unicode.IsControl(r):
			return "must not contain whitespace, NUL or control characters"
		}
	}
	if strings.Contains(name, "..") {
		return "must not contain \"..\""
	}
	if spec.IsReservedName(name) {
		return "is reserved for control plane settings"
	}
	return ""
}

// checkNewServiceName rejects a bad name unless the row already exists, so old offenders keep saving.
func (db *DB) checkNewServiceName(ctx context.Context, name string) error {
	problem := ServiceNameNamespaceProblem(name)
	if problem == "" {
		return nil
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM desired_services WHERE name = ?`, name).Scan(&n); err != nil {
		return fmt.Errorf("store: check service name %q: %w", name, err)
	}
	if n > 0 {
		return nil
	}
	return fmt.Errorf("%w: %q %s", ErrInvalidServiceName, name, problem)
}

// ServiceNamesBreakingNamespaceRule lists existing services whose names break the namespace rule.
func (db *DB) ServiceNamesBreakingNamespaceRule(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM desired_services`)
	if err != nil {
		return nil, fmt.Errorf("store: list service names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var bad []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan service name: %w", err)
		}
		if ServiceNameNamespaceProblem(n) != "" {
			bad = append(bad, n)
		}
	}
	return bad, rows.Err()
}
