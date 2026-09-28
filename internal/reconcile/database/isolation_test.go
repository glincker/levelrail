package database

import (
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

func TestQuoteIdentifierAndLiteralEscaping(t *testing.T) {
	if got, want := quoteIdentifier(`weird"name`), `"weird""name"`; got != want {
		t.Fatalf("quoteIdentifier = %q, want %q", got, want)
	}
	if got, want := quoteLiteral(`weird'pass`), `'weird''pass'`; got != want {
		t.Fatalf("quoteLiteral = %q, want %q", got, want)
	}
}
