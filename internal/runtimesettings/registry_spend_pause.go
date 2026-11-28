package runtimesettings

// KeySpendPauseStopsApps makes a spend cap with action pause stop the app's containers.
const KeySpendPauseStopsApps = "spend.pause_stops_apps"

func init() {
	Register(Definition{
		Key: KeySpendPauseStopsApps, Category: CategoryDeploys, Kind: KindBool, Default: "false",
		Env: "APP_SPEND_PAUSE_STOPS_APPS", Apply: ApplyLive,
		Title:       "Spend cap pause stops containers",
		Description: "Off by default: a paused app only answers with a maintenance page. Turn on to stop its containers too. A capped app is not woken by requests; it starts again when the limit resets or is raised.",
	})
}
