package runtimesettings

// KeyAlertsRenotifyInterval repeats a still-firing alert until it is read in the Inbox.
const KeyAlertsRenotifyInterval = "alerts.renotify_interval"

func init() {
	Register(Definition{
		Key: KeyAlertsRenotifyInterval, Category: CategoryObservability, Kind: KindInt, Default: "0",
		Env: "APP_ALERTS_RENOTIFY_INTERVAL", Apply: ApplyLive, CheckValue: intRangeCheck(0, 720),
		Title:       "Repeat unread alerts every N hours",
		Description: "Hours between repeat notifications for an alert that is still firing and has not been read in the Inbox. 0 turns repeats off.",
	})
}
