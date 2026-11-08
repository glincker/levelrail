package runtimesettings

// KeyUIOutboundUTM controls whether the dashboard tags outbound links to third party sites with UTM parameters.
const KeyUIOutboundUTM = "ui.outbound_utm"

func init() {
	Register(Definition{
		Key: KeyUIOutboundUTM, Category: CategoryObservability, Kind: KindBool, Default: "true",
		Env: "APP_UI_OUTBOUND_UTM", Apply: ApplyLive,
		Title:       "Tag outbound links with UTM parameters",
		Description: "When on, the dashboard adds utm_source, utm_medium and utm_campaign to links that leave for third party sites, such as template homepages, docs and upstream projects, so project maintainers can see where visits come from. Turn it off to send plain links. Only the dashboard reads this value; links are never rewritten on the server.",
	})
}
