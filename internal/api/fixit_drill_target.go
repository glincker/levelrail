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
	// Repoint moves the resource's backup schedule off the missing target;
	// false means the schedule did not use it.
	Repoint func(ctx context.Context, d store.BackupDrill, targetID string) (bool, error)
	// Drop deletes the failed drill rows that name a missing target.
	Drop func(ctx context.Context, missingTargetID string) (int, error)
	// Audit records what a fix changed.
	Audit func(ctx context.Context, actor, action string, detail map[string]string)
	// CLI is the command that lists the backup targets to choose from.
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
	out := danglingDrills{targets: targets}
	for _, dr := range backup.SummarizeDrillFailures(failed).Dangling {
		if !live[dr.TargetID] {
			out.drills = append(out.drills, dr)
		}
	}
	return out, nil
}

func (g danglingDrills) changes() []fixit.Change {
	out := make([]fixit.Change, 0, len(g.drills))
	for _, d := range g.drills {
		out = append(out, fixit.Change{Label: backup.DrillResourceLabel(d), Detail: "drill points at missing target " + d.TargetID})
	}
	return out
}

func (g danglingDrills) missingIDs() []string {
	seen := map[string]bool{}
	var ids []string
	for _, d := range g.drills {
		if !seen[d.TargetID] {
			seen[d.TargetID] = true
			ids = append(ids, d.TargetID)
		}
	}
	sort.Strings(ids)
	return ids
}

func (g danglingDrills) names() string {
	return strings.TrimPrefix(backup.DrillFailureSummary{Dangling: g.drills}.Detail(), "the drill points at a backup target that no longer exists ")
}

func (d drillTargetDeps) drop(ctx context.Context, g danglingDrills, actor string) error {
	for _, id := range g.missingIDs() {
		n, err := d.Drop(ctx, id)
		if err != nil {
			return fmt.Errorf("remove stale drill reference to %s: %w", id, err)
		}
		d.Audit(ctx, actor, drillAuditRemoved, map[string]string{"target_id": id, "drills": fmt.Sprint(n)})
	}
	return nil
}

// drillStaleRefFix removes the failed drill rows of a deleted target when no target exists to move to.
// Safe and reversible: no backup or target is touched, and the next drill recreates a row.
func drillStaleRefFix(d drillTargetDeps) fixit.Fix {
	return fixit.Fix{
		ID: drillStaleRefFixID, CheckID: drillTargetCheckID, Subject: "backup", Kind: fixit.KindAuto,
		Title: "Remove the stale drill reference",
		Why:   "The drill points at a backup target that no longer exists and there is no other target to move it to. Removing the stale reference clears the row. No backup or target is deleted.",
		Risk:  fixit.RiskSafe, Reversible: true, Needs: "admin",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			g, err := d.dangling(ctx)
			if err != nil {
				return fixit.Applicability{}, err
			}
			if len(g.drills) == 0 || len(g.targets) > 0 {
				return fixit.Applicability{Detail: "no stale drill reference to remove", Changes: []fixit.Change{}}, nil
			}
			return fixit.Applicability{Failing: true, Detail: "the drill points at a backup target that no longer exists " + g.names(), Changes: g.changes()}, nil
		},
		Run: func(ctx context.Context, in fixit.Input) error {
			g, err := d.dangling(ctx)
			if err != nil {
				return err
			}
			if len(g.drills) == 0 || len(g.targets) > 0 {
				return fixit.ErrCannotFixHere
			}
			return d.drop(ctx, g, in.Actor)
		},
	}
}

// drillRepointFix moves each affected schedule to a current target. It changes which destination
// holds future backups, so it is guided and needs a click, never auto.
func drillRepointFix(d drillTargetDeps) fixit.Fix {
	return fixit.Fix{
		ID: drillRepointFixID, CheckID: drillTargetCheckID, Subject: "backup", Kind: fixit.KindGuided,
		Title: "Point the drill at a current backup target",
		Why:   "The drill points at a backup target that no longer exists. Choosing a current target moves the resource's backup schedule there and clears the stale reference. Existing backups and targets are never deleted.",
		Risk:  fixit.RiskSafe, Needs: "admin",
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
			reason := "Choose the backup target the affected resources should use, then apply."
			if len(opts) == 1 {
				p.Default = opts[0].Value
				reason = "One backup target exists (" + opts[0].Label + "). Apply to move the affected resources to it."
			}
			return fixit.Applicability{
				Failing: true, Detail: "the drill points at a backup target that no longer exists " + g.names(),
				Kind: fixit.KindGuided, Reason: reason, Command: d.CLI + " backup-targets list",
				Changes: g.changes(), Param: p,
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
					d.Audit(ctx, in.Actor, drillAuditRepointed, map[string]string{"resource": backup.DrillResourceLabel(dr), "from": dr.TargetID, "to": target})
				}
			}
			return d.drop(ctx, g, in.Actor)
		},
	}
}

func chooseDrillTarget(targets []store.BackupTarget, want string) (string, error) {
	if want == "" && len(targets) == 1 {
		return targets[0].ID, nil
	}
	for _, t := range targets {
		if t.ID == want {
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
		Failed: rt.latestFailedDrills,
		Targets: func(ctx context.Context) ([]store.BackupTarget, error) {
			return rt.backupTargets.ListBackupTargets(ctx)
		},
		Repoint: rt.repointDrillResource,
		Drop:    rt.dropDrillsForTarget,
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

func (rt *Router) repointDrillResource(ctx context.Context, d store.BackupDrill, targetID string) (bool, error) {
	if d.ResourceKind == store.BackupResourceKindVolume {
		if rt.serviceVolumeBackupSchedule == nil {
			return false, nil
		}
		cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(ctx, d.ServiceName, d.VolumeName)
		if err != nil || cfg.BackupTargetID != d.TargetID {
			return false, nil
		}
		err = rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(ctx, d.ServiceName, d.VolumeName, targetID, cfg.BackupSchedule, cfg.BackupRetain, cfg.BackupRetainDays, cfg.SqlitePath)
		return err == nil, err
	}
	dbs, ok := rt.databases.(drillScheduleSetter)
	if !ok {
		return false, nil
	}
	cur, err := dbs.GetDesiredDatabase(ctx, d.DatabaseName)
	if err != nil || cur == nil || cur.BackupTargetID != d.TargetID {
		return false, nil
	}
	err = dbs.SetDatabaseBackupSchedule(ctx, d.DatabaseName, targetID, cur.BackupSchedule, cur.BackupRetain, cur.BackupRetainDays)
	return err == nil, err
}

func (rt *Router) dropDrillsForTarget(ctx context.Context, targetID string) (int, error) {
	db, ok := rt.fixit.store.(execer)
	if !ok {
		return 0, fixit.ErrCannotFixHere
	}
	res, err := db.ExecContext(ctx, `DELETE FROM backup_drills WHERE target_id = ? AND status = ?`, targetID, store.DrillStatusFailed)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func auditDetailJSON(detail map[string]string) string {
	b, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(b)
}

func (rt *Router) auditDrillFix(ctx context.Context, actor, action string, detail map[string]string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: drill fix audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorName: actor, Ability: AbilityWrite, Method: "FIX", Path: "fixit/" + action,
		StatusCode: 200, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system", Action: action,
		Detail: auditDetailJSON(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: drill fix audit failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
