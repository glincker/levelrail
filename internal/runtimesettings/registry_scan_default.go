package runtimesettings

// Registered keys for the instance default scan policy apps inherit.
const (
	KeyScanDefaultEnabled = "supply_chain.default_enabled"
	KeyScanDefaultGate    = "supply_chain.default_gate"
)

func init() {
	Register(Definition{
		Key: KeyScanDefaultEnabled, Category: CategorySecurity, Kind: KindBool,
		Default: "false", Env: "APP_SCAN_DEFAULT_ENABLED", Apply: ApplyLive,
		Title:       "Scan apps by default",
		Description: "Whether apps without their own scan policy get their images scanned. An app's own policy always wins. Read on every deploy; the environment variable only seeds the default.",
	})
	Register(Definition{
		Key: KeyScanDefaultGate, Category: CategorySecurity, Kind: KindString,
		Default: "off", Env: "APP_SCAN_DEFAULT_GATE", Apply: ApplyLive,
		AllowedStrings: []string{"off", "warn", "block_on_critical", "block_on_high"},
		Title:          "Default scan gate",
		Description:    "What a scan result does to a release for apps without their own policy: off, warn, block on critical findings or block on high findings. Needs scanning turned on to take effect.",
	})
}
