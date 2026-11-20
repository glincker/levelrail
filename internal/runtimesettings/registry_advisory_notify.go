package runtimesettings

// Advisory alert keys: off by default so a feed never adds noise unasked.
const (
	KeyAlertsAdvisoryNotify      = "alerts.advisory_notify"
	KeyAlertsAdvisoryMinSeverity = "alerts.advisory_min_severity"
)

func init() {
	Register(Definition{
		Key: KeyAlertsAdvisoryNotify, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_ALERTS_ADVISORY_NOTIFY", Apply: ApplyLive,
		Title:       "Alert on advisory matches for running images",
		Description: "Off by default. When on, an advisory_match alert rule fires while a running image matches a feed advisory at or above the severity floor. Needs an advisory feed to be configured.",
	})
	Register(Definition{
		Key: KeyAlertsAdvisoryMinSeverity, Category: CategoryObservability, Kind: KindString, Default: "high",
		Env: "APP_ALERTS_ADVISORY_MIN_SEVERITY", Apply: ApplyLive, AllowedStrings: []string{"low", "medium", "high", "critical"},
		Title:       "Lowest advisory severity that alerts",
		Description: "Advisory matches below this severity never reach an advisory_match alert rule.",
	})
}
