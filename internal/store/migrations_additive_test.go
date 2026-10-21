package store

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

//go:embed migrations_baseline.txt
var migrationsBaselineRaw string

const compatBreakMarker = "-- compat-break:"

var destructiveRules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"DROP TABLE", regexp.MustCompile(`(?i)\bDROP\s+TABLE\b`)},
	{"DROP COLUMN", regexp.MustCompile(`(?i)\bDROP\s+COLUMN\b`)},
	{"RENAME COLUMN", regexp.MustCompile(`(?i)\bRENAME\s+COLUMN\b`)},
	{"RENAME TO", regexp.MustCompile(`(?i)\bRENAME\s+TO\b`)},
	{"ALTER COLUMN", regexp.MustCompile(`(?i)\bALTER\s+COLUMN\b`)},
	{"ADD COLUMN NOT NULL without DEFAULT", regexp.MustCompile(`(?i)\bADD\s+(?:COLUMN\s+)?\w+[^;]*\bNOT\s+NULL\b`)},
}

var (
	defaultRe         = regexp.MustCompile(`(?i)\bDEFAULT\b`)
	createTableNameRe = regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z0-9_]+)`)
)

type compatFinding struct {
	line   int
	rule   string
	text   string
	reason string
}

// destructiveStatements scans one migration. Tables created in the same file may be dropped or renamed freely.
func destructiveStatements(sql string) (violations, overridden []compatFinding) {
	local := map[string]bool{}
	for _, m := range createTableNameRe.FindAllStringSubmatch(sql, -1) {
		local[strings.ToLower(m[1])] = true
	}
	for i, raw := range strings.Split(sql, "\n") {
		code, comment, _ := strings.Cut(raw, "--")
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		for _, rule := range destructiveRules {
			if !rule.re.MatchString(code) {
				continue
			}
			if rule.name == "ADD COLUMN NOT NULL without DEFAULT" && defaultRe.MatchString(code) {
				continue
			}
			if mentionsLocalTable(code, local) {
				continue
			}
			f := compatFinding{line: i + 1, rule: rule.name, text: code}
			if reason, ok := strings.CutPrefix(strings.TrimSpace("--"+comment), compatBreakMarker); ok && strings.TrimSpace(reason) != "" {
				f.reason = strings.TrimSpace(reason)
				overridden = append(overridden, f)
			} else {
				violations = append(violations, f)
			}
			break
		}
	}
	return violations, overridden
}

func mentionsLocalTable(code string, local map[string]bool) bool {
	fields := strings.FieldsFunc(strings.ToLower(code), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '(' || r == ')' || r == ',' || r == ';' || r == '"' || r == '`'
	})
	for i, f := range fields {
		if (f == "table" || f == "exists") && i+1 < len(fields) && local[fields[i+1]] {
			return true
		}
	}
	return false
}

func TestMigrationsAdditive(t *testing.T) {
	baseline, err := strconv.Atoi(strings.TrimSpace(migrationsBaselineRaw))
	if err != nil || baseline <= 0 {
		t.Fatalf("migrations_baseline.txt must hold one positive migration number: %v", err)
	}
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var reasons []string
	for _, m := range migs {
		if m.version <= baseline {
			continue
		}
		bad, ok := destructiveStatements(m.sql)
		for _, f := range bad {
			t.Errorf("migration %04d_%s line %d: %s breaks upgrades from the last release (%s); make it additive or add %q on the line", m.version, m.name, f.line, f.rule, f.text, compatBreakMarker+" <reason>")
		}
		for _, f := range ok {
			reasons = append(reasons, fmt.Sprintf("%04d_%s line %d %s: %s", m.version, m.name, f.line, f.rule, f.reason))
		}
	}
	for _, r := range reasons {
		t.Logf("compat-break override: %s", r)
	}
}

func TestDestructiveStatements(t *testing.T) {
	cases := []struct {
		name       string
		sql        string
		violations int
		overridden int
	}{
		{"create table", "CREATE TABLE a (id TEXT);", 0, 0},
		{"add nullable column", "ALTER TABLE a ADD COLUMN x TEXT;", 0, 0},
		{"add not null with default", "ALTER TABLE a ADD COLUMN x TEXT NOT NULL DEFAULT '';", 0, 0},
		{"add not null no default", "ALTER TABLE a ADD COLUMN x TEXT NOT NULL;", 1, 0},
		{"drop table", "DROP TABLE a;", 1, 0},
		{"drop table if exists lower", "drop table if exists a;", 1, 0},
		{"drop column", "ALTER TABLE a DROP COLUMN x;", 1, 0},
		{"rename column", "ALTER TABLE a RENAME COLUMN x TO y;", 1, 0},
		{"rename table", "ALTER TABLE a RENAME TO b;", 1, 0},
		{"alter column", "ALTER TABLE a ALTER COLUMN x TYPE INT;", 1, 0},
		{"override with reason", "DROP TABLE a; -- compat-break: table was never used", 0, 1},
		{"override without reason", "DROP TABLE a; -- compat-break:", 1, 0},
		{"comment only mention", "-- we used to DROP TABLE a", 0, 0},
		{"drop of table made here", "CREATE TABLE b_new (id TEXT);\nDROP TABLE IF EXISTS b_new;", 0, 0},
		{"rebuild of existing table", "CREATE TABLE a_new (id TEXT);\nDROP TABLE a;\nALTER TABLE a_new RENAME TO a;", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, o := destructiveStatements(tc.sql)
			if len(v) != tc.violations || len(o) != tc.overridden {
				t.Fatalf("got %d violations, %d overridden; want %d, %d", len(v), len(o), tc.violations, tc.overridden)
			}
		})
	}
}
