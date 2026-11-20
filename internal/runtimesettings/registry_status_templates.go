package runtimesettings

// KeyStatusTemplatesMax caps saved status page incident templates.
const KeyStatusTemplatesMax = "status_page.templates_max"

func init() {
	registerLimit(Definition{
		Key: KeyStatusTemplatesMax, Category: CategoryObservability, Default: "50", Env: "APP_STATUS_TEMPLATES_MAX", Apply: ApplyLive,
		Title:       "Status page incident templates",
		Description: "Most saved incident templates the status page keeps. From 1 to 500. Creating one past the cap is refused.",
	}, 1, 500)
}
