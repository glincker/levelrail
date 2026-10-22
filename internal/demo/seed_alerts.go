package demo

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	chanSlack = "chn_demo_slack"
	chanMail  = "chn_demo_email"
	chanHook  = "chn_demo_webhook"
	diskRule  = "rule_demo_disk"
)

type ruleSpec struct {
	id, name, resource, severity, channel string
	rule                                  alerting.Rule
}

func alertRules() []ruleSpec {
	return []ruleSpec{
		{"rule_demo_cpu", "Storefront CPU high", "service:storefront-web", alerting.SeverityWarning, chanSlack,
			alerting.Rule{Kind: alerting.KindThreshold, Metric: "cpu_percent", Comparator: alerting.GreaterThan, Threshold: 85, ForDuration: 5 * time.Minute}},
		{"rule_demo_mem", "Billing memory near limit", "service:" + crashloopApp, alerting.SeverityCritical, chanSlack,
			alerting.Rule{Kind: alerting.KindThreshold, Metric: "memory_usage_bytes", Comparator: alerting.GreaterThan, Threshold: 450 << 20, ForDuration: 2 * time.Minute}},
		{"rule_demo_restarts", "Container restarts", "service:" + incidentApp, alerting.SeverityWarning, chanMail,
			alerting.Rule{Kind: alerting.KindThreshold, Metric: "container_restart_count", Comparator: alerting.GreaterThan, Threshold: 2, ForDuration: time.Minute}},
		{"rule_demo_crashloop", "Billing crashloop", "service:" + crashloopApp, alerting.SeverityCritical, chanHook,
			alerting.Rule{Kind: alerting.KindCrashloop, RestartCountThreshold: 3, RestartWindow: 10 * time.Minute}},
		{"rule_demo_cert", "Certificate expiring", "all certificates", alerting.SeverityWarning, chanMail,
			alerting.Rule{Kind: alerting.KindCertExpiry}},
		{diskRule, "Node disk above 80 percent", "all nodes", alerting.SeverityWarning, chanSlack,
			alerting.Rule{Kind: alerting.KindNodeDiskSpace, Threshold: 80}},
		{"rule_demo_errors", "API error rate", "service:" + incidentApp, alerting.SeverityCritical, chanSlack,
			alerting.Rule{Kind: alerting.KindThreshold, Metric: "http_responses_5xx", Comparator: alerting.GreaterThan, Threshold: 30, ForDuration: 3 * time.Minute}},
		{"rule_demo_backup", "pg-main backup missing", "database:pg-main", alerting.SeverityCritical, chanMail,
			alerting.Rule{Kind: alerting.KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "pg-main"}},
	}
}

// SeedAlerting writes alert rules, channels pointed at reserved hosts, and a
// history of fired and resolved events with one rule firing now.
func SeedAlerting(ctx context.Context, adb *alerting.DB, opts Options) error {
	anchor := opts.Anchor.UTC()
	story := storyFor(anchor)
	r := newRNG(opts.Seed ^ 0xa1e7)

	channels := []alerting.NotificationChannel{
		{ID: chanSlack, Name: "Ops Slack", Kind: alerting.NotifySlack, NotifyURL: "https://hooks.example.invalid/slack/ops", Enabled: true},
		{ID: chanMail, Name: "On-call email", Kind: alerting.NotifyEmail, NotifyURL: "oncall@acme.example", Enabled: true},
		{ID: chanHook, Name: "Incident webhook", Kind: alerting.NotifyGeneric, NotifyURL: "https://hooks.example.invalid/incidents", Enabled: true},
	}
	for _, c := range channels {
		c.CreatedAt = anchor.AddDate(0, 0, -historyDays).Format(time.RFC3339Nano)
		c.UpdatedAt = c.CreatedAt
		if err := adb.SaveNotificationChannel(ctx, c); err != nil {
			return fmt.Errorf("demo: save channel %q: %w", c.ID, err)
		}
	}
	specs := alertRules()
	for _, s := range specs {
		rule := s.rule
		rule.ID, rule.Name, rule.ResourceID, rule.Severity, rule.ChannelID, rule.Enabled = s.id, s.name, s.resource, s.severity, s.channel, true
		if err := adb.SaveRule(ctx, rule); err != nil {
			return fmt.Errorf("demo: save rule %q: %w", s.id, err)
		}
	}

	type fired struct {
		rule, app, node, detail string
		at                      time.Time
		resolveAfter            time.Duration
	}
	events := []fired{
		{"rule_demo_errors", incidentApp, "", "5xx responses above 30 per window", story.slowDeploy.Add(8 * time.Minute), 36 * time.Minute},
		{"rule_demo_crashloop", crashloopApp, "", "3 restarts in 10 minutes", story.crashStart.Add(6 * time.Minute), 14 * time.Minute},
		{"rule_demo_mem", crashloopApp, "", "memory 498 MiB above 450 MiB", story.crashStart.Add(2 * time.Minute), 12 * time.Minute},
		{"rule_demo_restarts", incidentApp, "", "3 restarts in 8 hours", anchor.AddDate(0, 0, -5), 40 * time.Minute},
		{"rule_demo_backup", "", "", "pg-main backup failed: upload failed: connection reset by peer", anchor.AddDate(0, 0, -6).Add(3*time.Hour + 20*time.Minute), 22 * time.Hour},
	}
	for d := 0; len(events) < 14-1 && d < 40; d++ {
		pick := specs[r.intn(len(specs))]
		if pick.id == diskRule || pick.id == "rule_demo_backup" {
			continue
		}
		events = append(events, fired{
			pick.id, "", "", "threshold crossed",
			anchor.AddDate(0, 0, -1-r.intn(12)).Add(time.Duration(r.intn(20*60)) * time.Minute),
			time.Duration(8+r.intn(50)) * time.Minute,
		})
	}
	events = append(events, fired{diskRule, "", "worker-sgp-1", "worker-sgp-1 disk 82 percent", story.diskFired, 0})

	names := map[string]alerting.Rule{}
	for _, s := range specs {
		names[s.id] = alerting.Rule{ID: s.id, Name: s.name, Kind: s.rule.Kind, ResourceID: s.resource, Severity: s.severity, ChannelID: s.channel}
	}
	// History dedupe reads the last row by time, so seed in time order.
	sort.SliceStable(events, func(i, j int) bool { return events[i].at.Before(events[j].at) })
	for i, e := range events {
		rule := names[e.rule]
		base := alerting.HistoryEntry{
			RuleID: rule.ID, RuleName: rule.Name, RuleKind: string(rule.Kind), ResourceID: rule.ResourceID,
			App: e.app, Node: e.node, Severity: rule.Severity, Outcome: alerting.OutcomeSent, ChannelID: rule.ChannelID,
		}
		fire := base
		fire.ID, fire.At, fire.Event, fire.Detail = fmt.Sprintf("ah_demo_%02d_f", i), e.at, alerting.EventFired, e.detail
		if err := adb.RecordHistory(ctx, fire); err != nil {
			return err
		}
		if e.resolveAfter == 0 {
			continue
		}
		resolve := base
		resolve.ID, resolve.At, resolve.Event, resolve.Detail = fmt.Sprintf("ah_demo_%02d_r", i), e.at.Add(e.resolveAfter), alerting.EventResolved, "back within threshold"
		if resolve.At.After(anchor) {
			resolve.At = anchor.Add(-time.Minute)
		}
		if err := adb.RecordHistory(ctx, resolve); err != nil {
			return err
		}
	}
	value := 82.0
	firingSince := story.diskFired
	return adb.UpdateState(ctx, diskRule, nil, &firingSince, true, anchor.Add(-time.Minute), &value)
}
