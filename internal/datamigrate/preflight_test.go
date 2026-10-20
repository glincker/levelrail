package datamigrate

import (
	"regexp"
	"strings"
	"testing"
)

func severityOf(res PreflightResult, id string) string {
	for _, c := range res.Checks {
		if c.ID == id {
			return c.Severity
		}
	}
	return "missing"
}

func TestPreflightRules(t *testing.T) {
	const gib = int64(1) << 30
	base := PreflightInput{
		Engine: EnginePostgres, DB: DatabaseInfo{Name: "app", SizeBytes: 100 << 20}, SourceMajor: 16,
		TargetName: "app", TargetVersion: "16", FreeBytes: 10 * gib, DiskMargin: 1.5, MBPerSecond: 20,
	}
	tests := []struct {
		name    string
		mut     func(*PreflightInput)
		check   string
		want    string
		blocked bool
	}{
		{"plain ok", func(*PreflightInput) {}, "extensions", SeverityOK, false},
		{"stock extension", func(i *PreflightInput) { i.DB.Extensions = []string{"pg_trgm", "citext"} }, "extensions", SeverityOK, false},
		{"vector is served by the pgvector variant", func(i *PreflightInput) { i.DB.Extensions = []string{"vector"} }, "extensions", SeverityOK, false},
		{"vector with an unsupported extension blocks", func(i *PreflightInput) { i.DB.Extensions = []string{"vector", "postgis"} }, "extensions", SeverityBlock, true},
		{"unknown extension blocks", func(i *PreflightInput) { i.DB.Extensions = []string{"postgis"} }, "extensions", SeverityBlock, true},
		{"older target blocks", func(i *PreflightInput) { i.TargetVersion = "15" }, "version", SeverityBlock, true},
		{"newer target warns", func(i *PreflightInput) { i.TargetVersion = "17" }, "version", SeverityWarn, false},
		{"disk too small", func(i *PreflightInput) { i.FreeBytes = 50 << 20 }, "disk", SeverityBlock, true},
		{"disk unknown warns", func(i *PreflightInput) { i.FreeBytes = -1 }, "disk", SeverityWarn, false},
		{"foreign collision blocks", func(i *PreflightInput) { i.TargetExists = true }, "collision", SeverityBlock, true},
		{"own collision warns", func(i *PreflightInput) { i.TargetExists, i.OwnTarget = true, true }, "collision", SeverityWarn, false},
		{"bad name blocks", func(i *PreflightInput) { i.TargetName = "My DB" }, "name", SeverityBlock, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mut(&in)
			res := Preflight(in)
			if got := severityOf(res, tc.check); got != tc.want {
				t.Fatalf("%s severity = %s, want %s (%+v)", tc.check, got, tc.want, res.Checks)
			}
			if res.Blocked != tc.blocked {
				t.Fatalf("blocked = %v, want %v", res.Blocked, tc.blocked)
			}
		})
	}
}

func TestRequiredVariantNeverDropsExtensions(t *testing.T) {
	suffix, unsupported := requiredVariant([]string{"pg_trgm", "vector", "postgis"})
	if suffix != PgvectorVersionSuffix || len(unsupported) != 1 || unsupported[0] != "postgis" {
		t.Fatalf("got %q %v", suffix, unsupported)
	}
	if got := variantVersion(17, suffix); got != "17-pgvector" {
		t.Fatalf("variantVersion = %q", got)
	}
	if s, u := requiredVariant([]string{"citext"}); s != "" || len(u) != 0 {
		t.Fatalf("stock extension changed variant: %q %v", s, u)
	}
}

func TestDeriveTargetNames(t *testing.T) {
	got := DeriveTargetNames([]string{"My_App", "my-app", "2fast", "Données", "app"}, map[string]bool{"app": true})
	want := map[string]string{"My_App": "my-app", "my-app": "my-app-2", "2fast": "db-2fast", "Données": "donn-es", "app": "app-2"}
	for src, w := range want {
		if got[src] != w {
			t.Errorf("%q -> %q, want %q", src, got[src], w)
		}
		if !ValidTargetName(got[src]) {
			t.Errorf("%q derived invalid name %q", src, got[src])
		}
	}
}

func TestParseInventory(t *testing.T) {
	inv := ParseInventory(EnginePostgres, "version|16.4\ndb|a|b|1048576\ndb|postgres|7000000\nnoise\n")
	if inv.Major != 16 || len(inv.Databases) != 2 || inv.Databases[0].Name != "a|b" || inv.Databases[0].SizeBytes != 1048576 {
		t.Fatalf("unexpected inventory %+v", inv)
	}
	my := ParseInventory(EngineMySQL, "version|8.0.36\ndb|shop|2048|12\n")
	if my.Databases[0].Tables != 12 || my.Databases[0].SizeBytes != 2048 {
		t.Fatalf("unexpected mysql inventory %+v", my)
	}
}

var writeStatement = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|alter|truncate|create|grant|revoke|vacuum|reindex|flushall|flushdb)\b|--clean|--add-drop|pg_restore|dropdb|mongorestore|-c\s+['"]?(drop|delete)`)

// The helper must never be able to modify the source: every command builder
// that targets it is read-only SQL, a dump tool, or a catalog query.
func TestSourceCommandsAreReadOnly(t *testing.T) {
	var scripts []string
	for _, engine := range []string{EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis} {
		if c, err := DumpCommand(engine); err == nil {
			scripts = append(scripts, c[2])
		}
		if c, err := SourceCountCommand(engine); err == nil {
			scripts = append(scripts, c[2])
		}
		if c, err := InventoryListCommand(engine); err == nil {
			scripts = append(scripts, c[2])
		}
	}
	scripts = append(scripts, InventoryDatabaseCommand("x")[2], DiskFreeCommand()[2])
	for _, s := range scripts {
		if m := writeStatement.FindString(s); m != "" {
			t.Errorf("command can write or is destructive (%q):\n%s", m, s)
		}
	}
	for _, engine := range []string{EnginePostgres} {
		for _, build := range []func(string) ([]string, error){DumpCommand, SourceCountCommand, InventoryListCommand} {
			c, err := build(engine)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(c[2], "default_transaction_read_only=on") {
				t.Errorf("postgres command lacks the read-only session option:\n%s", c[2])
			}
		}
	}
	if !strings.Contains(InventoryDatabaseCommand("x")[2], "default_transaction_read_only=on") {
		t.Error("per-database inventory lacks the read-only session option")
	}
}
