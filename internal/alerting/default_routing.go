package alerting

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultAdminEmailChannelID is the notification channel the seeded rules are routed to when email is configured.
const DefaultAdminEmailChannelID = "chn_default_admin_email"

const defaultRouteSeedPrefix = "route-"

// RouteDefaultRules attaches every seeded rule that has no destination yet to an admin email channel, once per rule.
// An empty adminEmail (no email provider or no admin) leaves them in-app only, which the alert history and inbox always record.
// A rule is routed only once, so an operator who later detaches or changes its channel is never overridden.
func (db *DB) RouteDefaultRules(ctx context.Context, adminEmail string) (int, error) {
	if adminEmail == "" {
		return 0, nil
	}
	rules, err := db.ListDefaultRules(ctx)
	if err != nil {
		return 0, err
	}
	pending := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.ChannelID != "" || r.NotifyURL != "" {
			continue
		}
		var done int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_default_seeds WHERE key = ?`, defaultRouteSeedPrefix+r.ID).Scan(&done); err != nil {
			return 0, fmt.Errorf("alerting: read default route %q: %w", r.ID, err)
		}
		if done == 0 {
			pending = append(pending, r)
		}
	}
	if len(pending) == 0 {
		return 0, nil
	}
	if _, err := db.GetNotificationChannel(ctx, DefaultAdminEmailChannelID); errors.Is(err, ErrNotificationChannelNotFound) {
		if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: DefaultAdminEmailChannelID, Name: "Admin email", Kind: NotifyEmail, NotifyURL: adminEmail, Enabled: true}); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	var errs []error
	routed := 0
	for _, r := range pending {
		r.ChannelID, r.NotifyKind = DefaultAdminEmailChannelID, NotifyEmail
		if err := db.SaveRule(ctx, r); err != nil {
			errs = append(errs, fmt.Errorf("alerting: route default rule %q: %w", r.ID, err))
			continue
		}
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO alert_default_seeds (key, rule_id, seeded_at) VALUES (?, ?, ?)`,
			defaultRouteSeedPrefix+r.ID, r.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			errs = append(errs, fmt.Errorf("alerting: record default route %q: %w", r.ID, err))
			continue
		}
		routed++
	}
	return routed, errors.Join(errs...)
}
