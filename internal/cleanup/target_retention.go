package cleanup

import (
	"context"
	"fmt"
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
	RetentionCountOverCap(ctx context.Context, t store.RetentionTable, cutoff time.Time, keep int) (int64, error)
	RetentionDeleteOverCap(ctx context.Context, t store.RetentionTable, cutoff time.Time, keep, batch int) (int64, error)
}

// Targets for history that had no sweeper before. They start in preview.
const (
	TargetAuthExpired     = "auth.expired_credentials"
	TargetAIHistory       = "data.ai_history"
	TargetRestoreHistory  = "data.restore_history"
	TargetProbeAttempts   = "deploys.probe_attempts"
	TargetAppHistory      = "deploys.app_history"
	TargetForgeStatus     = "deploys.forge_status"
	TargetUsageCost       = "observability.usage_cost"
	TargetStatusIncidents = "observability.status_incidents"
	TargetDeployHistory   = "deploys.deploy_history"
	TargetNodeEvents      = "observability.node_events"
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
	// capped tables also honor retention.max_rows_per_app.
	capped []store.RetentionTable
}

func retentionDefs() []retentionDef {
	return []retentionDef{
		{TargetAuthExpired, "Expired sign-in records",
			"Sessions, device login codes, magic links and finished MCP OAuth requests, refresh rows and grants that expired before the window. Live and recently expired ones are kept.",
			runtimesettings.KeyAuthExpiredDays, CategoryAuth,
			[]store.RetentionTable{store.RetentionAuthSessions, store.RetentionAuthDeviceCodes, store.RetentionAuthMagicLinks,
				store.RetentionMCPOAuthRequests, store.RetentionMCPOAuthRefresh, store.RetentionMCPOAuthTokens, store.RetentionMCPOAuthGrants}, nil},
		{TargetAIHistory, "AI assistant history",
			"Chats idle past the window with their messages, and AI usage metering rows.",
			runtimesettings.KeyAIHistoryDays, CategoryData,
			[]store.RetentionTable{store.RetentionAIChatMessages, store.RetentionAIChatSessions, store.RetentionAIUsage}, nil},
		{TargetRestoreHistory, "Restore and drill history",
			"Finished restores, point-in-time restores and backup drills. Running ones are kept.",
			runtimesettings.KeyRestoreHistoryDays, CategoryData,
			[]store.RetentionTable{store.RetentionRestoreHistory, store.RetentionPITRRestore, store.RetentionBackupDrills}, nil},
		{TargetProbeAttempts, "Readiness probe attempts",
			"Per-attempt readiness probe detail recorded during deploys.",
			runtimesettings.KeyProbeAttemptDays, CategoryDeploys,
			[]store.RetentionTable{store.RetentionProbeAttempts}, nil},
		{TargetAppHistory, "App events and schedule history",
			"App lifecycle events, scheduled deploy history and last canary outcomes.",
			runtimesettings.KeyAppHistoryDays, CategoryDeploys,
			[]store.RetentionTable{store.RetentionAppEvents, store.RetentionAppSchedule, store.RetentionCanaryOutcomes},
			[]store.RetentionTable{store.RetentionAppEvents}},
		{TargetForgeStatus, "Forge commit status reports",
			"Commit statuses Levelrail posted to a git forge, shown on the app delivery timeline.",
			runtimesettings.KeyForgeStatusDays, CategoryDeploys,
			[]store.RetentionTable{store.RetentionForgeStatus}, nil},
		{TargetUsageCost, "Daily cost snapshots",
			"One cost showback snapshot per day and resource.",
			runtimesettings.KeyUsageCostDays, CategoryObservability,
			[]store.RetentionTable{store.RetentionUsageCostDaily}, nil},
		{TargetStatusIncidents, "Resolved status incidents",
			"Resolved incidents and completed maintenance. Open and scheduled ones are never removed.",
			runtimesettings.KeyStatusIncidentDays, CategoryObservability,
			[]store.RetentionTable{store.RetentionStatusIncidents}, nil},
		{TargetDeployHistory, "Deploy history",
			"Finished deploy attempts and their readiness probe detail. Unfinished, pinned and the newest succeeded deploy of each app are kept.",
			runtimesettings.KeyDeployAttemptDays, CategoryDeploys,
			[]store.RetentionTable{store.RetentionDeployAttempts}, []store.RetentionTable{store.RetentionDeployAttempts}},
		{TargetNodeEvents, "Node events",
			"Node status changes and node security events.",
			runtimesettings.KeyNodeEventDays, CategoryObservability,
			[]store.RetentionTable{store.RetentionNodeStatus, store.RetentionNodeSecurity}, nil},
	}
}

func retentionTargets(d BuiltinDeps) []Target {
	var out []Target
	for _, def := range retentionDefs() {
		m := baseMeta(def.id, def.cat, def.title, def.desc, def.policy)
		m.Origin = OriginNew
		out = append(out, retentionTarget(d, m, def.tables, def.capped))
	}
	return out
}

// retentionTarget sums its tables so one window governs the set; a table listed after another may depend on it
// being emptied first (chat messages before chats).
func retentionTarget(d BuiltinDeps, m Meta, tables, capped []store.RetentionTable) Target {
	t := ageTarget(d, m, tables)
	if len(capped) == 0 || d.Settings == nil {
		return t
	}
	return &cappedTarget{cutoffTarget: t, store: d.Retention, tables: capped, keep: func(ctx context.Context) int {
		return d.Settings.Int(ctx, runtimesettings.KeyMaxRowsPerApp)
	}}
}

func ageTarget(d BuiltinDeps, m Meta, tables []store.RetentionTable) *cutoffTarget {
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

// cappedTarget adds the per-app row limit to an age window: after the age pass it removes rows beyond the newest
// keep rows of each app, so a busy app cannot outgrow the limit inside the window.
type cappedTarget struct {
	*cutoffTarget
	store  RetentionStore
	tables []store.RetentionTable
	keep   func(ctx context.Context) int
}

func (c *cappedTarget) Preview(ctx context.Context, days int) (Impact, error) {
	imp, err := c.cutoffTarget.Preview(ctx, days)
	if err != nil {
		return imp, err
	}
	n, err := c.overCap(ctx, days)
	imp.Rows += n
	return imp, err
}

func (c *cappedTarget) overCap(ctx context.Context, days int) (int64, error) {
	keep := c.keep(ctx)
	var total int64
	for _, t := range c.tables {
		n, err := c.store.RetentionCountOverCap(ctx, t, c.cutoff(days), keep)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (c *cappedTarget) Run(ctx context.Context, days int, opts RunOptions) Result {
	if opts.DryRun {
		imp, err := c.Preview(ctx, days)
		return Result{Rows: imp.Rows, Bytes: imp.Bytes, Err: err}
	}
	res := c.cutoffTarget.Run(ctx, days, opts)
	if res.Err != nil || res.More {
		return res
	}
	keep := c.keep(ctx)
	for _, t := range c.tables {
		n, err := c.store.RetentionDeleteOverCap(ctx, t, c.cutoff(days), keep, deleteBatch())
		res.Rows += n
		if err != nil {
			res.Err = fmt.Errorf("cleanup: %s: %w", c.meta.ID, err)
			return res
		}
	}
	return res
}
