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

// KeyTemplatesCapHostNetwork gates the Elevated host_network capability class.
const KeyTemplatesCapHostNetwork = "templates.cap_host_network"

// KeyTemplatesCapDevices gates the Elevated devices capability class.
const KeyTemplatesCapDevices = "templates.cap_devices"

func init() {
	Register(Definition{
		Key: KeyTemplatesCapHostNetwork, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_TEMPLATE_CAP_HOST_NETWORK", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow templates to use the host network",
		Description: "Lets a template that declares the host_network capability run on the host network, for discovery and LAN protocols. The app then sees every host interface and port. Off by default; turning it on needs root, and turning it off stops the grant at the next reconcile.",
	})
	Register(Definition{
		Key: KeyTemplatesCapDevices, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_TEMPLATE_CAP_DEVICES", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow templates to use host devices",
		Description: "Lets a template that declares the devices capability map /dev/dri, /dev/ttyUSB*, /dev/ttyACM* or /dev/video* into its container. No other device is ever allowed. Off by default; turning it on needs root, and turning it off stops the grant at the next reconcile.",
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

// KeyTemplatesLogsGrantMultiTeam lets the logs docker_api grant be installed on an instance with several teams.
const KeyTemplatesLogsGrantMultiTeam = "templates.logs_grant_allow_multi_team"

func init() {
	Register(Definition{
		Key: KeyTemplatesLogsGrantMultiTeam, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_TEMPLATE_LOGS_GRANT_ALLOW_MULTI_TEAM", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow the logs grant on multi team instances",
		Description: "The logs docker_api grant reads the logs of every app on the host, whatever team owns it. With more than one team, installs and upgrades that carry it are refused unless this is on. Off by default; turning it on needs root and exposes all teams' logs to whoever can reach the granted container.",
	})
}
