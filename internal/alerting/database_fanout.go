package alerting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// KindBackupFailed is platform-wide: it fires while the newest backup attempt of any scheduled database failed.
const KindBackupFailed Kind = "backup_failed"

// DatabaseLister lists every managed database; *store.DB satisfies it.
type DatabaseLister interface {
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
}

// EvaluateDatabaseRuleAll runs a database rule against its named database, or against every managed database when the rule names none (the seeded defaults).
func EvaluateDatabaseRuleAll(ctx context.Context, health DatabaseHealthSource, backups DatabaseBackupSource, r Rule, now time.Time) (Rule, string, error) {
	if r.BackupDatabaseName != "" {
		return EvaluateDatabaseRule(ctx, health, backups, r, now)
	}
	quiet := quietDatabaseRule(r)
	quiet.LastEvaluatedAt = &now
	lister, ok := backups.(DatabaseLister)
	if !ok {
		return quiet, "", nil
	}
	dbs, err := lister.ListDesiredDatabases(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: list databases: %w", r.ID, err)
	}
	perDB := r
	if r.Kind == KindBackupFailed {
		perDB.Kind = KindDatabaseBackupFailing
	}
	needsSchedule := perDB.Kind == KindDatabaseBackupStale || perDB.Kind == KindDatabaseBackupFailing

	best := quiet
	var notices []string
	var errs []error
	for _, d := range dbs {
		if d.Suspended || (needsSchedule && d.BackupSchedule == "") {
			continue
		}
		perDB.BackupDatabaseName = d.Name
		out, notice, err := EvaluateDatabaseRule(ctx, health, backups, perDB, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		switch {
		case out.Firing:
			notices = append(notices, notice)
			if !best.Firing {
				best = out
			}
		case out.PendingSince != nil && !best.Firing && best.PendingSince == nil:
			best = out
		}
	}
	res := r
	res.LastEvaluatedAt = &now
	res.PendingSince, res.Firing, res.FiringSince, res.LastValue = best.PendingSince, best.Firing, best.FiringSince, best.LastValue
	if len(notices) == 0 && len(errs) > 0 {
		return r, "", errors.Join(errs...)
	}
	return res, strings.Join(notices, "; "), nil
}
