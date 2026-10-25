package cleanup

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RetentionStore is the table access of the age-bounded history targets. *store.DB satisfies it.
type RetentionStore interface {
	RetentionStats(ctx context.Context, t store.RetentionTable) (store.CleanupTableStats, error)
	RetentionCountOlder(ctx context.Context, t store.RetentionTable, cutoff time.Time) (int64, error)
	RetentionDeleteOlder(ctx context.Context, t store.RetentionTable, cutoff time.Time, batch int) (int64, error)
}

// Targets for history that had no sweeper before. They start in preview.
const (
	TargetAuthExpired     = "auth.expired_credentials"
	TargetAIHistory       = "data.ai_history"
	TargetRestoreHistory  = "data.restore_history"
	TargetProbeAttempts   = "deploys.probe_attempts"
	TargetAppHistory      = "deploys.app_history"
	TargetUsageCost       = "observability.usage_cost"
	TargetStatusIncidents = "observability.status_incidents"
)

const envDeleteBatch = "APP_CLEANUP_DELETE_BATCH"

// deleteBatch is the rows per DELETE statement; the store falls back to its own size when unset or invalid.
func deleteBatch() int {
	if n, err := strconv.Atoi(os.Getenv(envDeleteBatch)); err == nil && n > 0 {
		return n
	}
	return 0
}

type retentionDef struct {
	id, title, desc, policy string
	cat                     Category
	tables                  []store.RetentionTable
}

func retentionDefs() []retentionDef {
	return []retentionDef{
		{TargetAuthExpired, "Expired sign-in records",
			"Sessions, device login codes and magic links that expired before the window. Live and recently expired ones are kept.",
			runtimesettings.KeyAuthExpiredDays, CategoryAuth,
			[]store.RetentionTable{store.RetentionAuthSessions, store.RetentionAuthDeviceCodes, store.RetentionAuthMagicLinks}},
		{TargetAIHistory, "AI assistant history",
			"Chats idle past the window with their messages, and AI usage metering rows.",
			runtimesettings.KeyAIHistoryDays, CategoryData,
			[]store.RetentionTable{store.RetentionAIChatMessages, store.RetentionAIChatSessions, store.RetentionAIUsage}},
		{TargetRestoreHistory, "Restore and drill history",
			"Finished restores, point-in-time restores and backup drills. Running ones are kept.",
			runtimesettings.KeyRestoreHistoryDays, CategoryData,
			[]store.RetentionTable{store.RetentionRestoreHistory, store.RetentionPITRRestore, store.RetentionBackupDrills}},
		{TargetProbeAttempts, "Readiness probe attempts",
			"Per-attempt readiness probe detail recorded during deploys.",
			runtimesettings.KeyProbeAttemptDays, CategoryDeploys,
			[]store.RetentionTable{store.RetentionProbeAttempts}},
		{TargetAppHistory, "App events and schedule history",
			"App lifecycle events, scheduled deploy history and last canary outcomes.",
			runtimesettings.KeyAppHistoryDays, CategoryDeploys,
			[]store.RetentionTable{store.RetentionAppEvents, store.RetentionAppSchedule, store.RetentionCanaryOutcomes}},
		{TargetUsageCost, "Daily cost snapshots",
			"One cost showback snapshot per day and resource.",
			runtimesettings.KeyUsageCostDays, CategoryObservability,
			[]store.RetentionTable{store.RetentionUsageCostDaily}},
		{TargetStatusIncidents, "Resolved status incidents",
			"Resolved incidents and completed maintenance. Open and scheduled ones are never removed.",
			runtimesettings.KeyStatusIncidentDays, CategoryObservability,
			[]store.RetentionTable{store.RetentionStatusIncidents}},
	}
}

func retentionTargets(d BuiltinDeps) []Target {
	var out []Target
	for _, def := range retentionDefs() {
		m := baseMeta(def.id, def.cat, def.title, def.desc, def.policy)
		m.Origin = OriginNew
		out = append(out, retentionTarget(d, m, def.tables))
	}
	return out
}

// retentionTarget sums its tables so one window governs the set; a table listed after another may depend on it
// being emptied first (chat messages before chats).
func retentionTarget(d BuiltinDeps, m Meta, tables []store.RetentionTable) Target {
	s := d.Retention
	return &cutoffTarget{
		meta: m, now: d.Now,
		measure: func(ctx context.Context) (Usage, error) {
			var rows, bytes int64
			var oldest time.Time
			for _, t := range tables {
				st, err := s.RetentionStats(ctx, t)
				if err != nil {
					return Usage{}, err
				}
				rows += st.Rows
				bytes += st.Bytes
				if !st.Oldest.IsZero() && (oldest.IsZero() || st.Oldest.Before(oldest)) {
					oldest = st.Oldest
				}
			}
			return usageOf(rows, bytes, oldest), nil
		},
		count: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			var total int64
			for _, t := range tables {
				n, err := s.RetentionCountOlder(ctx, t, cutoff)
				if err != nil {
					return 0, err
				}
				total += n
			}
			return total, nil
		},
		del: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			var total int64
			for _, t := range tables {
				n, err := s.RetentionDeleteOlder(ctx, t, cutoff, deleteBatch())
				total += n
				if err != nil {
					return total, err
				}
			}
			return total, nil
		},
	}
}
