package authengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
)

// librarySlices lists, in order, the store migrations that embed the library
// schema and how many library migrations each one carries.
var librarySlices = []struct {
	file  string
	count int
}{
	{"../store/migrations/0372_authengine_tables.sql", 9},
	{"../store/migrations/0750_authengine_enterprise.sql", 7},
}

func TestMigrationMatchesLibrary(t *testing.T) {
	migs, err := sqlitestore.RenderMigrations()
	if err != nil {
		t.Fatal(err)
	}
	next := 0
	for _, s := range librarySlices {
		got, err := os.ReadFile(filepath.FromSlash(s.file))
		if err != nil {
			t.Fatal(err)
		}
		if next+s.count > len(migs) {
			t.Fatalf("%s claims more library migrations than the library has", s.file)
		}
		var want strings.Builder
		for _, m := range migs[next : next+s.count] {
			want.WriteString(m.SQL)
			want.WriteString("\n")
		}
		if !strings.HasPrefix(string(got), want.String()) {
			t.Fatalf("%s drifted from library migrations %s to %s; regenerate its library section",
				s.file, migs[next].Version, migs[next+s.count-1].Version)
		}
		next += s.count
	}
	if next != len(migs) {
		t.Fatalf("library migration %s is not embedded in any store migration", migs[next].Version)
	}
	first, err := os.ReadFile(filepath.FromSlash(librarySlices[0].file))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"authengine_user_map", "authengine_token_map"} {
		if !strings.Contains(string(first), table) {
			t.Fatalf("migration is missing %s", table)
		}
	}
}
