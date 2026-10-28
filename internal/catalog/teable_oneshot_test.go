package catalog

import (
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// The migration runs once per deploy and teable waits for it to exit 0.
func TestTeableMigrateRunsToCompletion(t *testing.T) {
	tpl, ok := TemplateByID("teable")
	if !ok {
		t.Fatal("teable template missing")
	}
	f, err := compose.Parse([]byte(tpl.Compose))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	svcs, _, err := compose.ToDesiredServices("teable", f)
	if err != nil {
		t.Fatalf("ToDesiredServices: %v", err)
	}
	found := 0
	for _, s := range svcs {
		switch s.Name {
		case "teable-teable-db-migrate":
			found++
			if !s.RunsToCompletion() {
				t.Errorf("teable-db-migrate RunsToCompletion = false, want true")
			}
		case "teable-teable":
			found++
			if s.RunsToCompletion() || !slices.Contains(s.DependsOnCompleted(), "teable-db-migrate") {
				t.Errorf("teable: RunsToCompletion = %v, DependsOnCompleted = %v, want a long-running service waiting on the migration", s.RunsToCompletion(), s.DependsOnCompleted())
			}
		}
	}
	if found != 2 {
		t.Fatalf("found %d of the 2 teable services", found)
	}
}
