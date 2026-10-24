package cleanup

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SweepStore is the table access the built-in sweeper targets use. *store.DB satisfies it.
type SweepStore interface {
	CleanupStats(ctx context.Context, t store.CleanupTable) (store.CleanupTableStats, error)
	CleanupCountOlder(ctx context.Context, t store.CleanupTable, cutoff time.Time) (int64, error)
	CountAuditEntriesTiered(ctx context.Context, cutoff, securityCutoff time.Time) (int64, error)
	DeleteAuditEntriesTiered(ctx context.Context, cutoff, securityCutoff time.Time) (int64, error)
	PruneEmailDeliveries(ctx context.Context, before time.Time) (int64, error)
	DeleteWebhookDeliveriesOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	DeleteEventDeliveriesOlderThan(ctx context.Context, cutoff string) (int64, error)
}

// AlertHistory is the alert_history access. *alerting.DB satisfies it.
type AlertHistory interface {
	HistoryStats(ctx context.Context) (rows int64, oldest time.Time, bytes int64, err error)
	CountHistoryBefore(ctx context.Context, cutoff time.Time) (int64, error)
	PruneHistory(ctx context.Context, cutoff time.Time) (int64, error)
}

// BuiltinDeps are the collaborators of the built-in targets. A nil Alerts, Tokens or Janitor skips those targets.
type BuiltinDeps struct {
	Store    SweepStore
	Alerts   AlertHistory
	Tokens   TokenSource
	Janitor  JanitorSource
	Settings Settings
	Now      func() time.Time
}

// Built-in target ids.
const (
	TargetAuditLog        = "audit.log"
	TargetEmailLog        = "messaging.email_log"
	TargetWebhookDelivery = "messaging.webhook_deliveries"
	TargetEventWebhook    = "messaging.event_webhook_deliveries"
	TargetAlertHistory    = "observability.alert_history"
	TargetAPITokens       = "auth.api_tokens" //nolint:gosec // target id, not a credential
	linkSettingsJanitor   = "/settings/cleanup?tab=disk"
	i18nTitle             = "cleanup:targets.%s.title"
	i18nDescKey           = "cleanup:targets.%s.description"
)

// AuditWindows applies the audit floors and the rule that security entries outlive the general window.
// 0 means keep forever; a general window of 0 keeps everything whatever the security value says.
func AuditWindows(general, security int) (int, int) {
	if general > 0 && general < runtimesettings.AuditLogFloorDays {
		general = runtimesettings.AuditLogFloorDays
	}
	if security > 0 {
		security = max(security, runtimesettings.AuditSecurityFloorDays, general)
	}
	return general, security
}

// baseMeta starts a target as an existing sweeper; a target for never-bounded data sets Origin to OriginNew.
func baseMeta(id string, cat Category, title, desc, policy string) Meta {
	return Meta{ID: id, Category: cat, Title: title, Description: desc, PolicySetting: policy, Origin: OriginExisting,
		TitleKey: fmt.Sprintf(i18nTitle, id), DescKey: fmt.Sprintf(i18nDescKey, id)}
}

// RegisterBuiltins registers the targets that wrap existing sweepers, the token cleanup and the janitor stages.
func RegisterBuiltins(r *Registry, d BuiltinDeps) {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Store != nil {
		r.Register(auditTarget(d))
		r.Register(emailTarget(d))
		r.Register(webhookDeliveryTarget(d))
		r.Register(eventWebhookTarget(d))
	}
	if d.Alerts != nil {
		r.Register(alertHistoryTarget(d))
	}
	if d.Tokens != nil {
		r.Register(newTokenTarget(d))
	}
	if d.Janitor != nil {
		for _, t := range janitorTargets(d.Janitor) {
			r.Register(t)
		}
	}
}

func tableMeasure(s SweepStore, t store.CleanupTable) func(context.Context) (Usage, error) {
	return func(ctx context.Context) (Usage, error) {
		st, err := s.CleanupStats(ctx, t)
		if err != nil {
			return Usage{}, err
		}
		return usageOf(st.Rows, st.Bytes, st.Oldest), nil
	}
}

func auditTarget(d BuiltinDeps) Target {
	m := baseMeta(TargetAuditLog, CategoryAudit, "Audit log",
		"Who did what on this control plane. Sign-in, token and policy entries follow a longer window.", runtimesettings.KeyAuditLogDays)
	m.Floor, m.Destructive = runtimesettings.AuditLogFloorDays, true
	cutoffs := func(ctx context.Context, cutoff time.Time, days int) (time.Time, time.Time) {
		_, sec := AuditWindows(days, d.Settings.Int(ctx, runtimesettings.KeyAuditSecurityDays))
		if sec == 0 {
			return cutoff, time.Time{}
		}
		return cutoff, d.Now().Add(-time.Duration(sec) * hoursPerDayDuration)
	}
	return &cutoffTarget{
		meta: m, now: d.Now, measure: tableMeasure(d.Store, store.CleanupAuditLog),
		count: func(ctx context.Context, cutoff time.Time, days int) (int64, error) {
			c, s := cutoffs(ctx, cutoff, days)
			return d.Store.CountAuditEntriesTiered(ctx, c, s)
		},
		del: func(ctx context.Context, cutoff time.Time, days int) (int64, error) {
			c, s := cutoffs(ctx, cutoff, days)
			return d.Store.DeleteAuditEntriesTiered(ctx, c, s)
		},
	}
}

func emailTarget(d BuiltinDeps) Target {
	m := baseMeta(TargetEmailLog, CategoryMessaging, "Email delivery log",
		"One row per email the control plane tried to send.", runtimesettings.KeyEmailLogDays)
	return &cutoffTarget{
		meta: m, now: d.Now, measure: tableMeasure(d.Store, store.CleanupEmailDeliveries),
		count: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Store.CleanupCountOlder(ctx, store.CleanupEmailDeliveries, cutoff)
		},
		del: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Store.PruneEmailDeliveries(ctx, cutoff)
		},
	}
}

func webhookDeliveryTarget(d BuiltinDeps) Target {
	m := baseMeta(TargetWebhookDelivery, CategoryMessaging, "Inbound webhook deliveries",
		"Received git webhook requests kept for troubleshooting.", runtimesettings.KeyWebhookDeliveryDays)
	return &cutoffTarget{
		meta: m, now: d.Now, measure: tableMeasure(d.Store, store.CleanupWebhookDeliveries),
		count: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Store.CleanupCountOlder(ctx, store.CleanupWebhookDeliveries, cutoff)
		},
		del: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Store.DeleteWebhookDeliveriesOlderThan(ctx, cutoff)
		},
	}
}

func eventWebhookTarget(d BuiltinDeps) Target {
	m := baseMeta(TargetEventWebhook, CategoryMessaging, "Event webhook deliveries",
		"Finished outbound event webhook deliveries. Pending ones are never removed.", runtimesettings.KeyEventWebhookDays)
	return &cutoffTarget{
		meta: m, now: d.Now, measure: tableMeasure(d.Store, store.CleanupEventDeliveries),
		count: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Store.CleanupCountOlder(ctx, store.CleanupEventDeliveries, cutoff)
		},
		del: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Store.DeleteEventDeliveriesOlderThan(ctx, store.FormatAuditTime(cutoff))
		},
	}
}

func alertHistoryTarget(d BuiltinDeps) Target {
	m := baseMeta(TargetAlertHistory, CategoryObservability, "Alert history",
		"Fired and resolved alerts, which also feed the notification inbox.", runtimesettings.KeyAlertHistoryDays)
	return &cutoffTarget{
		meta: m, now: d.Now,
		measure: func(ctx context.Context) (Usage, error) {
			rows, oldest, bytes, err := d.Alerts.HistoryStats(ctx)
			if err != nil {
				return Usage{}, err
			}
			return usageOf(rows, bytes, oldest), nil
		},
		count: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Alerts.CountHistoryBefore(ctx, cutoff)
		},
		del: func(ctx context.Context, cutoff time.Time, _ int) (int64, error) {
			return d.Alerts.PruneHistory(ctx, cutoff)
		},
	}
}
