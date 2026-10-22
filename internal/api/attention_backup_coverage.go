package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Item kinds for the backup coverage and orphaned data recommendations.
const (
	kindBackupUnprotected = "backup_unprotected"
	kindOrphanedData      = "orphaned_data"

	linkBackups       = "/backups"
	linkBackupsOrphan = "/backups#orphaned-data"
)

// backupCoverageAttentionItems recommends scheduling backups for resources
// that have none, and cleaning up orphaned data past a size threshold. Both
// are recommendations: nothing is applied or deleted from here.
func (rt *Router) backupCoverageAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	items := rt.unprotectedAttentionItems(r)
	return append(items, rt.orphanAttentionItems(r, abilities)...)
}

func (rt *Router) unprotectedAttentionItems(r *http.Request) []attention.Item {
	ctx := r.Context()
	canSeeDB, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.logger.Warn("api: attention feed: database visibility failed", slog.String("error", err.Error()))
		return nil
	}
	canSeeApp, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.logger.Warn("api: attention feed: app visibility failed", slog.String("error", err.Error()))
		return nil
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		rt.logger.Warn("api: attention feed: list databases failed", slog.String("error", err.Error()))
		return nil
	}
	dbs = rt.withoutPendingDatabases(ctx, dbs)
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.logger.Warn("api: attention feed: list apps failed", slog.String("error", err.Error()))
		return nil
	}
	scratchApps, scratchDBs, err := rt.scratchResourceNames(ctx)
	if err != nil {
		rt.logger.Warn("api: attention feed: list previews failed", slog.String("error", err.Error()))
		return nil
	}

	var names []string
	for _, d := range dbs {
		if scratchDBs[d.Name] || !canSeeDB(d.Name) || d.Suspended {
			continue
		}
		if d.BackupSchedule == "" || d.BackupTargetID == "" {
			names = append(names, d.Name)
		}
	}
	for _, svc := range services {
		if scratchApps[svc.Name] || store.IsCanaryService(svc.Name) || !canSeeApp(svc.Name) {
			continue
		}
		for _, v := range toAppVolumeResources(svc) {
			cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(ctx, svc.Name, v.Name)
			if err != nil || cfg.BackupSchedule == "" || cfg.BackupTargetID == "" {
				names = append(names, svc.Name+"/"+v.Name)
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	detail := fmt.Sprintf("%d resources have no backup schedule: %s", len(names), joinFirst(names, 5))
	it := feedItem(attention.Warning, kindBackupUnprotected, "resources", detail, map[string]string{"count": fmt.Sprint(len(names))})
	it.Title = fmt.Sprintf("%d resources are not backed up", len(names))
	it.Action = "Open Backups and protect them in one step"
	it.Link, it.Class = linkBackups, attention.ClassRecommendation
	return []attention.Item{it}
}

func (rt *Router) orphanAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.orphanedVolumes == nil || !hasAbility(abilities, AbilityWriteSensitive) {
		return nil
	}
	if scope, ok := callerTeamScope(r.Context()); ok && scope != nil && !scope.instance {
		return nil
	}
	orphans, err := rt.computeOrphanedVolumes(r.Context())
	if err != nil {
		rt.logger.Warn("api: attention feed: orphaned data failed", slog.String("error", err.Error()))
		return nil
	}
	var total int64
	for _, v := range orphans {
		if v.SizeBytes > 0 {
			total += v.SizeBytes
		}
	}
	if len(orphans) == 0 || total < orphanAttentionThreshold() {
		return nil
	}
	detail := fmt.Sprintf("%d volumes of deleted databases or apps use %s", len(orphans), humanBytes(total))
	it := feedItem(attention.Info, kindOrphanedData, "volumes", detail, map[string]string{"count": fmt.Sprint(len(orphans)), "bytes": fmt.Sprint(total)})
	it.Title = "Orphaned data is taking up " + humanBytes(total)
	it.Action = "Inspect it under Backups and reclaim what you no longer need"
	it.Link, it.Class = linkBackupsOrphan, attention.ClassRecommendation
	return []attention.Item{it}
}

func joinFirst(names []string, n int) string {
	out := ""
	for i, s := range names {
		if i == n {
			return out + fmt.Sprintf(", and %d more", len(names)-n)
		}
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
