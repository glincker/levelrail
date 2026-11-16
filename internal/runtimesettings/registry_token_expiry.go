package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Token expiry reminders, rotation grace and git credential renewal.
const (
	KeyTokensReminderWindows   = "credential_expiry.reminder_windows_days" //nolint:gosec // setting key, not a credential
	KeyTokensReminderInterval  = "credential_expiry.reminder_interval"     //nolint:gosec // setting key, not a credential
	KeyTokensRotationGrace     = "credential_expiry.rotation_grace"        //nolint:gosec // setting key, not a credential
	tokenReminderMaxWindowDays = 365
	tokenReminderMinInterval   = time.Minute
	tokenReminderMaxInterval   = 24 * time.Hour
	tokenRotationGraceMax      = 30 * 24 * time.Hour
)

func reminderWindowCheck(item string) error {
	n, err := strconv.Atoi(strings.TrimSpace(item))
	if err != nil || n < 1 || n > tokenReminderMaxWindowDays {
		return fmt.Errorf("%q is not a whole number of days from 1 to %d", item, tokenReminderMaxWindowDays)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyTokensReminderWindows, Category: CategorySecurity, Kind: KindStringList, Default: "14:7:1",
		Env: "APP_CREDENTIAL_REMINDER_WINDOWS_DAYS", Apply: ApplyLive, CheckItem: reminderWindowCheck,
		Title:       "Token expiry reminder windows",
		Description: "Days before expiry at which a token, or a pasted git provider token, raises a reminder: one per window per token, and once more when it has expired. Whole days from 1 to 365, for example 14:7:1 in the environment, or a list in the API.",
	})
	Register(Definition{
		Key: KeyTokensReminderInterval, Category: CategorySecurity, Kind: KindDuration, Default: "6h",
		Env: "APP_CREDENTIAL_REMINDER_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(tokenReminderMinInterval, tokenReminderMaxInterval),
		Title:       "Token expiry check interval",
		Description: "How often the control plane looks for tokens inside a reminder window, from 1m to 24h.",
	})
	Register(Definition{
		Key: KeyTokensRotationGrace, Category: CategorySecurity, Kind: KindDuration, Default: "24h",
		Env: "APP_CREDENTIAL_ROTATION_GRACE", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, tokenRotationGraceMax),
		Title:       "Token rotation grace",
		Description: "How long the old token keeps working after a rotation so callers can switch to the new secret, from 1m to 720h. After that it is revoked.",
	})
}
