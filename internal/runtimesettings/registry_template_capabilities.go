package runtimesettings

// KeyTemplatesCapNetAdmin gates the Elevated net_admin capability class.
const KeyTemplatesCapNetAdmin = "templates.capability_net_admin_enabled"

func init() {
	Register(Definition{
		Key: KeyTemplatesCapNetAdmin, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_TEMPLATE_CAP_NET_ADMIN", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow templates to add NET_ADMIN and NET_RAW",
		Description: "Lets a template that declares the net_admin capability install and run with those two Linux capabilities. Off by default; turning it on needs root.",
	})
}
