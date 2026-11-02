package selfupgrade

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPlanMigrationsListsNumberAndName(t *testing.T) {
	newest, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	p, err := PlanMigrations(newest-2, newest)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Pending) == 0 || len(p.Pending) > 2 || p.Pending[len(p.Pending)-1].Version != newest || p.Pending[0].Name == "" {
		t.Fatalf("pending = %+v", p.Pending)
	}
	if p.Source != PlanSourceRunningBinary || p.Note != "" {
		t.Fatalf("source/note = %q %q", p.Source, p.Note)
	}
}

func TestPlanMigrationsNotesTargetBeyondBinary(t *testing.T) {
	newest, _ := store.MaxSchemaVersion()
	p, err := PlanMigrations(newest, newest+5)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Pending) != 0 || p.Note == "" {
		t.Fatalf("plan = %+v", p)
	}
}
