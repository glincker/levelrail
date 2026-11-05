package selfupgrade

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// PlanSourceRunningBinary says the list comes from the migrations embedded in
// the running binary, because a release manifest carries only its schema number.
const PlanSourceRunningBinary = "running_binary"

// MigrationPlan is the ordered migrations an upgrade from schema db to target
// will run, and where the list came from.
type MigrationPlan struct {
	Pending []store.PendingMigration `json:"pending"`
	Source  string                   `json:"source"`
	// Note is set when the running binary cannot name every migration up to target.
	Note string `json:"note,omitempty"`
}

// PlanMigrations lists the embedded migrations in (db, target]. A target newer
// than this binary is only partly listable, and the note says so.
func PlanMigrations(db, target int) (MigrationPlan, error) {
	list, err := store.PendingMigrationList(db, target)
	if err != nil {
		return MigrationPlan{}, err
	}
	p := MigrationPlan{Pending: list, Source: PlanSourceRunningBinary}
	newest, err := store.MaxSchemaVersion()
	if err == nil && target > newest {
		p.Note = fmt.Sprintf("the target release has migrations up to %d, this binary knows %d; the rest are listed after the upgrade", target, newest)
	}
	return p, nil
}
