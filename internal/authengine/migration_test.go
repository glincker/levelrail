package authengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
)

const migrationFile = "../store/migrations/0292_authengine_tables.sql"

func TestMigrationMatchesLibrary(t *testing.T) {
	got, err := os.ReadFile(filepath.FromSlash(migrationFile))
	if err != nil {
		t.Fatal(err)
	}
	migs, err := sqlitestore.RenderMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for _, m := range migs {
		want.WriteString(m.SQL)
		want.WriteString("\n")
	}
	if !strings.HasPrefix(string(got), want.String()) {
		t.Fatal("0292_authengine_tables.sql drifted from the library migrations; regenerate its library section")
	}
	rest := strings.TrimPrefix(string(got), want.String())
	for _, table := range []string{"authengine_user_map", "authengine_token_map"} {
		if !strings.Contains(rest, table) {
			t.Fatalf("migration is missing %s", table)
		}
	}
}
