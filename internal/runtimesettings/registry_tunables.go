package runtimesettings

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Tunables that used to be env-only. The env name stays as the seed.
const (
	KeyProjectCloneMaxApps           = "projects.clone_max_apps"
	KeyProjectClonePlaceholderDomain = "projects.clone_placeholder_domain"
	KeyPreviewMigrateTimeout         = "previews.migrate_timeout"
	KeyPreviewMigrateWait            = "previews.migrate_wait"
	KeyCDNPurgeHistory               = "cdn.purge_history"
	KeyDeployExplainLogLines         = "deploy.explain_log_lines"
	KeyUpdateOfferedChannels         = "updates.offered_channels"
	KeyNetworkGCMaxRemovals          = "network.gc_max_removals"
	KeyMeshReadyRetrySeconds         = "mesh.ready_retry_seconds"
	KeyAppsContainersInspectTimeout  = "apps.containers_inspect_timeout"
	KeyBackupsComposeRestoreTimeout  = "backups.compose_restore_timeout"
	KeyBackupsScheduleRetainDefault  = "backups.schedule_retain_default"
	KeyBackupsDrillIntervalHours     = "backups.drill_interval_hours"
	KeyBackupsDrillRetentionDays     = "backups.drill_retention_days"
	KeyBackupsDrillTimeout           = "backups.drill_timeout"
)

var releaseChannels = []string{"stable", "beta", "edge"}

func init() {
	Register(Definition{
		Key: KeyBackupsDrillRetentionDays, Category: CategoryRetention, Kind: KindInt, Default: "90",
		Env: "APP_BACKUP_DRILL_RETENTION_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3650),
		Title:       "Restore drill history retention",
		Description: "Days of restore drill results to keep, from 0 to 3650. 0 keeps every result. The newest drill of each resource is always kept so health and alerts stay accurate. The next scheduler pass uses a changed value.",
	})
	Register(Definition{
		Key: KeyBackupsDrillTimeout, Category: CategoryRetention, Kind: KindDuration, Default: "1h",
		Env: "APP_BACKUP_DRILL_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, 24*time.Hour),
		Title:       "Restore drill timeout",
		Description: "How long one scheduled restore drill may run before it is cancelled and recorded as failed, from 1m to 24h. The next drill uses a changed value.",
	})
	Register(Definition{
		Key: KeyBackupsDrillIntervalHours, Category: CategoryRetention, Kind: KindInt, Default: "168",
		Env: "APP_BACKUP_DRILL_INTERVAL_HOURS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 8760),
		Title:       "Restore drill interval",
		Description: "Hours between scheduled restore drills of each backed up database or volume, from 0 to 8760. 0 turns scheduled drills off. A drill restores the newest backup into a throwaway target and records a pass or fail. The next scheduler pass uses a changed value.",
	})
	Register(Definition{
		Key: KeyBackupsScheduleRetainDefault, Category: CategoryRetention, Kind: KindInt, Default: "0",
		Env: "APP_BACKUP_SCHEDULE_RETAIN_DEFAULT", Apply: ApplyLive, CheckValue: intRangeCheck(0, 1000),
		Title:       "Compose backup schedule retain default",
		Description: "How many backups a compose database schedule keeps when it is saved without a retain count, from 0 to 1000. 0 keeps every backup. Schedules already saved are not changed.",
	})
	Register(Definition{
		Key: KeyBackupsComposeRestoreTimeout, Category: CategoryRetention, Kind: KindDuration, Default: "2h",
		Env: "APP_COMPOSE_RESTORE_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, 24*time.Hour),
		Title:       "Compose database restore timeout",
		Description: "How long one in-place restore of a compose database may run before it is cancelled, from 1m to 24h. The next restore uses a changed value.",
	})
	Register(Definition{
		Key: KeyAppsContainersInspectTimeout, Category: CategoryObservability, Kind: KindDuration, Default: "5s",
		Env: "APP_CONTAINERS_INSPECT_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, 2*time.Minute),
		Title:       "Container inspect timeout",
		Description: "How long the app containers view waits for a node's live inspect (health, restarts, networks, mounts), from 1s to 2m. When it runs out the view still answers, with those fields marked unavailable. The next request uses a changed value.",
	})
	Register(Definition{
		Key: KeyMeshReadyRetrySeconds, Category: CategoryNetworking, Kind: KindInt, Default: "10",
		Env: "APP_MESH_READY_RETRY_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(2, 300),
		Title:       "Mesh wait retry ceiling",
		Description: "The longest gap, in seconds from 2 to 300, between checks of an app waiting for its node's WireGuard mesh. Checks start at 2 seconds and double up to this value; the app starts as soon as the mesh is healthy. The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyNetworkGCMaxRemovals, Category: CategoryNetworking, Kind: KindInt, Default: "20",
		Env: "APP_NETWORK_GC_MAX_REMOVALS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000),
		Title:       "Network cleanup removals per pass",
		Description: "The most orphaned app networks one cleanup pass removes on a node, from 1 to 1000. Only networks labeled with this control plane are ever removed. The next pass uses a changed value.",
	})
	Register(Definition{
		Key: KeyProjectCloneMaxApps, Category: CategoryDeploys, Kind: KindInt, Default: "50",
		Env: "APP_PROJECT_CLONE_MAX_APPS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000),
		Title:       "Project clone app limit",
		Description: "The most apps one project clone may copy, from 1 to 1000. A project with more apps is refused with a clear message. The next clone uses a changed value.",
	})
	Register(Definition{
		Key: KeyProjectClonePlaceholderDomain, Category: CategoryDeploys, Kind: KindString, Default: "clone.invalid",
		Env: "APP_PROJECT_CLONE_PLACEHOLDER_DOMAIN", Apply: ApplyLive, CheckValue: placeholderDomainCheck,
		Title:       "Project clone placeholder domain",
		Description: "The domain suffix given to copied domains when a clone uses placeholder domains, so the copy never claims the real hostnames. The default is a reserved name that never resolves. The next clone uses a changed value.",
	})
	Register(Definition{
		Key: KeyPreviewMigrateTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "10m",
		Env: "APP_PREVIEW_MIGRATE_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(10*time.Second, 24*time.Hour),
		Title:       "Preview migration timeout",
		Description: "How long a preview migration command may run before the preview fails, from 10s to 24h. An app's own timeout still wins. The next preview uses a changed value.",
	})
	Register(Definition{
		Key: KeyPreviewMigrateWait, Category: CategoryDeploys, Kind: KindDuration, Default: "2m",
		Env: "APP_PREVIEW_MIGRATE_WAIT", Apply: ApplyLive, CheckValue: durationRangeCheck(5*time.Second, time.Hour),
		Title:       "Preview container wait",
		Description: "How long a preview migration waits for the preview container to appear before giving up, from 5s to 1h. The next preview uses a changed value.",
	})
	Register(Definition{
		Key: KeyCDNPurgeHistory, Category: CategoryRetention, Kind: KindInt, Default: "20",
		Env: "APP_CDN_PURGE_HISTORY", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000),
		Title:       "CDN purge history per app",
		Description: "How many edge purge records are kept per app in memory, from 1 to 1000. Records reset when the control plane restarts. Applies to the next purge.",
	})
	Register(Definition{
		Key: KeyDeployExplainLogLines, Category: CategoryObservability, Kind: KindInt, Default: "8",
		Env: "APP_DEPLOY_EXPLAIN_LOG_LINES", Apply: ApplyLive, CheckValue: intRangeCheck(1, 200),
		Title:       "Deploy explanation log lines",
		Description: "How many trailing log lines the failed-deploy explanation quotes, from 1 to 200. The next explanation uses a changed value.",
	})
	Register(Definition{
		Key: KeyUpdateOfferedChannels, Category: CategoryExperimental, Kind: KindStringList, Default: "stable",
		Env: "APP_UPDATE_CHANNELS_OFFERED", Apply: ApplyLive, CheckItem: releaseChannelCheck, EnvConvert: commaToColonList,
		Title:       "Update channels offered",
		Description: "Which release channels the dashboard offers for selection: stable, beta or edge. Others stay valid through the API. An empty list falls back to stable. The next page load shows a changed list.",
	})
}

func placeholderDomainCheck(canonical string) error {
	c := strings.TrimSpace(canonical)
	if c == "" || len(c) > 253 || strings.ContainsAny(c, " \t/:@") || strings.HasPrefix(c, ".") || strings.HasSuffix(c, ".") {
		return fmt.Errorf("value must be a bare domain suffix such as clone.invalid")
	}
	return nil
}

func releaseChannelCheck(item string) error {
	if !slices.Contains(releaseChannels, item) {
		return fmt.Errorf("unknown channel %q (allowed: %s)", item, strings.Join(releaseChannels, ", "))
	}
	return nil
}

// commaToColonList keeps the old comma separated env form working.
func commaToColonList(raw string) (string, error) {
	return strings.ReplaceAll(raw, ",", ":"), nil
}
