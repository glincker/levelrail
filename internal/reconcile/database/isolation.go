package database

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// maxIsolatedRoleNameLen keeps the role name under Postgres' 63-byte
// identifier limit with room to spare. Reused as-is for Redis ACL
// usernames (no such limit there, but sharing one naming scheme across
// engines avoids a second one).
const maxIsolatedRoleNameLen = 48

var roleUnsafeChars = regexp.MustCompile(`[^a-z0-9_]+`)

// IsolatedRoleName derives a deterministic, safe identifier for one
// preview's isolated credential on sourceKey: lowercase, only
// [a-z0-9_], truncated to stay well under Postgres' 63-byte identifier
// limit. Used both as a Postgres role name and, unchanged, as a Redis
// ACL username. Deterministic so a redeploy of the same preview finds
// the same role/user rather than minting a new one.
func IsolatedRoleName(previewName, sourceKey string) string {
	raw := "pv_" + previewName + "_" + sourceKey
	safe := roleUnsafeChars.ReplaceAllString(strings.ToLower(raw), "_")
	if len(safe) > maxIsolatedRoleNameLen {
		safe = safe[:maxIsolatedRoleNameLen]
	}
	return strings.Trim(safe, "_")
}

// GenerateIsolatedRolePassword returns a random password for a new
// isolated role, the same 24-byte/base64url shape the platform's own
// managed database credentials already use.
func GenerateIsolatedRolePassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("database: generate isolated role password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// quoteIdentifier double-quotes a Postgres identifier, escaping any
// embedded double quote. role and dbName are always code-derived
// (IsolatedRoleName, an existing desired_databases.name), never raw user
// input, but this is defense in depth.
func quoteIdentifier(id string) string {
	return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
}

// quoteLiteral single-quotes a Postgres string literal, escaping any
// embedded single quote. password is always our own generated value
// (base64url, no quotes possible), but this is defense in depth too.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// CreateIsolatedRoleSQL returns the SQL script that creates role (if it
// does not already exist) and grants it read/write on every table and
// sequence in dbName's public schema, including ones created later.
// Idempotent: safe to run again against a role that already exists.
func CreateIsolatedRoleSQL(role, dbName, password string) string {
	r, d, p := quoteIdentifier(role), quoteIdentifier(dbName), quoteLiteral(password)
	return fmt.Sprintf(`DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %s) THEN
    CREATE ROLE %s LOGIN PASSWORD %s;
  ELSE
    ALTER ROLE %s PASSWORD %s;
  END IF;
END $$;
GRANT CONNECT ON DATABASE %s TO %s;
GRANT USAGE ON SCHEMA public TO %s;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %s;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %s;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO %s;
`, quoteLiteral(role), r, p, r, p, d, r, r, r, r, r, r)
}

// DropIsolatedRoleSQL returns the SQL script that revokes role's grants
// on dbName and drops it. Idempotent: DROP ROLE IF EXISTS tolerates a
// role that is already gone (a retried teardown).
func DropIsolatedRoleSQL(role, dbName string) string {
	r, d := quoteIdentifier(role), quoteIdentifier(dbName)
	return fmt.Sprintf(`REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %s;
REVOKE ALL PRIVILEGES ON SCHEMA public FROM %s;
REVOKE CONNECT ON DATABASE %s FROM %s;
DROP ROLE IF EXISTS %s;
`, r, r, r, d, r, r)
}

// CreateIsolatedRedisACLCommand returns the redis-cli argv that creates
// or resets username's ACL entry: password-authenticated, restricted to
// keys under its own "username:" prefix (~prefix:* per Redis 6+ ACL key
// patterns), every command category except @admin/@dangerous/@scripting
// (scripting is excluded because a Lua script's own key access isn't
// reliably bound by the caller's key-pattern ACL). "reset" first clears
// any prior state for this username so a redeploy converges to the same
// rules rather than layering onto whatever ran before; idempotent, safe
// to run again against a username that already exists.
func CreateIsolatedRedisACLCommand(username, password string) []string {
	return []string{
		"redis-cli", "-p", strconv.Itoa(redisContainerPort),
		"ACL", "SETUSER", username,
		"reset", "on", ">" + password,
		"resetkeys", "~" + username + ":*",
		"resetchannels",
		"+@all", "-@admin", "-@dangerous", "-@scripting",
	}
}

// DropIsolatedRedisACLCommand returns the redis-cli argv that removes
// username's ACL entry. ACL DELUSER on a username that doesn't exist
// returns 0 deleted rather than erroring, so this is idempotent, safe to
// retry the same way DropIsolatedRoleSQL's DROP ROLE IF EXISTS is.
func DropIsolatedRedisACLCommand(username string) []string {
	return []string{"redis-cli", "-p", strconv.Itoa(redisContainerPort), "ACL", "DELUSER", username}
}
