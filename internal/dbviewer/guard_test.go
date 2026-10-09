package dbviewer

import (
	"errors"
	"strings"
	"testing"
)

func TestCheck_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		d       Dialect
		sql     string
		writes  bool
		wantErr error
	}{
		{"multi statement", DialectPostgres, "select 1; select 2", false, ErrMultiStatement},
		{"commit smuggle", DialectPostgres, "select 1; commit; drop table users", false, ErrMultiStatement},
		{"semicolon in string is fine", DialectPostgres, "select ';'", false, nil},
		{"semicolon in dollar quote is fine", DialectPostgres, "select $$a;b$$", false, nil},
		{"semicolon in line comment is fine", DialectPostgres, "select 1 -- ; drop table x", false, nil},
		{"semicolon in block comment is fine", DialectPostgres, "select 1 /* ; drop */", false, nil},
		{"nested comment hides semicolon", DialectPostgres, "select 1 /* a /* b */ ; drop table x */", false, nil},
		{"nested comment end then real semicolon", DialectPostgres, "select 1 /* a /* b */ c */ ; drop table x", false, ErrMultiStatement},
		{"trailing semicolon ok", DialectPostgres, "select 1;", false, nil},
		{"double trailing semicolon", DialectPostgres, "select 1;;", false, ErrMultiStatement},
		{"empty", DialectPostgres, "  -- nothing\n", false, ErrEmpty},
		{"only semicolon", DialectPostgres, ";", false, ErrEmpty},
		{"backslash meta command", DialectPostgres, "select 1 \\gexec", false, ErrBackslash},
		{"backslash bang", DialectPostgres, "\\! id", false, ErrBackslash},
		{"unicode escape ident", DialectPostgres, `select U&"pg!0072ead_file"('x') UESCAPE '!'`, false, ErrUnicodeEscape},
		{"unterminated string", DialectPostgres, "select 'abc", false, ErrUnterminated},
		{"unterminated comment", DialectPostgres, "select 1 /* abc", false, ErrUnterminated},
		{"data modifying cte", DialectPostgres, "with x as (delete from t returning *) select * from x", false, ErrDenied},
		{"cte insert", DialectPostgres, "with x as (insert into t values (1) returning *) select * from x", false, ErrDenied},
		{"select into", DialectPostgres, "select * into newt from t", false, ErrDenied},
		{"select for update", DialectPostgres, "select * from t for update", false, ErrDenied},
		{"copy", DialectPostgres, "copy t to program 'id'", false, ErrNotAllowed},
		{"copy writes mode", DialectPostgres, "copy t to program 'id'", true, ErrNotAllowed},
		{"insert", DialectPostgres, "insert into t values (1)", false, ErrNotAllowed},
		{"set role", DialectPostgres, "set role postgres", false, ErrNotAllowed},
		{"set transaction read write", DialectPostgres, "set transaction read write", false, ErrNotAllowed},
		{"commit", DialectPostgres, "commit", false, ErrNotAllowed},
		{"begin", DialectPostgres, "begin", false, ErrNotAllowed},
		{"do block", DialectPostgres, "do $$ begin perform 1; end $$", false, ErrNotAllowed},
		{"call", DialectPostgres, "call proc()", false, ErrNotAllowed},
		{"side effect fn", DialectPostgres, "select pg_terminate_backend(1)", false, ErrDenied},
		{"schema qualified fn", DialectPostgres, "select pg_catalog.pg_read_file('/etc/passwd')", false, ErrDenied},
		{"quoted fn", DialectPostgres, `select "pg_read_file"('/etc/passwd')`, false, ErrDenied},
		{"lo_import", DialectPostgres, "select lo_import('/etc/passwd')", false, ErrDenied},
		{"dblink", DialectPostgres, "select * from dblink('host=x','select 1') as t(a int)", false, ErrDenied},
		{"nextval", DialectPostgres, "select nextval('s')", false, ErrDenied},
		{"set_config", DialectPostgres, "select set_config('a','b',false)", false, ErrDenied},
		{"pg_authid", DialectPostgres, "select * from pg_authid", false, ErrDenied},
		{"advisory lock", DialectPostgres, "select pg_advisory_lock(1)", false, ErrDenied},
		{"query_to_xml", DialectPostgres, "select query_to_xml('delete from t', true, true, '')", false, ErrDenied},
		{"explain analyze write", DialectPostgres, "explain analyze delete from t", false, ErrDenied},
		{"column named update quoted ok", DialectPostgres, `select "update" from t`, false, nil},
		{"string literal with keyword ok", DialectPostgres, "select * from t where a = 'delete'", false, nil},
		{"ordinary select", DialectPostgres, "SELECT id, name FROM users WHERE id = $1 ORDER BY id LIMIT 10", false, nil},
		{"values", DialectPostgres, "values (1),(2)", false, nil},
		{"show", DialectPostgres, "show server_version", false, nil},
		{"write allowed when enabled", DialectPostgres, "update t set a = 1", true, nil},
		{"write mode still single", DialectPostgres, "update t set a = 1; drop table t", true, ErrMultiStatement},
		{"write mode alter system", DialectPostgres, "alter system set fsync = off", true, ErrDenied},
		{"write mode create extension", DialectPostgres, "create extension plpython3u", true, ErrDenied},
		{"write mode file fn", DialectPostgres, "insert into t select pg_read_file('/etc/passwd')", true, ErrDenied},
		{"write mode rejects txn control", DialectPostgres, "commit", true, ErrNotAllowed},
		{"mysql select into outfile", DialectMySQL, "select * from t into outfile '/tmp/x'", false, ErrDenied},
		{"mysql load_file", DialectMySQL, "select load_file('/etc/passwd')", false, ErrDenied},
		{"mysql exec comment", DialectMySQL, "select 1 /*!50000 union select 2 */", false, ErrExecutableComment},
		{"mariadb exec comment", DialectMariaDB, "select 1 /*M!100100 sleep(1) */", false, ErrExecutableComment},
		{"mysql hash comment hides semicolon", DialectMySQL, "select 1 # ; drop table t\n", false, nil},
		{"mysql double dash no space is not comment", DialectMySQL, "select 1 --1; drop table t", false, ErrMultiStatement},
		{"mysql double dash space is comment", DialectMySQL, "select 1 -- ; drop table t", false, nil},
		{"mysql backtick ident", DialectMySQL, "select `delete` from t", false, nil},
		{"mysql use", DialectMySQL, "use mysql", false, ErrNotAllowed},
		{"mysql system command", DialectMySQL, "system ls", false, ErrNotAllowed},
		{"mysql delimiter", DialectMySQL, "delimiter //", false, ErrNotAllowed},
		{"mysql for update", DialectMySQL, "select * from t for update", false, ErrDenied},
		{"mysql show tables", DialectMySQL, "show tables", false, nil},
		{"control char", DialectPostgres, "select 1\x00", false, ErrControlChar},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Check(tc.d, tc.sql, tc.writes)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Check(%q) error = %v, want nil", tc.sql, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Check(%q) error = %v, want %v", tc.sql, err, tc.wantErr)
			}
		})
	}
}

func TestCheck_StatementShape(t *testing.T) {
	st, err := Check(DialectPostgres, "  SELECT a FROM t WHERE b = 'x';  ", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.SQL != "SELECT a FROM t WHERE b = 'x'" {
		t.Errorf("SQL = %q", st.SQL)
	}
	if st.Kind != KindQuery || st.First != "select" {
		t.Errorf("kind/first = %s/%s", st.Kind, st.First)
	}

	ex, err := Check(DialectPostgres, "explain select 1", false)
	if err != nil || ex.Kind != KindExplain {
		t.Errorf("explain: %+v %v", ex, err)
	}
	w, err := Check(DialectPostgres, "delete from t", true)
	if err != nil || w.Kind != KindWrite {
		t.Errorf("write: %+v %v", w, err)
	}
}

func TestFingerprint_IgnoresLiteralsAndCase(t *testing.T) {
	a, _ := Check(DialectPostgres, "SELECT * FROM users WHERE id = 5 AND name = 'bob'", false)
	b, _ := Check(DialectPostgres, "select * from users where id = 99 and name = 'alice'", false)
	c, _ := Check(DialectPostgres, "select * from orders where id = 5", false)
	if a.Fingerprint != b.Fingerprint {
		t.Errorf("same shape must share a fingerprint: %s vs %s", a.Fingerprint, b.Fingerprint)
	}
	if a.Fingerprint == c.Fingerprint {
		t.Errorf("different statements must differ")
	}
	if strings.Contains(a.Fingerprint, "bob") || len(a.Fingerprint) != 16 {
		t.Errorf("fingerprint = %q", a.Fingerprint)
	}
}
