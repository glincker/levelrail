package runtimesettings

// KeyTemplatesFirstBootMaxSeconds caps a template's own first_boot_seconds.
const KeyTemplatesFirstBootMaxSeconds = "templates.first_boot_max_seconds"

func init() {
	Register(Definition{
		Key: KeyTemplatesFirstBootMaxSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "900",
		Env: "APP_TEMPLATE_FIRST_BOOT_MAX_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(30, 7200),
		Title:       "Template first-boot ceiling",
		Description: "The longest a template may ask a slow first start to take, in seconds. A template that declares more is lowered to this value.",
	})
}
