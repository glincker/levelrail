// Package dbaccess manages database users, temporary credentials and network
// scope. Statements use quoted identifiers and a pre-hashed password verifier,
// so no plaintext secret reaches SQL text, container logs or an audit row.
package dbaccess

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	minRoleNameLen = 3
	maxRoleNameLen = 40

	// TempRolePrefix marks roles minted by the temporary access path so a
	// sweeper pass and a human can tell them apart at a glance.
	TempRolePrefix = "tmp_"
)

var (
	roleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	// ErrRoleName is wrapped by every role-name validation failure.
	ErrRoleName = errors.New("invalid role name")

	reservedRoleNames = map[string]bool{
		"postgres": true, "public": true, "none": true, "root": true, "admin": true,
		"replicator": true, "rdsadmin": true, "current_user": true, "current_role": true,
		"session_user": true, "user": true,
	}
)

// ValidateRoleName checks name against the identifier rules and the names
// the platform and Postgres reserve. adminRole is the platform's own admin
// role for this database, which can never be recreated or shadowed.
func ValidateRoleName(name, adminRole string) error {
	switch {
	case len(name) < minRoleNameLen || len(name) > maxRoleNameLen:
		return fmt.Errorf("%w: must be %d to %d characters", ErrRoleName, minRoleNameLen, maxRoleNameLen)
	case !roleNamePattern.MatchString(name):
		return fmt.Errorf("%w: use lowercase letters, digits and underscores, starting with a letter", ErrRoleName)
	case strings.HasPrefix(name, "pg_"):
		return fmt.Errorf("%w: the pg_ prefix is reserved for system roles", ErrRoleName)
	case strings.HasPrefix(name, TempRolePrefix):
		return fmt.Errorf("%w: the %s prefix is reserved for temporary credentials", ErrRoleName, TempRolePrefix)
	case reservedRoleNames[name]:
		return fmt.Errorf("%w: %q is reserved", ErrRoleName, name)
	case adminRole != "" && name == adminRole:
		return fmt.Errorf("%w: %q is the platform admin role for this database", ErrRoleName, name)
	}
	return nil
}

// Protected reports whether an existing role must never be altered or
// dropped through this package: the platform admin, any superuser, and
// system roles.
func Protected(r Role, adminRole string) bool {
	return r.Superuser || r.Name == adminRole || strings.HasPrefix(r.Name, "pg_") || reservedRoleNames[r.Name]
}

// QuoteIdent double-quotes a Postgres identifier.
func QuoteIdent(id string) string { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }

// QuoteLiteral single-quotes a Postgres string literal.
func QuoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
