package runtimesettings

// KeyTemplatesCapDockerAPI gates the Elevated docker_api capability class.
const KeyTemplatesCapDockerAPI = "templates.cap_docker_api"

func init() {
	Register(Definition{
		Key: KeyTemplatesCapDockerAPI, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_TEMPLATE_CAP_DOCKER_API", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow templates to use a scoped Docker API",
		Description: "Lets a template that declares the docker_api capability get its own filtered Docker socket, limited to that app's containers. The host socket is never shared. Off by default; turning it on needs root, and turning it off cuts access at once.",
	})
}

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
