package dbaccess

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Preset is a named privilege bundle on the database's public schema.
type Preset string

// Presets.
const (
	PresetReadOnly  Preset = "read_only"
	PresetReadWrite Preset = "read_write"
	PresetOwner     Preset = "owner"
)

// ErrPreset is returned for an unknown preset.
var ErrPreset = errors.New("unknown preset")

// ValidPreset reports whether p is a known preset.
func ValidPreset(p Preset) bool {
	return p == PresetReadOnly || p == PresetReadWrite || p == PresetOwner
}

// TempPresetAllowed reports whether a temporary credential may use p. Owner
// is excluded: short-lived access should never be able to alter schema.
func TempPresetAllowed(p Preset) bool { return p == PresetReadOnly || p == PresetReadWrite }

// Role is one database role as the engine reports it.
type Role struct {
	Name        string `json:"name"`
	CanLogin    bool   `json:"can_login"`
	Superuser   bool   `json:"superuser"`
	CreateDB    bool   `json:"create_db"`
	CreateRole  bool   `json:"create_role"`
	Replication bool   `json:"replication"`
	ConnLimit   int    `json:"connection_limit"`
	ValidUntil  string `json:"valid_until,omitempty"`
	Connections int    `json:"connections"`
}

// CreateParams describes a role to create. Verifier is a SCRAM verifier from
// ScramVerifier, never a plaintext password.
type CreateParams struct {
	Role       string
	Database   string
	AdminRole  string
	Preset     Preset
	Verifier   string
	ConnLimit  int
	ValidUntil time.Time
}

const maxIdentLen = 63

const sqlTimeLayout = "2006-01-02 15:04:05+00"

// CreateRoleSQL builds the transaction that creates the role and applies its
// preset. Every identifier is validated and quoted; the password is a
// verifier.
func CreateRoleSQL(p CreateParams) (string, error) {
	if err := validateIdent(p.Role); err != nil {
		return "", err
	}
	if !ValidPreset(p.Preset) {
		return "", fmt.Errorf("%w: %q", ErrPreset, p.Preset)
	}
	if !strings.HasPrefix(p.Verifier, "SCRAM-SHA-256$") {
		return "", errors.New("dbaccess: password must be supplied as a SCRAM verifier")
	}
	r, d, admin := QuoteIdent(p.Role), QuoteIdent(p.Database), QuoteIdent(p.AdminRole)
	var b strings.Builder
	b.WriteString("BEGIN;\n")
	fmt.Fprintf(&b, "CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %s", r, QuoteLiteral(p.Verifier))
	if p.ConnLimit > 0 {
		fmt.Fprintf(&b, " CONNECTION LIMIT %d", p.ConnLimit)
	}
	if !p.ValidUntil.IsZero() {
		fmt.Fprintf(&b, " VALID UNTIL %s", QuoteLiteral(p.ValidUntil.UTC().Format(sqlTimeLayout)))
	}
	b.WriteString(";\n")
	fmt.Fprintf(&b, "GRANT CONNECT ON DATABASE %s TO %s;\n", d, r)
	fmt.Fprintf(&b, "GRANT USAGE ON SCHEMA public TO %s;\n", r)
	b.WriteString(presetGrants(p.Preset, r, admin))
	b.WriteString("COMMIT;\n")
	return b.String(), nil
}

func presetGrants(p Preset, role, admin string) string {
	switch p {
	case PresetReadOnly:
		return fmt.Sprintf(`GRANT SELECT ON ALL TABLES IN SCHEMA public TO %[1]s;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO %[1]s;
ALTER DEFAULT PRIVILEGES FOR ROLE %[2]s IN SCHEMA public GRANT SELECT ON TABLES TO %[1]s;
ALTER DEFAULT PRIVILEGES FOR ROLE %[2]s IN SCHEMA public GRANT SELECT ON SEQUENCES TO %[1]s;
ALTER ROLE %[1]s SET default_transaction_read_only = on;
`, role, admin)
	case PresetReadWrite:
		return fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %[1]s;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO %[1]s;
ALTER DEFAULT PRIVILEGES FOR ROLE %[2]s IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %[1]s;
ALTER DEFAULT PRIVILEGES FOR ROLE %[2]s IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO %[1]s;
`, role, admin)
	default:
		return fmt.Sprintf(`GRANT ALL PRIVILEGES ON SCHEMA public TO %[1]s;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO %[1]s;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO %[1]s;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO %[1]s;
ALTER DEFAULT PRIVILEGES FOR ROLE %[2]s IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO %[1]s;
ALTER DEFAULT PRIVILEGES FOR ROLE %[2]s IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO %[1]s;
`, role, admin)
	}
}

// RotateSQL replaces role's password with verifier and ends its sessions.
func RotateSQL(role, verifier string) (string, error) {
	if err := validateIdent(role); err != nil {
		return "", err
	}
	if !strings.HasPrefix(verifier, "SCRAM-SHA-256$") {
		return "", errors.New("dbaccess: password must be supplied as a SCRAM verifier")
	}
	return fmt.Sprintf("ALTER ROLE %s PASSWORD %s;\n%s", QuoteIdent(role), QuoteLiteral(verifier), terminateSQL(role)), nil
}

// SetLoginSQL enables or disables login. Disabling also ends open sessions.
func SetLoginSQL(role string, login bool) (string, error) {
	if err := validateIdent(role); err != nil {
		return "", err
	}
	if login {
		return fmt.Sprintf("ALTER ROLE %s LOGIN;\n", QuoteIdent(role)), nil
	}
	return fmt.Sprintf("ALTER ROLE %s NOLOGIN;\n%s", QuoteIdent(role), terminateSQL(role)), nil
}

func terminateSQL(role string) string {
	return fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = %s AND pid <> pg_backend_pid();\n", QuoteLiteral(role))
}

// DropRoleSQL ends the role's sessions, hands anything it owns to the admin
// role (never dropping data), strips its privileges and drops it. Safe to
// run again against a role that is already gone.
func DropRoleSQL(role, adminRole string) (string, error) {
	if err := validateIdent(role); err != nil {
		return "", err
	}
	if err := validateIdent(adminRole); err != nil {
		return "", err
	}
	r, a := QuoteIdent(role), QuoteIdent(adminRole)
	return fmt.Sprintf(`DO $$ BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = %[1]s) THEN
    ALTER ROLE %[2]s NOLOGIN;
    PERFORM pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = %[1]s;
    REASSIGN OWNED BY %[2]s TO %[3]s;
    DROP OWNED BY %[2]s;
    DROP ROLE %[2]s;
  END IF;
END $$;
`, QuoteLiteral(role), r, a), nil
}

// ListRolesSQL selects every non-system role with its attributes and live
// connection count, as CSV.
const ListRolesSQL = `SELECT r.rolname, r.rolcanlogin, r.rolsuper, r.rolcreatedb, r.rolcreaterole, r.rolreplication, r.rolconnlimit,
  COALESCE(to_char(r.rolvaliduntil AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
  (SELECT count(*) FROM pg_stat_activity a WHERE a.usename = r.rolname)
FROM pg_roles r WHERE r.rolname !~ '^pg_' ORDER BY r.rolname;`

// ParseRoles turns ListRolesSQL's CSV rows into roles.
func ParseRoles(rows [][]string) ([]Role, error) {
	out := make([]Role, 0, len(rows))
	for i, row := range rows {
		if len(row) != 9 {
			return nil, fmt.Errorf("dbaccess: role row %d has %d columns, want 9", i, len(row))
		}
		limit, err := strconv.Atoi(row[6])
		if err != nil {
			return nil, fmt.Errorf("dbaccess: role row %d connection limit: %w", i, err)
		}
		conns, err := strconv.Atoi(row[8])
		if err != nil {
			return nil, fmt.Errorf("dbaccess: role row %d connections: %w", i, err)
		}
		out = append(out, Role{
			Name: row[0], CanLogin: row[1] == "t", Superuser: row[2] == "t", CreateDB: row[3] == "t",
			CreateRole: row[4] == "t", Replication: row[5] == "t", ConnLimit: limit, ValidUntil: row[7], Connections: conns,
		})
	}
	return out, nil
}

// validateIdent bounds an identifier that is always quoted anyway: the
// platform admin role is a database name and may contain hyphens, so the
// strict role-name pattern only applies to names users choose.
func validateIdent(id string) error {
	if id == "" || len(id) > maxIdentLen {
		return fmt.Errorf("%w: %q", ErrRoleName, id)
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: control characters are not allowed", ErrRoleName)
		}
	}
	return nil
}
