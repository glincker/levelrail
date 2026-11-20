package runtimesettings

// KeyAlertsNotifyOnSleep lets alerts through for apps that sleep when idle.
const KeyAlertsNotifyOnSleep = "alerts.notify_on_sleep"

func init() {
	Register(Definition{
		Key: KeyAlertsNotifyOnSleep, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_ALERTS_NOTIFY_ON_SLEEP", Apply: ApplyLive,
		Title:       "Alert on apps that sleep when idle",
		Description: "Off by default: an app stopped by sleep-when-idle sends no alerts while it sleeps. Turn on to be told about sleeping apps too.",
	})
}
