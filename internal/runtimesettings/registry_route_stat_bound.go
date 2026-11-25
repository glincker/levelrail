package runtimesettings

// KeyAPIRouteStatBound caps how many distinct API routes the hot routes list tracks.
const KeyAPIRouteStatBound = "api.route_stat_bound"

func init() {
	registerLimit(Definition{
		Key: KeyAPIRouteStatBound, Category: CategoryObservability, Default: "1024", Env: "APP_API_ROUTE_STAT_BOUND", Apply: ApplyLive,
		Title:       "Hot routes tracked",
		Description: "The most distinct API routes counted for the control plane health hot routes list, from 64 to 65536. Routes past the cap are not counted. A saved change applies to the next request.",
	}, 64, 65536)
}
