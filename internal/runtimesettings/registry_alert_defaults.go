package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
)

// Default alert and weekly digest setting keys.
const (
	KeyAlertsDefaultsEnabled   = "alerts.defaults_enabled"
	KeyAlertsDefaultCrashLoop  = "alerts.default_crashloop_restarts"
	KeyAlertsDefault5xxPercent = "alerts.default_5xx_percent"
	KeyAlertsDefault5xxMinReqs = "alerts.default_5xx_min_requests"
	KeyAlertsDigestSchedule    = "alerts.digest_schedule"
)

// DigestScheduleOff is the value that turns the weekly digest off.
const DigestScheduleOff = "off"

// ParseDigestSchedule reads "off" or "<day>@<hour>" (day mon..sun, hour 0..23, UTC), e.g. "mon@08".
func ParseDigestSchedule(v string) (weekday int, hour int, on bool, err error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == DigestScheduleOff {
		return 0, 0, false, nil
	}
	day, hh, ok := strings.Cut(v, "@")
	days := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	d, known := days[day]
	h, herr := strconv.Atoi(hh)
	if !ok || !known || herr != nil || h < 0 || h > 23 {
		return 0, 0, false, fmt.Errorf("use off or <day>@<hour>, for example mon@08")
	}
	return d, h, true, nil
}

func init() {
	Register(Definition{
		Key: KeyAlertsDefaultsEnabled, Category: CategoryObservability, Kind: KindBool, Default: "true",
		Env: "APP_ALERTS_DEFAULTS_ENABLED", Apply: ApplyLive,
		Title:       "Default alerts",
		Description: "Master switch for the built-in alert rules (crash loop, disk, certificate expiry, 5xx rate, failed backups). Turning it off disables them; rules you deleted are never re-added.",
	})
	Register(Definition{
		Key: KeyAlertsDefaultCrashLoop, Category: CategoryObservability, Kind: KindInt, Default: "3",
		Env: "APP_ALERTS_DEFAULT_CRASHLOOP_RESTARTS", Apply: ApplyLive, CheckValue: intRangeCheck(2, 50),
		Title:       "Default crash loop restarts",
		Description: "Restarts of one app inside ten minutes that fire the default crash loop alert, from 2 to 50.",
	})
	Register(Definition{
		Key: KeyAlertsDefault5xxPercent, Category: CategoryObservability, Kind: KindInt, Default: "5",
		Env: "APP_ALERTS_DEFAULT_5XX_PERCENT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100),
		Title:       "Default 5xx rate",
		Description: "Percent of 5xx responses over the request window that fires the default error rate alert for any app, from 1 to 100.",
	})
	Register(Definition{
		Key: KeyAlertsDefault5xxMinReqs, Category: CategoryObservability, Kind: KindInt, Default: "20",
		Env: "APP_ALERTS_DEFAULT_5XX_MIN_REQUESTS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100000),
		Title:       "Default 5xx minimum requests",
		Description: "Requests an app needs inside the request window before the default 5xx alert can fire, so one failed request on a quiet app never pages you.",
	})
	Register(Definition{
		Key: KeyAlertsDigestSchedule, Category: CategoryObservability, Kind: KindString, Default: DigestScheduleOff,
		Env: "APP_ALERTS_DIGEST_SCHEDULE", Apply: ApplyLive,
		CheckValue: func(v string) error {
			_, _, _, err := ParseDigestSchedule(v)
			return err
		},
		Title:       "Weekly digest schedule",
		Description: "Send one weekly summary mail to the admin (apps, deploys, restarts, certificates, backups, disk). Off, or a day and UTC hour such as mon@08. Needs email configured.",
	})
}
