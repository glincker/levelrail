package database

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestIsolatedRoleName(t *testing.T) {
	cases := []struct {
		name, previewName, sourceKey, want string
	}{
		{"simple", "myapp-pr-12", "main", "pv_myapp_pr_12_main"},
		{"uppercase and dashes sanitized", "MyApp-PR-3", "Main-DB", "pv_myapp_pr_3_main_db"},
		{"deterministic", "myapp-pr-12", "main", "pv_myapp_pr_12_main"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsolatedRoleName(tc.previewName, tc.sourceKey)
			if got != tc.want {
				t.Fatalf("IsolatedRoleName(%q, %q) = %q, want %q", tc.previewName, tc.sourceKey, got, tc.want)
			}
			if len(got) > maxIsolatedRoleNameLen {
				t.Fatalf("role name %q is %d bytes, want <= %d", got, len(got), maxIsolatedRoleNameLen)
			}
		})
	}
}

func TestIsolatedRoleNameTruncatesLongInput(t *testing.T) {
	got := IsolatedRoleName("a-very-long-preview-environment-name-from-a-branch", "some-source-key")
	if len(got) > maxIsolatedRoleNameLen {
		t.Fatalf("role name %q is %d bytes, want <= %d", got, len(got), maxIsolatedRoleNameLen)
	}
}

func TestGenerateIsolatedRolePassword(t *testing.T) {
	a, err := GenerateIsolatedRolePassword()
	if err != nil {
		t.Fatalf("GenerateIsolatedRolePassword: %v", err)
	}
	b, err := GenerateIsolatedRolePassword()
	if err != nil {
		t.Fatalf("GenerateIsolatedRolePassword: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("expected a non-empty password")
	}
	if a == b {
		t.Fatal("expected two calls to produce different passwords")
	}
}

func TestCreateIsolatedRoleSQLContainsExpectedStatements(t *testing.T) {
	sql := CreateIsolatedRoleSQL(`pv_app_main`, `main`, `s3cr3t`)
	for _, want := range []string{
		`CREATE ROLE "pv_app_main" LOGIN PASSWORD 's3cr3t';`,
		`ALTER ROLE "pv_app_main" PASSWORD 's3cr3t';`,
		`GRANT CONNECT ON DATABASE "main" TO "pv_app_main";`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "pv_app_main";`,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "pv_app_main";`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("CreateIsolatedRoleSQL missing %q\ngot:\n%s", want, sql)
		}
	}
}

func TestDropIsolatedRoleSQLContainsExpectedStatements(t *testing.T) {
	sql := DropIsolatedRoleSQL(`pv_app_main`, `main`)
	for _, want := range []string{
		`REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM "pv_app_main";`,
		`REVOKE CONNECT ON DATABASE "main" FROM "pv_app_main";`,
		`DROP ROLE IF EXISTS "pv_app_main";`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("DropIsolatedRoleSQL missing %q\ngot:\n%s", want, sql)
		}
	}
}

func TestCreateIsolatedRedisACLCommand(t *testing.T) {
	cases := []struct {
		name     string
		username string
		password string
	}{
		{"simple", "pv_app_main", "s3cr3t"},
		{"different user and password", "pv_other_cache", "another-pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CreateIsolatedRedisACLCommand(tc.username, tc.password)
			want := []string{
				"redis-cli", "-p", strconv.Itoa(redisContainerPort),
				"ACL", "SETUSER", tc.username,
				"reset", "on", ">" + tc.password,
				"resetkeys", "~" + tc.username + ":*",
				"resetchannels",
				"+@all", "-@admin", "-@dangerous", "-@scripting",
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("CreateIsolatedRedisACLCommand(%q, %q) = %v, want %v", tc.username, tc.password, got, want)
			}
		})
	}
}

// TestCreateIsolatedRedisACLCommand_RestrictsCommands checks the
// command-restriction contract directly: the generated ACL grants every
// category (+@all) then explicitly strips the three categories a
// preview's own key-prefix-scoped user must never reach, and scopes
// keys to its own "username:*" prefix rather than "*".
func TestCreateIsolatedRedisACLCommand_RestrictsCommands(t *testing.T) {
	got := CreateIsolatedRedisACLCommand("pv_app_main", "s3cr3t")

	for _, want := range []string{"+@all", "-@admin", "-@dangerous", "-@scripting"} {
		if !containsArg(got, want) {
			t.Errorf("CreateIsolatedRedisACLCommand missing %q in %v", want, got)
		}
	}
	if containsArg(got, "~*") {
		t.Errorf("CreateIsolatedRedisACLCommand grants unscoped key pattern ~* in %v, want only ~pv_app_main:*", got)
	}
	if !containsArg(got, "~pv_app_main:*") {
		t.Errorf("CreateIsolatedRedisACLCommand missing scoped key pattern in %v", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestDropIsolatedRedisACLCommand(t *testing.T) {
	got := DropIsolatedRedisACLCommand("pv_app_main")
	want := []string{"redis-cli", "-p", strconv.Itoa(redisContainerPort), "ACL", "DELUSER", "pv_app_main"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DropIsolatedRedisACLCommand(%q) = %v, want %v", "pv_app_main", got, want)
	}
}

func TestQuoteIdentifierAndLiteralEscaping(t *testing.T) {
	if got, want := quoteIdentifier(`weird"name`), `"weird""name"`; got != want {
		t.Fatalf("quoteIdentifier = %q, want %q", got, want)
	}
	if got, want := quoteLiteral(`weird'pass`), `'weird''pass'`; got != want {
		t.Fatalf("quoteLiteral = %q, want %q", got, want)
	}
}
