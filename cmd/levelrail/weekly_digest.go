package main

import (
	"context"
	"log/slog"
	"net/mail"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const weeklyDigestCheckEvery = 10 * time.Minute

// firstAdminEmail returns the first user's valid address, or "" when there is none.
func firstAdminEmail(ctx context.Context, db *store.DB) string {
	users, err := db.ListUsers(ctx)
	if err != nil {
		return ""
	}
	for _, u := range users {
		if u.IsFirstUser && u.Email != "" {
			if _, err := mail.ParseAddress(u.Email); err == nil {
				return u.Email
			}
		}
	}
	return ""
}

// wireWeeklyDigest registers the preview route and starts the schedule check. The digest is built only from data the
// control plane already holds, and is mailed only when an email provider is configured.
func wireWeeklyDigest(ctx context.Context, logger *slog.Logger, router *api.Router, adb *alerting.DB, db *store.DB, metrics alerting.MetricsSource, tracker *alerting.RestartTracker, sender *email.DynamicSender, rs *runtimesettings.Service) {
	if rs == nil {
		return
	}
	src := alerting.WeeklySources{
		Services: db, Attempts: db, Databases: db, Backups: db, Certs: db, Nodes: db,
		Metrics: metrics, Tracker: tracker, Logger: logger,
	}
	router.SetAlertDigestPreview(func(c context.Context) (string, string, error) {
		subject, text, _ := alerting.RenderWeeklyDigest(alerting.CollectWeeklyFacts(c, src, time.Now()))
		return subject, text, nil
	})
	w := &alerting.WeeklyDigest{
		Sources: src,
		Mailer:  sender,
		Schedule: func(c context.Context) (int, int, bool) {
			wd, hr, on, err := runtimesettings.ParseDigestSchedule(rs.String(c, runtimesettings.KeyAlertsDigestSchedule))
			return wd, hr, on && err == nil
		},
		Recipient: func(c context.Context) string { return firstAdminEmail(c, db) },
		Last: func(c context.Context) string {
			day := time.Now().UTC().Format("2006-01-02")
			if adb.WeeklyDigestSentOn(c, day) {
				return day
			}
			return ""
		},
		SetLast: func(c context.Context, day string) { adb.MarkWeeklyDigestSent(c, day) },
		Logger:  logger,
	}
	go loopTicks(ctx, "weekly digest", weeklyDigestCheckEvery, false, func() { w.Tick(ctx, time.Now()) })
}
