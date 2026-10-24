package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Retention and cleanup scheduler keys. Each retention key is the runtime form of an
// older env var, which stays as that setting's env override.
const (
	KeyAuditLogDays         = "retention.audit_log_days"
	KeyAuditSecurityDays    = "retention.audit_security_days"
	KeyEmailLogDays         = "retention.email_log_days"
	KeyWebhookDeliveryDays  = "retention.webhook_delivery_days"
	KeyEventWebhookDays     = "retention.event_webhook_days"
	KeyAlertHistoryDays     = "retention.alert_history_days"
	KeyCleanupInterval      = "cleanup.interval"
	KeyCleanupEnabled       = "cleanup.enabled"
	AuditLogFloorDays       = 90
	AuditSecurityFloorDays  = 365
	CleanupMinInterval      = 15 * time.Minute
	cleanupDefaultInterval  = "6h"
	hoursPerDay             = 24
	maxDurationEnvDaysValue = 36500
)

// RetentionPresets are the day counts a cleanup policy offers; 0 keeps the data forever.
var RetentionPresets = []int{0, 30, 60, 90, 180, 365}

// MinDaysCheck rejects a positive day count below floor. 0 (keep forever) always passes.
func MinDaysCheck(floor int) func(string) error {
	return func(canonical string) error {
		n, err := strconv.Atoi(canonical)
		if err != nil || n == 0 || n >= floor {
			return nil
		}
		return fmt.Errorf("retention cannot be shorter than %d days, choose %d or more (or 0 to keep forever)", floor, floor)
	}
}

// MinDurationCheck rejects a duration below floor.
func MinDurationCheck(floor time.Duration) func(string) error {
	return func(canonical string) error {
		d, err := time.ParseDuration(canonical)
		if err == nil && d < floor {
			return fmt.Errorf("must be at least %s", floor)
		}
		return nil
	}
}

// DurationEnvToDays converts a legacy duration env value (a Go duration such as 720h, or Nd)
// into whole days, rounding up so a window never shrinks. allowZero says whether 0 means keep forever.
func DurationEnvToDays(allowZero bool) func(string) (string, error) {
	return func(raw string) (string, error) {
		raw = strings.TrimSpace(raw)
		var d time.Duration
		if n, ok := strings.CutSuffix(raw, "d"); ok {
			days, err := strconv.Atoi(n)
			if err != nil || days < 0 {
				return "", fmt.Errorf("%q is not a duration such as 720h or 30d", raw)
			}
			d = time.Duration(days) * hoursPerDay * time.Hour
		} else {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed < 0 {
				return "", fmt.Errorf("%q is not a duration such as 720h or 30d", raw)
			}
			d = parsed
		}
		if d == 0 && !allowZero {
			return "", fmt.Errorf("a zero duration is not a retention window")
		}
		days := int((d + hoursPerDay*time.Hour - 1) / (hoursPerDay * time.Hour))
		if days > maxDurationEnvDaysValue {
			return "", fmt.Errorf("%q is longer than %d days", raw, maxDurationEnvDaysValue)
		}
		return strconv.Itoa(days), nil
	}
}

// PositiveDaysEnv accepts only a positive whole-day env value, matching the old audit sweepers where 0 meant "use the default".
func PositiveDaysEnv(raw string) (string, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return "", fmt.Errorf("%q is not a positive number of days", raw)
	}
	return strconv.Itoa(n), nil
}

func init() {
	retention := func(key, env, def, title, desc string) Definition {
		return Definition{
			Key: key, Category: CategoryRetention, Kind: KindInt, Default: def, Env: env, Apply: ApplyLive,
			AllowedInts: RetentionPresets, EnvLoose: true,
			TitleKey: "runtimeSettings:items." + key + ".title", DescKey: "runtimeSettings:items." + key + ".description",
			Title: title, Description: desc,
		}
	}
	audit := retention(KeyAuditLogDays, "APP_AUDIT_LOG_RETENTION_DAYS", "90",
		"Keep audit log entries for", "Days an audit log entry is kept before the cleanup removes it. Sign-in and policy entries follow their own, longer window. 0 keeps everything.")
	audit.Risk, audit.EnvConvert, audit.CheckValue = RiskWeakensSecurity, PositiveDaysEnv, MinDaysCheck(AuditLogFloorDays)
	Register(audit)
	sec := retention(KeyAuditSecurityDays, "APP_AUDIT_LOG_SECURITY_RETENTION_DAYS", "365",
		"Keep security audit entries for", "Days sign-in, step-up, token and policy audit entries are kept. Never shorter than the general window. 0 keeps them forever.")
	sec.Risk, sec.EnvConvert, sec.CheckValue = RiskWeakensSecurity, PositiveDaysEnv, MinDaysCheck(AuditSecurityFloorDays)
	Register(sec)
	Register(retention(KeyEmailLogDays, "APP_EMAIL_LOG_RETENTION_DAYS", "30",
		"Keep the email delivery log for", "Days an email delivery log row is kept. 0 keeps them forever."))
	Register(retention(KeyWebhookDeliveryDays, "APP_WEBHOOK_DELIVERY_RETENTION_DAYS", "30",
		"Keep inbound webhook deliveries for", "Days a received git webhook delivery is kept for troubleshooting. 0 keeps them forever."))
	ev := retention(KeyEventWebhookDays, "APP_EVENT_WEBHOOK_RETENTION", "30",
		"Keep event webhook deliveries for", "Days a finished outbound event webhook delivery is kept. 0 keeps them forever.")
	ev.EnvConvert = DurationEnvToDays(false)
	Register(ev)
	al := retention(KeyAlertHistoryDays, "APP_ALERT_HISTORY_RETENTION", "30",
		"Keep alert history for", "Days an alert history entry is kept. 0 keeps them forever.")
	al.EnvConvert = DurationEnvToDays(true)
	Register(al)
	Register(Definition{
		Key: KeyCleanupInterval, Category: CategoryRetention, Kind: KindDuration, Default: cleanupDefaultInterval,
		Env: "APP_CLEANUP_INTERVAL", Apply: ApplyLive, CheckValue: MinDurationCheck(CleanupMinInterval),
		TitleKey: "runtimeSettings:items.cleanupInterval.title", DescKey: "runtimeSettings:items.cleanupInterval.description",
		Title: "Cleanup interval", Description: "How often the scheduled cleanup checks every data type. The minimum is 15 minutes.",
	})
	Register(Definition{
		Key: KeyCleanupEnabled, Category: CategoryRetention, Kind: KindBool, Default: "true",
		Env: "APP_CLEANUP_ENABLED", Apply: ApplyLive,
		TitleKey: "runtimeSettings:items.cleanupEnabled.title", DescKey: "runtimeSettings:items.cleanupEnabled.description",
		Title: "Run cleanup on a schedule", Description: "Turn the scheduled cleanup off entirely. Manual cleanups still work.",
	})
}
