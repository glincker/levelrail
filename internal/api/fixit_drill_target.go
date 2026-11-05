package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	drillRepointFixID   = "backup.drill.repoint"
	drillStaleRefFixID  = "backup.drill.stale_reference"
	drillTargetCheckID  = "backup_drill_target"
	drillAuditRepointed = "backup.drill_target.repointed"
	drillAuditRemoved   = "backup.drill_target.reference_removed"
)

// drillTargetDeps are the seams the stale drill target fixes use.
type drillTargetDeps struct {
	// Failed returns the latest failed drill of every resource.
	Failed func(ctx context.Context) ([]store.BackupDrill, error)
	// Targets returns the backup targets that exist now.
	Targets func(ctx context.Context) ([]store.BackupTarget, error)
	// Repoint moves the resource's backup schedule off the missing target. It
	// returns an error whenever it cannot prove the schedule is safe to leave;
	// false with no error means the schedule provably does not use the target.
	Repoint func(ctx context.Context, d store.BackupDrill, targetID string) (bool, error)
	// Drop deletes exactly these failed drill rows.
	Drop func(ctx context.Context, drillIDs []string) (int, error)
	// Audit records what a fix changed.
	Audit func(ctx context.Context, action string, detail map[string]string)
	// CLI is the command name used in hints.
	CLI string
}

type danglingDrills struct {
	drills  []store.BackupDrill
	targets []store.BackupTarget
}

// dangling keeps only drills whose target is really gone, so a stale error text never triggers a change.
func (d drillTargetDeps) dangling(ctx context.Context) (danglingDrills, error) {
	failed, err := d.Failed(ctx)
	if err != nil {
		return danglingDrills{}, err
	}
	targets, err := d.Targets(ctx)
	if err != nil {
		return danglingDrills{}, err
	}
	live := map[string]bool{}
	for _, t := range targets {
		live[t.ID] = true
	}
	return danglingDrills{
		drills:  backup.SummarizeDrillFailures(failed, func(id string) bool { return live[id] }).Dangling,
		targets: targets,
	}, nil
}

func (g danglingDrills) changes() []fixit.Change {
	out := make([]fixit.Change, 0, len(g.drills))
	for _, d := range g.drills {
		out = append(out, fixit.Change{Label: backup.DrillResourceLabel(d), Detail: "drill points at missing target " + d.TargetID})
	}
	return out
}

func (g danglingDrills) ids() []string {
	ids := make([]string, 0, len(g.drills))
	for _, d := range g.drills {
		ids = append(ids, d.ID)
	}
	sort.Strings(ids)
	return ids
}

func (g danglingDrills) detail() string {
	return backup.DrillFailureSummary{Dangling: g.drills}.Detail()
}

func (d drillTargetDeps) drop(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := d.Drop(ctx, ids)
	if err != nil {
		return fmt.Errorf("remove stale drill reference: %w", err)
	}
	d.Audit(ctx, drillAuditRemoved, map[string]string{"drill_ids": strings.Join(ids, ","), "removed": fmt.Sprint(n)})
	return nil
}

// drillStaleRefFix removes the stale failed drill rows of a deleted target when no target exists to move to.
// Only the dangling rows go: real failure rows stay. The deleted rows cannot be restored.
func drillStaleRefFix(d drillTargetDeps) fixit.Fix {
	return fixit.Fix{
		ID: drillStaleRefFixID, CheckID: drillTargetCheckID, Subject: "backup", Kind: fixit.KindAuto,
		Title: "Remove the stale drill reference",
		Why:   "The drill points at a backup target that no longer exists and there is no other target to move it to. This deletes only those stale drill rows, which cannot be restored. No backup or target is deleted.",
		Risk:  fixit.RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			g, err := d.dangling(ctx)
			if err != nil {
				return fixit.Applicability{}, err
			}
			if len(g.drills) == 0 || len(g.targets) > 0 {
				return fixit.Applicability{Detail: "no stale drill reference to remove", Changes: []fixit.Change{}}, nil
			}
			return fixit.Applicability{Failing: true, Detail: g.detail(), Changes: g.changes()}, nil
		},
		Run: func(ctx context.Context, _ fixit.Input) error {
			g, err := d.dangling(ctx)
			if err != nil {
				return err
			}
			if len(g.drills) == 0 || len(g.targets) > 0 {
				return fixit.ErrCannotFixHere
			}
			return d.drop(ctx, g.ids())
		},
	}
}

// drillRepointFix moves each affected schedule to a target the user picked. It changes which destination
// holds future backups, so it is auto only once the click supplies target_id, like the other param fixes.
func drillRepointFix(d drillTargetDeps) fixit.Fix {
	return fixit.Fix{
		ID: drillRepointFixID, CheckID: drillTargetCheckID, Subject: "backup", Kind: fixit.KindAuto,
		Title: "Point the drill at a current backup target",
		Why:   "The drill points at a backup target that no longer exists. Choosing a current target moves the resource's backup schedule there and removes the stale drill rows, which cannot be restored. Existing backups and targets are never deleted.",
		Risk:  fixit.RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			g, err := d.dangling(ctx)
			if err != nil {
				return fixit.Applicability{}, err
			}
			if len(g.drills) == 0 || len(g.targets) == 0 {
				return fixit.Applicability{Detail: "no stale drill target to repoint", Changes: []fixit.Change{}}, nil
			}
			opts := make([]fixit.Option, 0, len(g.targets))
			for _, t := range g.targets {
				opts = append(opts, fixit.Option{Value: t.ID, Label: t.Name})
			}
			p := &fixit.Param{Name: "target_id", Options: opts}
			if len(opts) == 1 {
				p.Default = opts[0].Value
			}
			return fixit.Applicability{
				Failing: true, Detail: g.detail(), Param: p, Changes: g.changes(),
				Reason: "Choose the backup target the affected resources should use, then apply. " + d.CLI + " backup-targets list shows them.",
			}, nil
		},
		Run: func(ctx context.Context, in fixit.Input) error {
			g, err := d.dangling(ctx)
			if err != nil {
				return err
			}
			target, err := chooseDrillTarget(g.targets, in.Params["target_id"])
			if err != nil {
				return err
			}
			if len(g.drills) == 0 {
				return fixit.ErrCannotFixHere
			}
			for _, dr := range g.drills {
				moved, err := d.Repoint(ctx, dr, target)
				if err != nil {
					return fmt.Errorf("repoint %s: %w", backup.DrillResourceLabel(dr), err)
				}
				if moved {
					d.Audit(ctx, drillAuditRepointed, map[string]string{"resource": backup.DrillResourceLabel(dr), "from": dr.TargetID, "to": target})
				}
				if err := d.drop(ctx, []string{dr.ID}); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func chooseDrillTarget(targets []store.BackupTarget, want string) (string, error) {
	for _, t := range targets {
		if want != "" && t.ID == want {
			return want, nil
		}
	}
	return "", errors.New("choose one of the existing backup targets")
}

type drillScheduleSetter interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	SetDatabaseBackupSchedule(ctx context.Context, name, targetID, schedule string, retain, retainDays int) error
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (rt *Router) drillTargetDeps() drillTargetDeps {
	return drillTargetDeps{
		Failed:  rt.latestFailedDrills,
		Targets: rt.liveBackupTargets,
		Repoint: rt.repointDrillResource,
		Drop:    rt.dropDrills,
		Audit:   rt.auditDrillFix,
		CLI:     rt.cliName(),
	}
}

func (rt *Router) drillTargetFixes() []fixit.Fix {
	if rt.backupProtectionStore == nil || rt.backupTargets == nil {
		return nil
	}
	d := rt.drillTargetDeps()
	return []fixit.Fix{drillStaleRefFix(d), drillRepointFix(d)}
}

func (rt *Router) liveBackupTargets(ctx context.Context) ([]store.BackupTarget, error) {
	if rt.backupTargets == nil {
		return nil, nil
	}
	return rt.backupTargets.ListBackupTargets(ctx)
}

// targetExists reports whether a backup target id is live; unknown stores count as live so nothing is downgraded.
func (rt *Router) targetExists(ctx context.Context) (func(string) bool, error) {
	if rt.backupTargets == nil {
		return func(string) bool { return true }, nil
	}
	targets, err := rt.backupTargets.ListBackupTargets(ctx)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, t := range targets {
		live[t.ID] = true
	}
	return func(id string) bool { return live[id] }, nil
}

// latestFailedDrills returns the newest finished drill of each resource when it failed.
func (rt *Router) latestFailedDrills(ctx context.Context) ([]store.BackupDrill, error) {
	if rt.backupProtectionStore == nil {
		return nil, nil
	}
	drills, err := rt.backupProtectionStore.ListBackupDrills(ctx, "", "", "", 1000)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var failed []store.BackupDrill
	for _, d := range drills {
		if d.Status == store.DrillStatusRunning {
			continue
		}
		key := d.ResourceKind + "/" + d.DatabaseName + "/" + d.ServiceName + "/" + d.VolumeName
		if seen[key] {
			continue
		}
		seen[key] = true
		if d.Status == store.DrillStatusFailed {
			failed = append(failed, d)
		}
	}
	return failed, nil
}

var errCannotRepoint = errors.New("this resource's backup schedule cannot be read or changed here")

func (rt *Router) repointDrillResource(ctx context.Context, d store.BackupDrill, targetID string) (bool, error) {
	if d.ResourceKind == store.BackupResourceKindVolume {
		if rt.serviceVolumeBackupSchedule == nil {
			return false, errCannotRepoint
		}
		cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(ctx, d.ServiceName, d.VolumeName)
		if errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if cfg.BackupTargetID != d.TargetID {
			return false, nil
		}
		err = rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(ctx, d.ServiceName, d.VolumeName, targetID, cfg.BackupSchedule, cfg.BackupRetain, cfg.BackupRetainDays, cfg.SqlitePath)
		return err == nil, err
	}
	dbs, ok := rt.databases.(drillScheduleSetter)
	if !ok {
		return false, errCannotRepoint
	}
	cur, err := dbs.GetDesiredDatabase(ctx, d.DatabaseName)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if cur == nil || cur.BackupTargetID != d.TargetID {
		return false, nil
	}
	err = dbs.SetDatabaseBackupSchedule(ctx, d.DatabaseName, targetID, cur.BackupSchedule, cur.BackupRetain, cur.BackupRetainDays)
	return err == nil, err
}

// dropDrills deletes the given failed rows one by one, never by target, so older real failures stay.
func (rt *Router) dropDrills(ctx context.Context, ids []string) (int, error) {
	db, ok := rt.fixit.store.(execer)
	if !ok {
		return 0, fixit.ErrCannotFixHere
	}
	total := 0
	for _, id := range ids {
		res, err := db.ExecContext(ctx, `DELETE FROM backup_drills WHERE id = ? AND status = ?`, id, store.DrillStatusFailed)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

func auditDetailJSON(detail map[string]string) string {
	b, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(b)
}

type fixitActorKey struct{}

// fixitActor is who clicked Apply, carried to the fix so its audit row names a real principal.
type fixitActor struct{ typ, id, name string }

func withFixitActor(ctx context.Context, principalType, id, name string) context.Context {
	typ := auditActorSession
	if principalType == store.PrincipalTypeToken {
		typ = "token"
	}
	return context.WithValue(ctx, fixitActorKey{}, fixitActor{typ: typ, id: id, name: name})
}

func (rt *Router) auditDrillFix(ctx context.Context, action string, detail map[string]string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: drill fix audit id failed", slog.String("error", err.Error()))
		return
	}
	actor, ok := ctx.Value(fixitActorKey{}).(fixitActor)
	if !ok {
		actor = fixitActor{typ: "system", name: "fixit"}
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actor.typ, ActorID: actor.id, ActorName: actor.name, Ability: AbilityRoot, Method: "POST",
		Path: "/api/v1/fixes", StatusCode: 200, CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: "dashboard", Action: action, Detail: auditDetailJSON(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: drill fix audit failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
